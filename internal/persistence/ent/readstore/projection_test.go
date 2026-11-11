package readstore_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

// projectedRows lists the rows of the value projection of a result as "schema/path@position=value".
func (w *world) projectedRows(result uuid.UUID) []string {
	w.t.Helper()
	var rows []string
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			w.t.Fatal(err)
		}
		found, err := executor.QueryContext(ctx, `SELECT schema_key, path, target_position, encode(text_digest, 'hex'), integer_value,
			number_value, to_char(time_value AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), boolean_value
			FROM message_attribute_values WHERE result_id = $1`, result)
		if err != nil {
			w.t.Fatal(err)
		}
		defer func() { _ = found.Close() }()
		for found.Next() {
			var key, path string
			var position sql.NullInt32
			var text, instant sql.NullString
			var integer sql.NullInt64
			var number sql.NullFloat64
			var boolean sql.NullBool
			if err := found.Scan(&key, &path, &position, &text, &integer, &number, &instant, &boolean); err != nil {
				w.t.Fatal(err)
			}
			at := ""
			if position.Valid {
				at = fmt.Sprintf("@%d", position.Int32)
			}
			var typed string
			switch {
			case text.Valid:
				typed = "text:" + text.String
			case integer.Valid:
				typed = fmt.Sprintf("integer:%d", integer.Int64)
			case number.Valid:
				typed = fmt.Sprintf("number:%g", number.Float64)
			case instant.Valid:
				typed = "time:" + instant.String
			case boolean.Valid:
				typed = fmt.Sprintf("boolean:%t", boolean.Bool)
			}
			rows = append(rows, key+"/"+path+at+"="+typed)
		}
		if err := found.Err(); err != nil {
			w.t.Fatal(err)
		}
	})
	slices.Sort(rows)
	return rows
}

// TestProjectionHoldsOneRowPerScalarPath expects scalars, list elements and the scalar
// properties of objects at any depth to be projected with their target position, and nulls,
// bytes, objects and lists of objects to produce no row.
func TestProjectionHoldsOneRowPerScalarPath(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	transmission := w.assigned(message, network, unit, agency, base, "")
	workflow, version := w.workflow()
	untargetedResult := w.result(transmission, workflow, version, message, base, `["derived"]`,
		untargeted(fieldsOf("incident",
			entry("level", "integer", `"7"`),
			entry("ratio", "number", `2.5`),
			entry("when", "timestamp", `"2026-11-08T12:00:00Z"`),
			entry("urgent", "boolean", `true`),
			entry("tags", "list", `[`+value("string", `"x"`)+`,`+value("string", `"y"`)+`]`),
			entry("owner", "object", `{"name":`+value("string", `"Ada"`)+`,"address":`+
				value("object", `{"city":`+value("string", `"Bonn"`)+`}`)+`,"nothing":`+value("null", `null`)+`}`),
			entry("people", "list", `[`+value("object", `{"name":`+value("string", `"Bob"`)+`}`)+`]`),
			entry("blob", "bytes", `"AAEC"`),
			entry("none", "null", `null`),
		)))
	want := []string{
		"incident/level=integer:7", "incident/ratio=number:2.5", "incident/when=time:2026-11-08T12:00:00Z",
		"incident/urgent=boolean:true", "incident/tags=text:" + digest("x"), "incident/tags=text:" + digest("y"),
		"incident/owner.name=text:" + digest("Ada"), "incident/owner.address.city=text:" + digest("Bonn"),
	}
	slices.Sort(want)
	if got := w.projectedRows(untargetedResult); !slices.Equal(got, want) {
		t.Errorf("untargeted rows:\n got %v\nwant %v", got, want)
	}
	targetedResult := w.result(transmission, workflow, version, message, base, `["derived"]`,
		targeted(2, fieldsOf("incident", entry("city", "string", `"Bonn"`))))
	if got, want := w.projectedRows(targetedResult), []string{"incident/city@2=text:" + digest("Bonn")}; !slices.Equal(got, want) {
		t.Errorf("targeted rows: got %v, want %v", got, want)
	}
}

// TestPrefilterNarrowsThroughTheProjection expects an indexed comparison to be answered from the
// projection while predicates without a necessary condition still reach the exact check. The
// projection rows are removed to tell the two apart.
func TestPrefilterNarrowsThroughTheProjection(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	transmission := w.assigned(message, network, unit, agency, base, "")
	workflow, version := w.workflow()
	w.result(transmission, workflow, version, message, base, `["derived","payload"]`,
		untargeted(fieldsOf("incident", entry("level", "integer", `"7"`))))
	w.exec(`DELETE FROM message_attribute_values`)
	level := compare("eq", "attributes.incident.level", "integer", `"7"`)
	for name, test := range map[string]struct {
		predicate string
		want      int
	}{
		"comparison":         {level, 0},
		"and with prefilter": {logical("and", present("message.text", true), level), 0},
		"or of prefilters":   {logical("or", level, member("attributes.incident.level", "integer", `"7"`, `"8"`)), 0},
		"or with an operand": {logical("or", level, present("message.text", true)), 1},
		"double negation":    {logical("not", logical("not", level)), 1},
		"presence":           {present("attributes.incident.level", true), 1},
		"and without any":    {logical("and", present("attributes.incident.level", true), present("message.text", true)), 1},
		"negated comparison": {logical("not", level), 0},
	} {
		result := w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{Selection: compile(t, test.predicate),
			DerivedGroups: []string{"derived", "payload"}})
		if len(result.Items) != test.want {
			t.Errorf("%s: %d rows, want %d", name, len(result.Items), test.want)
		}
	}
}

// digest renders the hex SHA-256 of a string like the projection stores it.
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TestProjectionStoresLongStringsAndPrefiltersThemByDigest expects a published string of 16 KiB
// of incompressible text to project without failing the publication, and an equality filter on
// it to find the Message through the digest index, while a different long string does not.
func TestProjectionStoresLongStringsAndPrefiltersThemByDigest(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	transmission := w.assigned(message, network, unit, agency, base, "")
	workflow, version := w.workflow()
	random := make([]byte, 8<<10)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	long := hex.EncodeToString(random)
	w.result(transmission, workflow, version, message, base, `["derived","payload"]`,
		untargeted(fieldsOf("incident", entry("name", "string", `"`+long+`"`))))

	other := make([]byte, 8<<10)
	if _, err := rand.Read(other); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		predicate string
		want      int
	}{
		"eq":       {compare("eq", "attributes.incident.name", "string", `"`+long+`"`), 1},
		"in":       {member("attributes.incident.name", "string", `"`+hex.EncodeToString(other)+`"`, `"`+long+`"`), 1},
		"other eq": {compare("eq", "attributes.incident.name", "string", `"`+hex.EncodeToString(other)+`"`), 0},
		"other in": {member("attributes.incident.name", "string", `"`+hex.EncodeToString(other)+`"`), 0},
	} {
		result := w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{Selection: compile(t, test.predicate),
			DerivedGroups: []string{"derived", "payload"}})
		if len(result.Items) != test.want {
			t.Errorf("%s: %d rows, want %d", name, len(result.Items), test.want)
		}
	}
}

// TestProjectionAcceptsBoundaryNumbers expects the smallest and largest finite numbers and the
// largest timestamp offsets that typed values accept to be projected, so that publication never fails on a value evaluation accepted.
func TestProjectionAcceptsBoundaryNumbers(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	transmission := w.assigned(message, network, unit, agency, base, "")
	workflow, version := w.workflow()
	result := w.result(transmission, workflow, version, message, base, `["derived"]`,
		untargeted(fieldsOf("incident", entry("tiny", "number", `1e-310`), entry("huge", "number", `1.7e308`),
			entry("east", "timestamp", `"2026-01-01T12:00:00+15:59"`), entry("west", "timestamp", `"2026-01-01T12:00:00-15:59"`))))
	if got := w.projectedRows(result); len(got) != 4 {
		t.Fatalf("projected rows = %v, want four", got)
	}
}
