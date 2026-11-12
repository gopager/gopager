package delivery

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// instanceStub serves one fixed instance.
type instanceStub struct {
	instance processing.InstanceConfiguration
}

func (s instanceStub) CurrentInstance(context.Context, platform.TransactionContext, uuid.UUID) (processing.InstanceConfiguration, error) {
	return s.instance, nil
}

// TestDisabledInstanceDeniesNonAdmittedSubscriber checks that a subscriber the instance does not
// admit never gets a Delivery for it, also while the instance is disabled.
func TestDisabledInstanceDeniesNonAdmittedSubscriber(t *testing.T) {
	t.Parallel()
	admitted, stranger := uuid.New(), uuid.New()
	instance := processing.InstanceConfiguration{ID: uuid.New(), Revision: 3, Enabled: false,
		TargetUse: &processing.TargetUse{PrincipalIDs: []uuid.UUID{admitted}}}
	deps := Dependencies{Instances: instanceStub{instance}}
	target := TargetReference{InstanceID: instance.ID}

	denied, err := render(t.Context(), nil, deps, Subscription{SubscriberID: stranger}, target, nil)
	if err != nil || !denied.denied {
		t.Fatalf("non-admitted subscriber: denied = %v, %v", denied.denied, err)
	}
	pending, err := render(t.Context(), nil, deps, Subscription{SubscriberID: admitted}, target, nil)
	if err != nil || pending.denied || pending.available {
		t.Fatalf("admitted subscriber: %+v, %v, want a pending dependency", pending, err)
	}
}
