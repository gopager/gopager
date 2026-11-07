package ingest

import (
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

// TestNextGatewayState expects first contact to leave never-seen, heartbeat and Submission
// to recover only silent states, and explicit reports to set their state.
func TestNextGatewayState(t *testing.T) {
	t.Parallel()
	cases := []struct{ current, operation, want string }{
		{StateNeverSeen, "heartbeat", StateActive},
		{StateNeverSeen, "submission", StateActive},
		{StateNeverSeen, "degraded", StateDegraded},
		{StateNeverSeen, "terminated", StateStopped},
		{StateNeverSeen, "recovered", StateActive},
		{StateActive, "heartbeat", StateActive},
		{StateDegraded, "heartbeat", StateDegraded},
		{StateDegraded, "submission", StateDegraded},
		{StateStopped, "heartbeat", StateActive},
		{StateContactLost, "submission", StateActive},
		{StateContactLost, "degraded", StateDegraded},
		{StateDegraded, "recovered", StateActive},
		{StateActive, "terminated", StateStopped},
		{StateStopped, "unknown", StateStopped},
	}
	for _, c := range cases {
		if got := NextGatewayState(c.current, c.operation); got != c.want {
			t.Errorf("NextGatewayState(%s, %s)=%s want %s", c.current, c.operation, got, c.want)
		}
	}
}

// TestContactConfigValidate expects finite positive durations, a timeout of at least the
// heartbeat interval and a scan interval of at most the timeout.
func TestContactConfigValidate(t *testing.T) {
	t.Parallel()
	valid := ContactConfig{HeartbeatInterval: DefaultHeartbeatInterval, Timeout: DefaultContactTimeout,
		ScanInterval: DefaultContactScan}
	if err := valid.Validate(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	equal := ContactConfig{HeartbeatInterval: time.Minute, Timeout: time.Minute, ScanInterval: time.Minute}
	if err := equal.Validate(); err != nil {
		t.Fatalf("timeout equal to heartbeat and scan: %v", err)
	}
	for name, config := range map[string]ContactConfig{
		"timeout below heartbeat": {HeartbeatInterval: time.Minute, Timeout: 59 * time.Second, ScanInterval: time.Second},
		"zero heartbeat":          {Timeout: time.Minute, ScanInterval: time.Second},
		"zero scan":               {HeartbeatInterval: time.Second, Timeout: time.Minute},
		"negative timeout":        {HeartbeatInterval: time.Second, Timeout: -time.Minute, ScanInterval: time.Second},
		"scan above timeout":      {HeartbeatInterval: time.Second, Timeout: time.Minute, ScanInterval: 61 * time.Second},
	} {
		if err := config.Validate(); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: %v, want INVALID_INPUT", name, err)
		}
	}
}
