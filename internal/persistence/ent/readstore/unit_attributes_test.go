package readstore_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
	"github.com/gopager/gopager/internal/units"
)

// unitSchemas declares the Unit attributes that the predicate tests address.
func unitSchemas() []attributes.Schema {
	return []attributes.Schema{{Key: "station", Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{
		"town": {Kind: processing.StringValue},
		"kind": {Kind: processing.StringValue},
		"tags": {Kind: processing.ListValue, Element: &attributes.Field{Kind: processing.StringValue}},
		"site": {Kind: processing.ObjectValue, Properties: map[string]attributes.Field{
			"zone": {Kind: processing.StringValue},
			"geo": {Kind: processing.ObjectValue, Properties: map[string]attributes.Field{
				"grid": {Kind: processing.IntegerValue},
			}},
		}},
	}}}
}

// compileUnits checks the predicate JSON against the built-in fields and the Unit attributes.
func compileUnits(t testing.TB, predicate string) *fields.Filter {
	t.Helper()
	var decoded processing.Predicate
	if err := processing.Decode([]byte(predicate), &decoded); err != nil {
		t.Fatalf("predicate %s: %v", predicate, err)
	}
	registry := fields.New(fields.Builtin(), attributes.NewProvider(append(testSchemas(), unitSchemas()...), 1))
	filter, err := fields.CompileFilter(decoded, registry.Declarations(fields.ScopeMessage, true))
	if err != nil {
		t.Fatalf("predicate %s: %v", predicate, err)
	}
	return filter
}

// unitSchema registers a `unit` schema and assigns it to the Agencies.
func (w *world) unitSchema(key string, agencies ...uuid.UUID) {
	w.exec(`INSERT INTO attribute_schemas (key, revision, label, subject, fields, created_by, created_at, updated_at)
		VALUES ($1, 1, $1, 'unit', '{}', $2, $3, $3)`, key, uuid.New(), base)
	for _, agency := range agencies {
		w.exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ($1, $2)`, key, agency)
	}
}

// settings sets the stored attribute entries of a Unit; entries are rendered by own and unset.
func (w *world) settings(unit uuid.UUID, attributes string) {
	w.exec(`UPDATE units SET settings = jsonb_build_object('attributes', $2::jsonb) WHERE id = $1`, unit, attributes)
}

// defaults sets the attribute defaults of an Organization.
func (w *world) defaults(organization uuid.UUID, attributes string) {
	w.exec(`UPDATE organizations SET defaults = jsonb_build_object('attributes', $2::jsonb) WHERE id = $1`, organization, attributes)
}

// own renders a stored "value" entry of a station field.
func own(name, kind, raw string) string {
	return `"` + name + `":{"mode":"value","value":` + value(kind, raw) + `}`
}

// station renders the entries of the schema `station`.
func station(entries ...string) string { return `{"station":{` + strings.Join(entries, ",") + `}}` }

const unset = `{"mode":"unset"}`

// matching returns the Messages that the predicate selects, with the simple filters.
func (w *world) matching(predicate string, filters readmodel.Filters) []uuid.UUID {
	w.t.Helper()
	validated, err := filters.Validate()
	if err != nil {
		w.t.Fatal(err)
	}
	return ids(w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{Filters: validated, Selection: compileUnits(w.t, predicate)}))
}

func str(path, text string) string { return compare("eq", path, "string", `"`+text+`"`) }

// TestUnitAttributePredicateHoldsOnOneAppearance shows that all Unit leaves of a predicate
// describe the same Unit, that the simple Unit filters meet them on that appearance and that
// other fields keep reading the Message.
func TestUnitAttributePredicateHoldsOnOneAppearance(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	w.unitSchema("station", agency)
	x, y := w.unit(agency, "X", nil), w.unit(agency, "Y", nil)
	w.settings(x, station(own("town", "string", `"A"`), own("kind", "string", `"ladder"`)))
	w.settings(y, station(own("town", "string", `"B"`), own("kind", "string", `"engine"`)))
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	w.assigned(message, network, x, agency, base, "")
	w.assigned(message, network, y, agency, base, "")
	onlyX := w.message(base.Add(time.Minute), "text", "other")
	w.assigned(onlyX, network, x, agency, base.Add(time.Minute), "")
	town, kind := "unit.attributes.station.town", "unit.attributes.station.kind"
	for name, test := range map[string]struct {
		predicate string
		filters   readmodel.Filters
		want      []uuid.UUID
	}{
		"town A and kind engine":           {logical("and", str(town, "A"), str(kind, "engine")), readmodel.Filters{}, nil},
		"town B and kind engine":           {logical("and", str(town, "B"), str(kind, "engine")), readmodel.Filters{}, []uuid.UUID{message}},
		"town A and kind ladder":           {logical("and", str(town, "A"), str(kind, "ladder")), readmodel.Filters{}, []uuid.UUID{onlyX, message}},
		"kind engine on unit X":            {str(kind, "engine"), readmodel.Filters{Units: []uuid.UUID{x}}, nil},
		"kind engine on unit Y":            {str(kind, "engine"), readmodel.Filters{Units: []uuid.UUID{y}}, []uuid.UUID{message}},
		"town B and kind engine on unit X": {logical("and", str(town, "B"), str(kind, "engine")), readmodel.Filters{Units: []uuid.UUID{x}}, nil},
		"not town A":                       {logical("not", str(town, "A")), readmodel.Filters{}, []uuid.UUID{message}},
		"or across appearances":            {logical("or", str(town, "A"), str(kind, "engine")), readmodel.Filters{}, []uuid.UUID{onlyX, message}},
		"unit leaf and Message field":      {logical("and", str(town, "A"), str("message.unitNames", "Y")), readmodel.Filters{}, []uuid.UUID{message}},
	} {
		if got := w.matching(test.predicate, test.filters); !slices.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", name, got, test.want)
		}
	}
}

// TestUnitAttributesInheritTheOrganizationDefault covers inheritance: a default counts only for
// a missing entry and only while the schema is assigned to the Unit's Agency.
func TestUnitAttributesInheritTheOrganizationDefault(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	assigned, other := w.agency("county"), w.agency("other")
	w.unitSchema("station", assigned)
	organization := w.organization("fire")
	w.defaults(organization, station(own("town", "string", `"D"`)))
	inheriting, unassigned := w.unit(assigned, "inheriting", &organization), w.unit(other, "unassigned", &organization)
	hidden, overriding := w.unit(assigned, "hidden", &organization), w.unit(assigned, "overriding", &organization)
	w.settings(hidden, station(`"town":`+unset))
	w.settings(overriding, station(own("town", "string", `"Own"`)))
	messages := map[string]uuid.UUID{}
	for name, unit := range map[string]uuid.UUID{"inheriting": inheriting, "unassigned": unassigned, "hidden": hidden, "overriding": overriding} {
		agency, network := assigned, w.network(assigned, name, "pocsag")
		if unit == unassigned {
			agency, network = other, w.network(other, name, "pocsag")
		}
		messages[name] = w.message(base, "text", name)
		w.assigned(messages[name], network, unit, agency, base, "")
	}
	town := "unit.attributes.station.town"
	for name, test := range map[string]struct {
		predicate string
		want      []string
	}{
		"default":         {str(town, "D"), []string{"inheriting"}},
		"own value":       {str(town, "Own"), []string{"overriding"}},
		"present":         {present(town, true), []string{"inheriting", "overriding"}},
		"absent":          {present(town, false), []string{"hidden", "unassigned"}},
		"contains of own": {compare("contains", town, "string", `"w"`), []string{"overriding"}},
	} {
		got := w.matching(test.predicate, readmodel.Filters{})
		var want []uuid.UUID
		for _, key := range test.want {
			want = append(want, messages[key])
		}
		if !sameIDs(got, want) {
			t.Errorf("%s: %d messages, want %v", name, len(got), test.want)
		}
	}
}

// TestUnitAttributeListsAndObjects covers list fields (any element), object property paths, a
// Unit-less appearance (no candidates) and a deleted Unit (keeps its values).
func TestUnitAttributeListsAndObjects(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	w.unitSchema("station", agency)
	network := w.network(agency, "N", "pocsag")
	rich := w.unit(agency, "rich", nil)
	w.settings(rich, station(
		own("tags", "list", `[`+value("string", `"a"`)+`,`+value("string", `"b"`)+`]`),
		own("site", "object", `{"zone":`+value("string", `"north"`)+`,"geo":`+value("object", `{"grid":`+value("integer", `"7"`)+`}`)+`}`)))
	gone := w.unit(agency, "gone", nil)
	w.settings(gone, station(own("town", "string", `"Old"`)))
	w.exec(`UPDATE units SET deleted = true WHERE id = $1`, gone)
	withUnit, goneMessage, unitless := w.message(base, "text", "a"), w.message(base, "text", "b"), w.message(base, "text", "c")
	w.assigned(withUnit, network, rich, agency, base, "")
	w.assigned(goneMessage, network, gone, agency, base, "")
	w.transmission(transmissionSpec{message: unitless, network: network, accepted: base})
	w.appearance(unitless, nil, agency, base, base)
	for name, test := range map[string]struct {
		predicate string
		want      []uuid.UUID
	}{
		"list element":         {str("unit.attributes.station.tags", "b"), []uuid.UUID{withUnit}},
		"other list element":   {str("unit.attributes.station.tags", "z"), nil},
		"object property":      {str("unit.attributes.station.site.zone", "north"), []uuid.UUID{withUnit}},
		"nested property":      {compare("gte", "unit.attributes.station.site.geo.grid", "integer", `"7"`), []uuid.UUID{withUnit}},
		"nested other":         {compare("gt", "unit.attributes.station.site.geo.grid", "integer", `"7"`), nil},
		"deleted unit":         {str("unit.attributes.station.town", "Old"), []uuid.UUID{goneMessage}},
		"unit-less absent":     {present("unit.attributes.station.town", false), []uuid.UUID{unitless, withUnit}},
		"unit-less not equals": {logical("not", str("unit.attributes.station.town", "Old")), []uuid.UUID{unitless, withUnit}},
	} {
		if got := w.matching(test.predicate, readmodel.Filters{}); !sameIDs(got, test.want) {
			t.Errorf("%s: %v, want %v", name, got, test.want)
		}
	}
}

// sameIDs reports whether both lists hold the same IDs, whatever their order.
func sameIDs(got, want []uuid.UUID) bool {
	order := func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) }
	return slices.Equal(slices.SortedFunc(slices.Values(got), order), slices.SortedFunc(slices.Values(want), order))
}

// flatten lists effective attributes as "key.field=kind:raw@source" sorted, for comparison.
func flatten(t *testing.T, list []readmodel.UnitAttribute) []string {
	t.Helper()
	var out []string
	for _, entry := range list {
		var decoded any
		if err := json.Unmarshal(entry.Value.Value, &decoded); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(decoded)
		out = append(out, entry.SchemaKey+"."+entry.Field+"="+string(entry.Value.Kind)+":"+string(encoded)+"@"+entry.Source)
	}
	return out
}

// TestUnitAttributeProjectionMatchesTheGoResolution compares the SQL effective list of a Unit,
// as the feed and the detail show it, with units.ResolveAttributes.
func TestUnitAttributeProjectionMatchesTheGoResolution(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	assigned, other := w.agency("county"), w.agency("other")
	w.unitSchema("station", assigned)
	w.unitSchema("fleet", assigned, other)
	organization := w.organization("fire")
	pv := func(kind, raw string) *processing.Value {
		return &processing.Value{Kind: processing.ValueKind(kind), Value: json.RawMessage(raw)}
	}
	defaults := units.AttributeDefaults{
		"station": {"town": {Mode: "value", Value: pv("string", `"D"`)}, "kind": {Mode: "value", Value: pv("string", `"E"`)}},
		"fleet":   {"size": {Mode: "value", Value: pv("integer", `"12"`)}},
	}
	encodedDefaults, _ := json.Marshal(map[string]any{"attributes": defaults})
	w.exec(`UPDATE organizations SET defaults = $2::jsonb WHERE id = $1`, organization, string(encodedDefaults))
	cases := map[string]struct {
		agency   uuid.UUID
		settings units.AttributeSettings
		detached bool
	}{
		"inherits all": {assigned, nil, false},
		"mixed": {assigned, units.AttributeSettings{
			"station": {"town": {Mode: "value", Value: pv("object", `{"b":`+value("string", `"x"`)+`,"a":`+value("integer", `"1"`)+`}`)},
				"kind": {Mode: "unset"}},
			"fleet": {"extra": {Mode: "value", Value: pv("boolean", `true`)}}}, false},
		"other agency":    {other, units.AttributeSettings{"station": {"town": {Mode: "value", Value: pv("string", `"own"`)}}}, false},
		"no organization": {assigned, units.AttributeSettings{"fleet": {"size": {Mode: "value", Value: pv("integer", `"3"`)}}}, true},
	}
	schemas := units.AttributeSchemas{"station": {Agencies: []uuid.UUID{assigned}}, "fleet": {Agencies: []uuid.UUID{assigned, other}}}
	messages := map[string]uuid.UUID{}
	for name, test := range cases {
		var org *uuid.UUID
		if !test.detached {
			org = &organization
		}
		unit := w.unit(test.agency, name, org)
		encoded, _ := json.Marshal(map[string]any{"attributes": test.settings})
		w.exec(`UPDATE units SET settings = $2::jsonb WHERE id = $1`, unit, string(encoded))
		network := w.network(test.agency, name, "pocsag")
		messages[name] = w.message(base, "text", name)
		w.assigned(messages[name], network, unit, test.agency, base, "")
	}
	for name, test := range cases {
		var resolvedDefaults units.AttributeDefaults
		if !test.detached {
			resolvedDefaults = defaults
		}
		want := units.ResolveAttributes(test.settings, resolvedDefaults, schemas.AssignedTo(test.agency))
		var expected []string
		for key, fieldsOf := range want {
			for field, effective := range fieldsOf {
				var decoded any
				_ = json.Unmarshal(effective.Value.Value, &decoded)
				encoded, _ := json.Marshal(decoded)
				expected = append(expected, key+"."+field+"="+string(effective.Value.Kind)+":"+string(encoded)+"@"+effective.Source)
			}
		}
		slices.Sort(expected)
		item := w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{IDs: []uuid.UUID{messages[name]}}).Items[0]
		got := flatten(t, item.Units[0].Attributes)
		if !slices.IsSorted(got) {
			t.Errorf("%s: attributes not sorted: %v", name, got)
		}
		slices.Sort(got)
		if !slices.Equal(got, expected) {
			t.Errorf("%s: SQL %v, Go %v", name, got, expected)
		}
		detail, err := w.detail(all(), base.Add(time.Hour), messages[name])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(detail.Units[0].Attributes, item.Units[0].Attributes) {
			t.Errorf("%s: detail %v differs from feed %v", name, detail.Units[0].Attributes, item.Units[0].Attributes)
		}
	}
}

// TestUnitAttributePredicateCombinesWithMessageAttributes shows that Message attribute leaves,
// which narrow the page through the index, and Unit leaves meet in one predicate.
func TestUnitAttributePredicateCombinesWithMessageAttributes(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	w.unitSchema("station", agency)
	x, y := w.unit(agency, "X", nil), w.unit(agency, "Y", nil)
	w.settings(x, station(own("town", "string", `"A"`)))
	w.settings(y, station(own("town", "string", `"B"`)))
	network := w.network(agency, "N", "pocsag")
	workflow, version := w.workflow()
	seven, three := w.message(base, "text", "seven"), w.message(base.Add(time.Minute), "text", "three")
	for message, level := range map[uuid.UUID]string{seven: "7", three: "3"} {
		first := w.assigned(message, network, x, agency, base, "")
		w.assigned(message, network, y, agency, base, "")
		w.result(first, workflow, version, message, base, `["derived"]`,
			untargeted(fieldsOf("incident", entry("level", "integer", `"`+level+`"`))))
	}
	level := compare("eq", "attributes.incident.level", "integer", `"7"`)
	town := str("unit.attributes.station.town", "B")
	for name, test := range map[string]struct {
		predicate string
		want      []uuid.UUID
	}{
		"both leaves":       {logical("and", level, town), []uuid.UUID{seven}},
		"message or unit":   {logical("or", level, str("unit.attributes.station.town", "Z")), []uuid.UUID{seven}},
		"unit only":         {town, []uuid.UUID{three, seven}},
		"level and no town": {logical("and", level, str("unit.attributes.station.town", "Z")), nil},
	} {
		got := ids(w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{Selection: compileUnits(t, test.predicate),
			DerivedGroups: []string{"derived"}}))
		if !sameIDs(got, test.want) {
			t.Errorf("%s: %v, want %v", name, got, test.want)
		}
	}
}
