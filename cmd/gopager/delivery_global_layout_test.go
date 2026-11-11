package main

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
)

// TestGlobalSubscriptionLayoutMustBeUsableByTheAuthor proves that the Layout of a global
// Subscription is checked against the rights of the author, not of the technical subscriber: a
// Layout of an Agency in which the author holds neither presentation.manage nor message.read is
// refused, and one of an Agency where the author reads Messages is accepted.
func TestGlobalSubscriptionLayoutMustBeUsableByTheAuthor(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	_, technical := f.subscriber("payload", "unit")
	foreign := seedAgency(t, f.store)
	foreignLayout := f.layoutCreated(f.layoutManager(), withAgencies(layoutBody("text", []any{textBlock("x")}), foreign))
	ownLayout := f.layoutCreated(f.layoutManager(), layoutBody("text", []any{textBlock("y")}))
	author, principal := f.caller([]string{"subscription.manage-global", "message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	// The author reads Messages of the foreign Agency only, so the Layout of the own Agency is
	// the one outside the author's rights until the grant for it is added.
	f.exec(`UPDATE identity_grants SET actions = '["subscription.manage-global"]'::jsonb WHERE subject_id = $1`, principal)
	f.subGrant(principal, "message.read", authz.Scope{Kind: "agency", References: []string{foreign.String()}})
	global := func(layout uuid.UUID) map[string]any {
		return subInput(instance, map[string]any{"kind": "global", "subscriberId": technical, "layoutId": layout})
	}

	f.subRejected(author, global(ownLayout.ID), http.StatusBadRequest, "INVALID_INPUT")
	created := f.subCreated(author, global(foreignLayout.ID))
	if created.Kind != delivery.KindGlobal || created.LayoutID == nil || *created.LayoutID != foreignLayout.ID {
		t.Fatalf("created = %+v, want a global Subscription with the foreign Agency's Layout", created)
	}

	f.subGrant(principal, "message.read", authz.Scope{Kind: "agency", References: []string{f.agency.String()}})
	f.subCreated(author, global(ownLayout.ID))
}
