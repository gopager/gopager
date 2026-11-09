package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestTextSearchIsLiteralUnicodeCaseFoldedSubstring checks:
func TestTextSearchIsLiteralUnicodeCaseFoldedSubstring(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	messages := map[string]uuid.UUID{}
	i := 0
	add := func(key, form, value string) {
		i++
		m := w.message(base.Add(time.Duration(i)*time.Second), form, value)
		w.assigned(m, network, unit, agency, base, "")
		messages[key] = m
	}
	add("strasse-sharp", "text", "Alarm Straße 5")
	add("strasse-caps", "text", "ALARM STRASSE 7")
	add("composed", "text", "café")
	add("decomposed", "text", "café nord")
	add("plain-e", "text", "cafe south")
	add("percent", "text", "100% sure")
	add("underscore", "text", "a_b")
	add("dot-star", "text", "x.*y")
	add("plain", "text", "axxb")
	add("numeric", "numeric", "112 (45)")
	add("binary", "binary", "\xff\xfe\x00")
	add("tone", "tone-only", "")
	caller := all()
	now := base.Add(time.Hour)
	search := func(term string) []string {
		var keys []string
		for _, item := range w.filtered(caller, now, readmodel.Filters{Text: term}).Items {
			for key, id := range messages {
				if id == item.MessageID {
					keys = append(keys, key)
				}
			}
		}
		slices.Sort(keys)
		return keys
	}
	for term, want := range map[string][]string{
		"STRASSE":      {"strasse-caps", "strasse-sharp"},
		"straße":       {"strasse-caps", "strasse-sharp"},
		"strasse 5":    {"strasse-sharp"},
		"café":         {"composed", "decomposed"},
		"café":        {"composed", "decomposed"},
		"cafe":         {"plain-e"},
		"é":            {"composed", "decomposed"},
		"e":            {"percent", "plain-e", "strasse-caps", "strasse-sharp"},
		"%":            {"percent"},
		"_":            {"underscore"},
		".*":           {"dot-star"},
		"a_b":          {"underscore"},
		"112 (45)":     {"numeric"},
		"ALARM":        {"strasse-caps", "strasse-sharp"},
		"does not hit": nil,
	} {
		got := search(term)
		if !slices.Equal(got, want) {
			t.Errorf("term %q = %v, want %v", term, got, want)
		}
	}
	// "é" must not match a message with only a plain "e"; accents stay distinct.
	if slices.Contains(search("é"), "plain-e") {
		t.Error("é matched e")
	}
	if got := len(w.filtered(caller, now, readmodel.Filters{}).Items); got != 12 {
		t.Errorf("empty text is no filter: %d of 12", got)
	}
}
