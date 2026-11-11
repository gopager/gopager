package readstore_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// recordSource offers the values of a projected Message to fields.Filter.Match, the reference
// semantics that the SQL of the read store must agree with.
type recordSource struct{ record readmodel.FeedRecord }

func typed(kind processing.ValueKind, raw any) processing.Value {
	encoded, _ := json.Marshal(raw)
	return processing.Value{Kind: kind, Value: encoded}
}

func (s recordSource) Values(reference processing.FieldRef) []processing.Value {
	record := s.record
	if reference.Namespace == "attributes" {
		return s.attribute(reference.Path)
	}
	var payload struct{ Form, Value string }
	if record.Representative != nil {
		_ = json.Unmarshal(record.Representative.Payload, &payload)
	}
	switch strings.Join(reference.Path, ".") {
	case "id":
		return []processing.Value{typed(processing.StringValue, record.MessageID.String())}
	case "time":
		return []processing.Value{typed(processing.TimestampValue, record.Time.UTC().Format(time.RFC3339Nano))}
	case "processingStatus":
		return []processing.Value{typed(processing.StringValue, record.ProcessingStatus)}
	case "correctionStatus":
		return []processing.Value{typed(processing.StringValue, record.CorrectionStatus)}
	case "protocol":
		if record.Representative != nil {
			return []processing.Value{typed(processing.StringValue, record.Representative.Protocol)}
		}
	case "form":
		if record.Representative != nil {
			return []processing.Value{typed(processing.StringValue, payload.Form)}
		}
	case "text", "numeric":
		if record.Representative != nil && payload.Form == reference.Path[0] {
			return []processing.Value{typed(processing.StringValue, payload.Value)}
		}
	case "unitIds", "unitNames":
		list := []processing.Value{}
		for _, unit := range record.Units {
			if reference.Path[0] == "unitIds" {
				list = append(list, typed(processing.StringValue, unit.UnitID.String()))
			} else {
				list = append(list, typed(processing.StringValue, unit.Name))
			}
		}
		return []processing.Value{typed(processing.ListValue, list)}
	}
	return nil
}

// attribute resolves attributes.<schemaKey>.<field>[.<property>…] over every effective position.
func (s recordSource) attribute(path []string) []processing.Value {
	var values []processing.Value
	for _, attribute := range s.record.Attributes {
		if attribute.SchemaKey != path[0] || attribute.Field != path[1] {
			continue
		}
		var current processing.Value
		if json.Unmarshal(attribute.Value, &current) != nil {
			continue
		}
		found := true
		for _, property := range path[2:] {
			var object map[string]processing.Value
			if current.Kind != processing.ObjectValue || json.Unmarshal(current.Value, &object) != nil {
				found = false
				break
			}
			if current, found = object[property]; !found {
				break
			}
		}
		if found {
			values = append(values, current)
		}
	}
	return values
}

// TestPredicatesAgreeBetweenSQLAndTheGoEvaluator runs a table of predicates over a set of
// Messages through the SQL feed and through fields.Filter.Match on the projected values, and
// requires the same Messages. Expected labels pin the meaning of the cases.
func TestPredicatesAgreeBetweenSQLAndTheGoEvaluator(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	workflow, version := w.workflow()
	const groups = `["derived","payload"]`
	labels := map[uuid.UUID]string{}
	add := func(label string, message uuid.UUID) uuid.UUID { labels[message] = label; return message }

	m1 := add("m1", w.message(base, "text", "alarm"))
	t1 := w.assigned(m1, network, a, agency, base, `{"form":"text","value":"alarm"}`)
	w.result(t1, workflow, version, m1, base.Add(time.Minute), groups, untargeted(fieldsOf("incident",
		entry("level", "integer", `"7"`), entry("name", "string", `"Brand"`),
		entry("tags", "list", `[`+value("string", `"x"`)+`,`+value("string", `"y"`)+`]`),
		entry("ratio", "number", `1.5`), entry("urgent", "boolean", `true`),
		entry("at", "timestamp", `"2026-11-08T12:00:00Z"`),
		entry("levels", "list", `[`+value("integer", `"1"`)+`,`+value("integer", `"5"`)+`]`),
		entry("owner", "object", `{"name":`+value("string", `"Ada"`)+`,"age":`+value("integer", `"30"`)+`}`),
		entry("note", "null", `null`))))

	m2 := add("m2", w.message(base.Add(time.Minute), "numeric", "1234"))
	t2 := w.assigned(m2, network, b, agency, base.Add(time.Minute), `{"form":"numeric","value":"1234"}`)
	w.result(t2, workflow, version, m2, base.Add(time.Minute), groups, untargeted(fieldsOf("incident",
		entry("level", "integer", `"3"`), entry("city", "string", `"Köln"`))))
	w.result(t2, workflow, version, m2, base.Add(2*time.Minute), groups, untargeted(fieldsOf("incident",
		entry("level", "integer", `"9"`), entry("name", "string", `"brand"`), entry("tags", "list", `[]`),
		entry("ratio", "number", `2.5`), entry("urgent", "boolean", `false`),
		entry("at", "timestamp", `"2026-11-08T13:30:00.5Z"`),
		entry("owner", "object", `{"name":`+value("string", `"Bob"`)+`}`),
		entry("note", "string", `"n"`))))
	w.result(t2, workflow, version, m2, base.Add(3*time.Minute), groups,
		targeted(0, fieldsOf("incident", entry("city", "string", `"Bonn"`))))

	m3 := add("m3", w.message(base.Add(2*time.Minute), "text", "plain"))
	w.transmission(transmissionSpec{message: m3, network: network, accepted: base.Add(2 * time.Minute),
		status: "pending", payload: `{"form":"text","value":"plain"}`})
	w.appearance(m3, nil, agency, base.Add(2*time.Minute), base.Add(2*time.Minute))

	m4 := add("m4", w.message(base.Add(3*time.Minute), "text", "both"))
	w.assigned(m4, network, a, agency, base.Add(3*time.Minute), `{"form":"text","value":"both"}`)
	t4 := w.assigned(m4, network, b, agency, base.Add(4*time.Minute), `{"form":"text","value":"both"}`)
	w.exec(`UPDATE transmissions SET protocol = 'flex' WHERE id = $1`, t4)
	w.correcting(m4)
	w.result(t4, workflow, version, m4, base.Add(time.Minute), groups, targeted(0, fieldsOf("incident",
		entry("city", "string", `"Bonn"`), entry("level", "integer", `"7"`))))

	level := "attributes.incident.level"
	id := func(label string) string {
		for message, name := range labels {
			if name == label {
				return `"` + message.String() + `"`
			}
		}
		return `""`
	}
	cases := []struct {
		name      string
		predicate string
		want      []string
	}{
		{"integer eq", compare("eq", level, "integer", `"7"`), []string{"m1", "m4"}},
		{"integer in", member(level, "integer", `"3"`, `"9"`), []string{"m2"}},
		{"integer gt", compare("gt", level, "integer", `"7"`), []string{"m2"}},
		{"integer gte", compare("gte", level, "integer", `"7"`), []string{"m1", "m2", "m4"}},
		{"integer lt", compare("lt", level, "integer", `"9"`), []string{"m1", "m4"}},
		{"integer lte", compare("lte", level, "integer", `"7"`), []string{"m1", "m4"}},
		{"integer present", present(level, true), []string{"m1", "m2", "m4"}},
		{"integer absent", present(level, false), []string{"m3"}},
		{"number eq", compare("eq", "attributes.incident.ratio", "number", `1.5`), []string{"m1"}},
		{"number gt", compare("gt", "attributes.incident.ratio", "number", `2`), []string{"m2"}},
		{"number lte", compare("lte", "attributes.incident.ratio", "number", `2.5`), []string{"m1", "m2"}},
		{"boolean true", compare("eq", "attributes.incident.urgent", "boolean", `true`), []string{"m1"}},
		{"boolean false", compare("eq", "attributes.incident.urgent", "boolean", `false`), []string{"m2"}},
		{"timestamp eq", compare("eq", "attributes.incident.at", "timestamp", `"2026-11-08T13:00:00+01:00"`), []string{"m1"}},
		{"timestamp gt", compare("gt", "attributes.incident.at", "timestamp", `"2026-11-08T12:00:00Z"`), []string{"m2"}},
		{"timestamp lte", compare("lte", "attributes.incident.at", "timestamp", `"2026-11-08T13:30:00.5Z"`), []string{"m1", "m2"}},
		{"timestamp in", member("attributes.incident.at", "timestamp", `"2026-11-08T12:00:00Z"`), []string{"m1"}},
		{"string eq", compare("eq", "attributes.incident.name", "string", `"Brand"`), []string{"m1"}},
		{"string in", member("attributes.incident.name", "string", `"Brand"`, `"brand"`), []string{"m1", "m2"}},
		{"string contains", compare("contains", "attributes.incident.name", "string", `"ran"`), []string{"m1", "m2"}},
		{"string contains case", compare("contains", "attributes.incident.name", "string", `"Ran"`), nil},
		{"string contains empty", compare("contains", "attributes.incident.name", "string", `""`), []string{"m1", "m2"}},
		{"list element", compare("eq", "attributes.incident.tags", "string", `"y"`), []string{"m1"}},
		{"list contains", compare("contains", "attributes.incident.tags", "string", `"x"`), []string{"m1"}},
		{"list in", member("attributes.incident.tags", "string", `"y"`, `"z"`), []string{"m1"}},
		{"list present", present("attributes.incident.tags", true), []string{"m1"}},
		{"empty list is absent", present("attributes.incident.tags", false), []string{"m2", "m3", "m4"}},
		{"integer list", compare("gte", "attributes.incident.levels", "integer", `"5"`), []string{"m1"}},
		{"integer list none", compare("lt", "attributes.incident.levels", "integer", `"1"`), nil},
		{"null eq", compare("eq", "attributes.incident.note", "null", `null`), []string{"m1"}},
		{"null present", present("attributes.incident.note", true), []string{"m1", "m2"}},
		{"null or string", member("attributes.incident.note", "string", `"n"`), []string{"m2"}},
		{"property eq", compare("eq", "attributes.incident.owner.name", "string", `"Ada"`), []string{"m1"}},
		{"property gt", compare("gt", "attributes.incident.owner.age", "integer", `"20"`), []string{"m1"}},
		{"property absent", present("attributes.incident.owner.age", false), []string{"m2", "m3", "m4"}},
		{"target value", compare("eq", "attributes.incident.city", "string", `"Bonn"`), []string{"m2", "m4"}},
		{"two positions", compare("eq", "attributes.incident.city", "string", `"Köln"`), []string{"m2"}},
		{"not leaf", logical("not", compare("eq", level, "integer", `"7"`)), []string{"m2", "m3"}},
		{"not absent", logical("not", present(level, true)), []string{"m3"}},
		{"and", logical("and", compare("eq", level, "integer", `"7"`), compare("eq", "attributes.incident.name", "string", `"Brand"`)), []string{"m1"}},
		{"or", logical("or", compare("eq", level, "integer", `"9"`), compare("eq", "attributes.incident.name", "string", `"Brand"`)), []string{"m1", "m2"}},
		{"nested", logical("or", logical("and", present(level, true), logical("not", compare("eq", level, "integer", `"7"`))),
			compare("eq", "message.text", "string", `"plain"`)), []string{"m2", "m3"}},
		{"not over missing property", logical("not", compare("gt", "attributes.incident.owner.age", "integer", `"20"`)), []string{"m2", "m3", "m4"}},
		{"message id", compare("eq", "message.id", "string", id("m2")), []string{"m2"}},
		{"message id in", member("message.id", "string", id("m1"), id("m3")), []string{"m1", "m3"}},
		{"message time", compare("lt", "message.time", "timestamp", `"2026-11-08T12:01:30Z"`), []string{"m1", "m2"}},
		{"message time eq", compare("eq", "message.time", "timestamp", `"2026-11-08T12:02:00Z"`), []string{"m3"}},
		{"processing status", compare("eq", "message.processingStatus", "string", `"pending"`), []string{"m3"}},
		{"processing status none", compare("eq", "message.processingStatus", "string", `"none"`), []string{"m1", "m2", "m4"}},
		{"correction status", compare("eq", "message.correctionStatus", "string", `"correcting"`), []string{"m4"}},
		{"protocol", compare("eq", "message.protocol", "string", `"pocsag"`), []string{"m1", "m2", "m3", "m4"}},
		{"form", compare("eq", "message.form", "string", `"numeric"`), []string{"m2"}},
		{"text", compare("eq", "message.text", "string", `"alarm"`), []string{"m1"}},
		{"text contains", compare("contains", "message.text", "string", `"la"`), []string{"m1", "m3"}},
		{"numeric", compare("eq", "message.numeric", "string", `"1234"`), []string{"m2"}},
		{"numeric absent", present("message.numeric", false), []string{"m1", "m3", "m4"}},
		{"unit names", compare("eq", "message.unitNames", "string", `"A"`), []string{"m1", "m4"}},
		{"unit names in", member("message.unitNames", "string", `"B"`, `"C"`), []string{"m2", "m4"}},
		{"unit names contains", compare("contains", "message.unitNames", "string", `"B"`), []string{"m2", "m4"}},
		{"unit names absent", present("message.unitNames", false), []string{"m3"}},
		{"unit ids", compare("eq", "message.unitIds", "string", `"`+a.String()+`"`), []string{"m1", "m4"}},
		{"mixed namespaces", logical("and", compare("eq", "message.unitNames", "string", `"A"`),
			compare("gte", level, "integer", `"7"`)), []string{"m1", "m4"}},
	}
	caller := all()
	now := base.Add(time.Hour)
	allGroups := []string{"derived", "gateway", "payload", "unit"}
	everything := w.feed(caller, now, readmodel.FeedQuery{DerivedGroups: allGroups})
	if len(everything.Items) != 4 {
		t.Fatalf("fixture has %d Messages", len(everything.Items))
	}
	labelsOf := func(found []uuid.UUID) []string {
		out := []string{}
		for _, message := range found {
			out = append(out, labels[message])
		}
		slices.Sort(out)
		return out
	}
	for _, test := range cases {
		filter := compile(t, test.predicate)
		sql := labelsOf(ids(w.feed(caller, now, readmodel.FeedQuery{Selection: filter, DerivedGroups: allGroups})))
		var reference []uuid.UUID
		for _, record := range everything.Items {
			if filter.Match(recordSource{record}) {
				reference = append(reference, record.MessageID)
			}
		}
		if got := labelsOf(reference); !slices.Equal(sql, got) {
			t.Errorf("%s: SQL %v, Go %v", test.name, sql, got)
		}
		want := test.want
		if want == nil {
			want = []string{}
		}
		if !slices.Equal(sql, want) {
			t.Errorf("%s: %v, want %v", test.name, sql, want)
		}
	}
}
