package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestRangePrefilterKeepsMessagesWhoseBoundsAreMetByDifferentCandidates pins the meaning of an
// `and` of ordered comparisons on one field: each leaf asks for some candidate, not for the same
// one. A scalar field has several candidates when a Message has an untargeted and a
// target-scoped value, a list field has one per element, so a range is not a single-row
// condition on the projection. SQL and the Go evaluator must agree on all of these.
func TestRangePrefilterKeepsMessagesWhoseBoundsAreMetByDifferentCandidates(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	workflow, version := w.workflow()
	const groups = `["derived","payload"]`
	labels := map[uuid.UUID]string{}
	build := func(label string, at time.Duration) (message, transmission uuid.UUID) {
		message = w.message(base.Add(at), "text", label)
		labels[message] = label
		return message, w.assigned(message, network, unit, agency, base.Add(at), `{"form":"text","value":"`+label+`"}`)
	}
	inside, t1 := build("inside", 0)
	w.result(t1, workflow, version, inside, base.Add(time.Minute), groups, untargeted(fieldsOf("incident",
		entry("level", "integer", `"55"`), entry("levels", "list", `[`+value("integer", `"55"`)+`]`))))
	apart, t2 := build("apart", time.Minute)
	w.result(t2, workflow, version, apart, base.Add(time.Minute), groups, untargeted(fieldsOf("incident",
		entry("level", "integer", `"70"`), entry("levels", "list", `[`+value("integer", `"40"`)+`,`+value("integer", `"70"`)+`]`))))
	w.result(t2, workflow, version, apart, base.Add(2*time.Minute), groups, targeted(0, fieldsOf("incident",
		entry("level", "integer", `"40"`))))
	outside, t3 := build("outside", 2*time.Minute)
	w.result(t3, workflow, version, outside, base.Add(time.Minute), groups, untargeted(fieldsOf("incident",
		entry("level", "integer", `"80"`), entry("levels", "list", `[`+value("integer", `"80"`)+`,`+value("integer", `"90"`)+`]`))))

	level, levels := "attributes.incident.level", "attributes.incident.levels"
	cases := []struct {
		name      string
		predicate string
		want      []string
	}{
		{"scalar range, one candidate inside, two candidates around it", logical("and", compare("gte", level, "integer", `"50"`), compare("lt", level, "integer", `"60"`)),
			[]string{"apart", "inside"}},
		{"scalar bounds met by different candidates", logical("and", compare("gte", level, "integer", `"60"`), compare("lt", level, "integer", `"50"`)),
			[]string{"apart"}},
		{"scalar range nobody meets", logical("and", compare("gt", level, "integer", `"85"`), compare("lt", level, "integer", `"88"`)),
			[]string{}},
		{"list range, elements on both sides of it", logical("and", compare("gte", levels, "integer", `"50"`), compare("lt", levels, "integer", `"60"`)),
			[]string{"apart", "inside"}},
		{"list range met by different elements", logical("and", compare("gt", levels, "integer", `"85"`), compare("lt", levels, "integer", `"88"`)),
			[]string{"outside"}},
	}
	caller := all()
	now := base.Add(time.Hour)
	allGroups := []string{"derived", "payload"}
	everything := w.feed(caller, now, readmodel.FeedQuery{DerivedGroups: allGroups})
	if len(everything.Items) != 3 {
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
		if !slices.Equal(sql, test.want) {
			t.Errorf("%s: %v, want %v", test.name, sql, test.want)
		}
	}
}
