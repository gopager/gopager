package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestRestrictingKeyIntersectsActionScopeAndAge checks: Unit A is old (30 s grant),
// Unit B recent with an unlimited grant; a key narrowed to B returns B only and the older
// Transmission of A never represents the Message.
func TestRestrictingKeyIntersectsActionScopeAndAge(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(at(12, 0, 0), "text", "alarm")
	w.assigned(message, network, a, agency, at(12, 0, 0), `{"form":"text","value":"A text"}`)
	w.assigned(message, network, b, agency, at(12, 0, 55), `{"form":"text","value":"B text"}`)
	now := at(12, 1, 0)
	owner := principal(grant(scope("unit", a), seconds(30), "message.read", "message.detail"),
		grant(scope("unit", b), nil, "message.read", "message.detail"))
	t.Run("owner alone", func(t *testing.T) {
		t.Parallel()
		item := w.feed(owner, now, readmodel.FeedQuery{}).Items[0]
		if !slices.Equal(unitNames(item), []string{"B"}) || string(item.Representative.Payload) != `{"form":"text","value":"B text"}` {
			t.Fatalf("item = %+v", item)
		}
	})
	t.Run("key narrowed to B", func(t *testing.T) {
		t.Parallel()
		keyed := owner
		keyed.Key = &authz.Restrictions{Actions: []string{"message.read", "message.detail"}, Scopes: []authz.Scope{scope("unit", b)}}
		item := w.feed(keyed, now, readmodel.FeedQuery{}).Items[0]
		if !slices.Equal(unitNames(item), []string{"B"}) {
			t.Fatalf("units = %v", unitNames(item))
		}
		detail, err := w.detail(keyed, now, message)
		if err != nil || len(detail.Transmissions) != 1 {
			t.Fatalf("detail = %+v, %v", detail, err)
		}
	})
	t.Run("key without message.detail", func(t *testing.T) {
		t.Parallel()
		keyed := owner
		keyed.Key = &authz.Restrictions{Actions: []string{"message.read"}, Scopes: []authz.Scope{scope("all")}}
		if got := w.feed(keyed, now, readmodel.FeedQuery{}); len(got.Items) != 1 {
			t.Fatalf("feed = %v", ids(got))
		}
		if _, err := w.detail(keyed, now, message); err == nil {
			t.Fatal("detail must need the key's message.detail action")
		}
	})
	t.Run("key age", func(t *testing.T) {
		t.Parallel()
		keyed := principal(grant(scope("all"), nil, "message.read"))
		keyed.Key = &authz.Restrictions{Actions: []string{"message.read"}, Scopes: []authz.Scope{scope("all")}, MaxAgeSeconds: seconds(30)}
		if got := w.feed(keyed, now, readmodel.FeedQuery{}).Items[0]; !slices.Equal(unitNames(got), []string{"B"}) {
			t.Fatalf("units = %v", unitNames(got))
		}
	})
	t.Run("key action missing", func(t *testing.T) {
		t.Parallel()
		keyed := owner
		keyed.Key = &authz.Restrictions{Actions: []string{"unit.read"}, Scopes: []authz.Scope{scope("all")}}
		access, err := authz.NewMessageAccess(keyed, now)
		if err != nil || access.Read.Granted() {
			t.Fatalf("a key without message.read grants nothing: %+v, %v", access.Read, err)
		}
	})
}

// TestUnitlessRowsUnderKeyAgencyScope checks that a key narrowed to a Unit scope cannot see
// Unit-less traffic, which needs an all or agency key scope.
func TestUnitlessRowsUnderKeyAgencyScope(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	w.transmission(transmissionSpec{message: message, network: network, accepted: base})
	w.appearance(message, nil, agency, base, base)
	owner := principal(grant(scope("all"), nil, "message.read", "without-unit.read"))
	now := base.Add(time.Hour)
	for name, test := range map[string]struct {
		key  authz.Restrictions
		want int
	}{
		"agency scope":           {authz.Restrictions{Actions: []string{"message.read", "without-unit.read"}, Scopes: []authz.Scope{scope("agency", agency)}}, 1},
		"unit scope":             {authz.Restrictions{Actions: []string{"message.read", "without-unit.read"}, Scopes: []authz.Scope{scope("unit", uuid.New())}}, 0},
		"no without-unit action": {authz.Restrictions{Actions: []string{"message.read"}, Scopes: []authz.Scope{scope("all")}}, 0},
	} {
		keyed := owner
		key := test.key
		keyed.Key = &key
		if got := len(w.feed(keyed, now, readmodel.FeedQuery{}).Items); got != test.want {
			t.Errorf("%s: %d rows, want %d", name, got, test.want)
		}
	}
}
