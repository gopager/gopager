package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/authz"
)

// TestMessageCardsFollowTheLayoutLayers proves that a feed card renders the Layout assigned to
// the Message's Unit, and that Units with different Layouts fall back to the installation
// default with the conflict reported.
func TestMessageCardsFollowTheLayoutLayers(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	engine, ladder := f.unit("Engine 1", nil), f.unit("Ladder 1", nil)
	f.message(time.Now().UTC().Add(-2*time.Hour).Truncate(time.Microsecond), "FIRE", engine)
	assigned, installed := f.textLayoutRow("json", f.agency), f.textLayoutRow("json", f.agency)
	f.exec(`INSERT INTO layout_assignments (scope_kind, scope_id, output_kind, layout_id, revision, updated_at)
		VALUES ('unit', $1, 'json', $2, 1, now())`, engine, assigned)
	f.exec(`UPDATE installation_settings SET default_layout_id = $1`, installed)
	reader, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})

	card := cardOf(t, f.feed(reader, "", http.StatusOK).Data.Items[0])
	if card["layoutId"] != assigned.String() || card["layoutConflict"] != false || card["fallback"] != false {
		t.Fatalf("card = %v, want the Unit's Layout %s", card, assigned)
	}

	f.exec(`INSERT INTO layout_assignments (scope_kind, scope_id, output_kind, layout_id, revision, updated_at)
		VALUES ('unit', $1, 'json', $2, 1, now())`, ladder, installed)
	f.message(time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond), "FLOOD", engine, ladder)
	card = cardOf(t, f.feed(reader, "", http.StatusOK).Data.Items[0])
	if card["layoutId"] != installed.String() || card["layoutConflict"] != true {
		t.Fatalf("card = %v, want the installation default %s with a conflict", card, installed)
	}
}
