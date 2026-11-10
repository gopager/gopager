package readstore_test

import (
	"context"
	"errors"
	"github.com/gopager/gopager/internal/platform"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestSuppressedAndHiddenTargetsStayOutOfTheProjection checks: a Message with a
// permitted and a suppressed target shows the permitted one only.
func TestSuppressedAndHiddenTargetsStayOutOfTheProjection(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, c := w.unit(agency, "A", nil), w.unit(agency, "C", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	permitted := w.assigned(message, network, a, agency, base, "")
	hidden := w.transmission(transmissionSpec{message: message, network: network, accepted: base.Add(time.Second)})
	w.target(hidden, 0, targetSpec{unit: &c, address: 2000})
	w.target(permitted, 1, targetSpec{unit: &a, address: 1001})
	w.suppress(permitted, 1)
	caller := principal(grant(scope("unit", a), nil, "message.read", "message.detail"))
	detail, err := w.detail(caller, base.Add(time.Hour), message)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(unitNames(detail.FeedRecord), []string{"A"}) || len(detail.WithoutUnit) != 0 {
		t.Fatalf("units = %+v", detail.Units)
	}
	if len(detail.Transmissions) != 1 || detail.Transmissions[0].ID != permitted || len(detail.Transmissions[0].Targets) != 1 ||
		detail.Transmissions[0].Targets[0].Position != 0 {
		t.Fatalf("transmissions = %+v", detail.Transmissions)
	}
}

// TestMessageWithOnlyHiddenTargetsIsAbsent checks for feed, eligibility and detail:
func TestMessageWithOnlyHiddenTargetsIsAbsent(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	transmission := w.transmission(transmissionSpec{message: message, network: network, accepted: base})
	w.target(transmission, 0, targetSpec{unit: &unit, address: 1000})
	w.suppress(transmission, 0)
	// A suppressed target produces no appearance, so the cache holds no row.
	caller := all()
	now := base.Add(time.Hour)
	if got := w.feed(caller, now, readmodel.FeedQuery{}); len(got.Items) != 0 {
		t.Fatalf("feed = %v", ids(got))
	}
	if _, err := w.detail(caller, now, message); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("detail err = %v", err)
	}
	access, _ := authz.NewMessageAccess(caller, now)
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		found, err := w.reads.Eligible(ctx, tx, access, []uuid.UUID{message})
		if err != nil || len(found) != 0 {
			t.Errorf("Eligible = %v, %v", found, err)
		}
	})
}

// TestHiddenMessagesNeverConsumePageSlots checks authorization before pagination: with the
// newest messages hidden, the visible ones still fill whole pages.
func TestHiddenMessagesNeverConsumePageSlots(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	visible, hidden := w.unit(agency, "visible", nil), w.unit(agency, "hidden", nil)
	network := w.network(agency, "N", "pocsag")
	var want []uuid.UUID
	for i := range 5 {
		m := w.message(base.Add(time.Duration(i)*time.Minute), "text", "v")
		w.assigned(m, network, visible, agency, base.Add(time.Duration(i)*time.Minute), "")
		want = append([]uuid.UUID{m}, want...)
	}
	for i := 5; i < 12; i++ {
		m := w.message(base.Add(time.Duration(i)*time.Minute), "text", "h")
		w.assigned(m, network, hidden, agency, base.Add(time.Duration(i)*time.Minute), "")
	}
	caller := principal(grant(scope("unit", visible), nil, "message.read"))
	now := base.Add(time.Hour)
	first := w.feed(caller, now, readmodel.FeedQuery{Limit: 2})
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first page = %v more=%v", ids(first), first.HasMore)
	}
	last := first.Items[1]
	second := w.feed(caller, now, readmodel.FeedQuery{Limit: 2, After: &readmodel.Cursor{Time: last.Time, ID: last.MessageID}})
	last = second.Items[1]
	third := w.feed(caller, now, readmodel.FeedQuery{Limit: 2, After: &readmodel.Cursor{Time: last.Time, ID: last.MessageID}})
	got := slices.Concat(ids(first), ids(second), ids(third))
	if !slices.Equal(got, want) || third.HasMore || len(third.Items) != 1 {
		t.Fatalf("pages = %v, want %v", got, want)
	}
}

// TestUnitlessRowsNeedWithoutUnitRead covers the without-unit.read requirement.
func TestUnitlessRowsNeedWithoutUnitRead(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	w.transmission(transmissionSpec{message: message, network: network, accepted: base})
	w.appearance(message, nil, agency, base, base)
	now := base.Add(time.Hour)
	readOnly := principal(grant(scope("agency", agency), nil, "message.read"))
	if got := w.feed(readOnly, now, readmodel.FeedQuery{}); len(got.Items) != 0 {
		t.Fatalf("message.read alone must not show Unit-less traffic: %v", ids(got))
	}
	withIt := principal(grant(scope("agency", agency), nil, "message.read", "without-unit.read"))
	got := w.feed(withIt, now, readmodel.FeedQuery{})
	if len(got.Items) != 1 || len(got.Items[0].WithoutUnit) != 1 || got.Items[0].WithoutUnit[0].AgencyID != agency {
		t.Fatalf("Unit-less row = %+v", got.Items)
	}
	ownAge := principal(grant(scope("agency", agency), nil, "message.read"), grant(scope("agency", agency), seconds(1), "without-unit.read"))
	if got := w.feed(ownAge, now, readmodel.FeedQuery{}); len(got.Items) != 0 {
		t.Fatalf("without-unit.read has its own age: %v", ids(got))
	}
}

// TestDetailRequiresDetailRightOnTheSameAppearance checks message.detail.
func TestDetailRequiresDetailRightOnTheSameAppearance(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	w.assigned(message, network, a, agency, base, "")
	w.assigned(message, network, b, agency, base, "")
	now := base.Add(time.Hour)
	// detail on B only: the detail shows B alone, though the feed shows both.
	caller := principal(grant(scope("all"), nil, "message.read"), grant(scope("unit", b), nil, "message.detail"))
	if got := w.feed(caller, now, readmodel.FeedQuery{}).Items[0]; len(got.Units) != 2 {
		t.Fatalf("feed units = %d", len(got.Units))
	}
	detail, err := w.detail(caller, now, message)
	if err != nil || !slices.Equal(unitNames(detail.FeedRecord), []string{"B"}) || len(detail.Transmissions) != 1 {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
	readOnly := principal(grant(scope("all"), nil, "message.read"))
	if _, err := w.detail(readOnly, now, message); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("detail without message.detail: %v", err)
	}
}
