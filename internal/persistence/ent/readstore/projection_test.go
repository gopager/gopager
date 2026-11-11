package readstore_test

import (
	"context"
	"database/sql"
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
		found, err := executor.QueryContext(ctx, `SELECT schema_key, path, target_position, text_value, integer_value,
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
		"incident/urgent=boolean:true", "incident/tags=text:x", "incident/tags=text:y",
		"incident/owner.name=text:Ada", "incident/owner.address.city=text:Bonn",
	}
	slices.Sort(want)
	if got := w.projectedRows(untargetedResult); !slices.Equal(got, want) {
		t.Errorf("untargeted rows:\n got %v\nwant %v", got, want)
	}
	targetedResult := w.result(transmission, workflow, version, message, base, `["derived"]`,
		targeted(2, fieldsOf("incident", entry("city", "string", `"Bonn"`))))
	if got, want := w.projectedRows(targetedResult), []string{"incident/city@2=text:Bonn"}; !slices.Equal(got, want) {
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
