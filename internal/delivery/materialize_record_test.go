package delivery

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestUnavailableTargetAtFanOutStaysDueNow checks that a Delivery whose target the fan-out node
// cannot describe is recorded as dependency-unavailable but due at once, so a node that can serve
// the target is not held back by the node that happened to run the fan-out.
func TestUnavailableTargetAtFanOutStaysDueNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	subscription := Subscription{ID: uuid.New(), Revision: 1, SubscriberID: uuid.New()}
	current := &subject{event: eventView{ID: uuid.New(), Type: "message.created"}}
	target := TargetReference{InstanceID: uuid.New()}
	record := newRecord(subscription, current, split{}, target, rendering{instanceRevision: 1}, now, now.Add(time.Hour))
	if record.State != Prepared || record.OutcomeCode == nil || *record.OutcomeCode != CodeDependency ||
		!record.Due.Equal(now) || record.Input != nil {
		t.Fatalf("got state %s code %v due %s input %v", record.State, record.OutcomeCode, record.Due, record.Input)
	}
}
