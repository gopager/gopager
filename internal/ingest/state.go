package ingest

import "time"

// Gateway states. NeverSeen is the state of a Gateway without any authenticated
// contact; Active and ContactLost are derived by the server, the other states
// follow the Gateway's own reports.
const (
	StateNeverSeen   = "never-seen"
	StateActive      = "active"
	StateDegraded    = "degraded"
	StateStopped     = "stopped"
	StateContactLost = "contact-lost"
)

// Causes of a Gateway state transition, as recorded in its history.
const (
	CauseReport     = "report"
	CauseSubmission = "submission"
	CauseTimeout    = "timeout"
)

// NextGatewayState returns the state after authenticated contact in state current.
// operation is a GatewayStateReport operation or "submission" for an accepted
// Submission. A heartbeat or Submission makes a never-seen, stopped or
// contact-lost Gateway active and leaves active or degraded unchanged;
// "degraded", "recovered" and "terminated" set degraded, active and stopped.
// An unknown operation leaves the state unchanged.
func NextGatewayState(current, operation string) string {
	switch operation {
	case "degraded":
		return StateDegraded
	case "recovered":
		return StateActive
	case "terminated":
		return StateStopped
	case "heartbeat", "submission":
		if current == StateNeverSeen || current == StateStopped || current == StateContactLost {
			return StateActive
		}
	}
	return current
}

// GatewayTransition is one recorded Gateway state change. Revision is the
// Gateway revision the change produced; ContactAt is the triggering contact,
// or for a timeout the last contact. ReasonCode and Detail are set only for a
// reported degradation.
type GatewayTransition struct {
	Revision   int64     `json:"revision,string"`
	OldState   string    `json:"oldState"`
	NewState   string    `json:"newState"`
	Cause      string    `json:"cause"`
	OccurredAt time.Time `json:"occurredAt"`
	ContactAt  time.Time `json:"contactAt"`
	ReasonCode string    `json:"reasonCode,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}
