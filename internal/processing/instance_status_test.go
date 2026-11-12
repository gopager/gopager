package processing

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestStatusDoesNotHoldTheHostWhileAProcessStarts checks that reading the status of an instance
// whose plugin is still in its handshake (its process lock held) does not keep the host lock, so
// other instances' status reads and process acquisition continue meanwhile.
func TestStatusDoesNotHoldTheHostWhileAProcessStarts(t *testing.T) {
	t.Parallel()
	host := NewPluginHost()
	starting, other := uuid.New(), uuid.New()
	process := &pluginProcess{host: host, key: processKey{instance: starting, revision: 1}, pooled: true,
		slot: make(chan struct{}, 1), refs: 1}
	host.processes[process.key] = process

	process.mu.Lock() // the handshake holds the process lock
	slow := make(chan PluginStatus, 1)
	go func() { slow <- host.Status(starting) }()

	done := make(chan PluginStatus, 1)
	go func() {
		time.Sleep(50 * time.Millisecond) // let the first read reach the process lock
		done <- host.Status(other)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		process.mu.Unlock()
		t.Fatal("another instance's status read waited for a starting process")
	}
	process.mu.Unlock()
	if status := <-slow; status.Running {
		t.Fatalf("a process that has not started reports running: %+v", status)
	}
}
