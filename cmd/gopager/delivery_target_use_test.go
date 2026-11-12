package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// instanceWith returns an enabled, unsaved Delivery instance with the given target use.
func (f *deliveryFixture) instanceWith(use *processing.TargetUse) processing.InstanceConfiguration {
	f.t.Helper()
	config, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		f.t.Fatal(err)
	}
	actor, _ := uuid.Parse(f.admin.PrincipalID)
	return processing.InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true, ExecutionPrincipalID: uuid.New(),
		TargetUse: use, CreatedBy: actor, Configuration: config, Main: processing.ProvisionedArtifact{
			ManifestPath: "target/manifest.json", Pin: processing.ArtifactPin{PluginID: "target", Version: "1",
				CapabilityID: "deliver", Digest: strings.Repeat("d", 64)}}}
}

// reviseTargetUse stores the next revision of an instance with another target use.
func (f *deliveryFixture) reviseTargetUse(value processing.InstanceConfiguration, use *processing.TargetUse) {
	f.t.Helper()
	value.TargetUse = use
	value.Revision++
	f.saveInstance(value, value.Revision-1)
}

// TestSubscriptionTargetUseAdmission proves a Subscription may select an instance only for a
// subscriber the target use admits, and that a refusal looks like an unavailable target.
func TestSubscriptionTargetUseAdmission(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, principal := f.subscriber("payload", "unit")
	for name, test := range map[string]struct {
		use      *processing.TargetUse
		accepted bool
	}{
		"everyone":     {&processing.TargetUse{Everyone: true}, true},
		"listed":       {&processing.TargetUse{PrincipalIDs: []uuid.UUID{principal}}, true},
		"other listed": {&processing.TargetUse{PrincipalIDs: []uuid.UUID{uuid.New()}}, false},
		"nobody":       {&processing.TargetUse{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			value := f.instanceWith(test.use)
			f.saveInstance(value, 0)
			if test.accepted {
				f.subCreated(bearer, subInput(value.ID, nil))
				return
			}
			response := f.subRejected(bearer, subInput(value.ID, nil), http.StatusServiceUnavailable, string(platform.CodeAuthorityUnavailable))
			if response.Parameters["status"] != "unavailable" {
				t.Fatalf("parameters = %v", response.Parameters)
			}
		})
	}
}

// TestFanOutSkipsSubscribersWithoutTargetUse proves that revoking the target use creates no
// Delivery for the subscriber on the next fact.
func TestFanOutSkipsSubscribersWithoutTargetUse(t *testing.T) {
	t.Parallel()
	s := newBareScene(t, delivery.AdmissionLimits{})
	value := s.instanceWith(&processing.TargetUse{PrincipalIDs: []uuid.UUID{s.subscriberID}})
	s.saveInstance(value, 0)
	s.instanceID = value.ID
	s.subscribeTo(s.subscriberBearer)
	s.reviseTargetUse(value, &processing.TargetUse{PrincipalIDs: []uuid.UUID{uuid.New()}})
	message := s.newMessage("revoked")
	s.messageFact("message.created", message, nil)
	s.fanOutAll()
	if got := s.count(`SELECT count(*) FROM deliveries WHERE message_id = $1`, message); got != 0 {
		t.Fatalf("%d Deliveries for a subscriber without target use", got)
	}
}

// TestPreparedDeliveryIsCancelledWhenTargetUseIsRevoked proves a prepared Delivery is cancelled
// with authority-revoked, never sent, once its subscriber loses the target use.
func TestPreparedDeliveryIsCancelledWhenTargetUseIsRevoked(t *testing.T) {
	t.Parallel()
	s := newBareScene(t, delivery.AdmissionLimits{})
	value := s.instanceWith(&processing.TargetUse{Everyone: true})
	s.saveInstance(value, 0)
	s.instanceID = value.ID
	s.subscribeTo(s.subscriberBearer)
	id := s.deliverOne("prepared")
	s.reviseTargetUse(value, &processing.TargetUse{})
	s.attempt()
	view := s.expectState(id, delivery.Cancelled, delivery.CodeAuthorityRevoked)
	if view.AttemptCount != 0 || len(s.targets.calls()) != 0 {
		t.Fatalf("cancelled Delivery %+v, %d calls", view, len(s.targets.calls()))
	}
}
