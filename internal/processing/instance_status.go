package processing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Failure codes of PluginStatus.LastFailureCode.
const (
	// StatusStartFailed means the process did not complete its handshake.
	StatusStartFailed = "start-failed"
	// StatusChecksumMismatch means the executable differs from its pinned digest.
	StatusChecksumMismatch = "checksum-mismatch"
	// StatusInvocationFailed means a started plugin failed an invocation.
	StatusInvocationFailed = "invocation-failed"
	// StatusDeadlineExceeded means an invocation outran its deadline and the process was killed.
	StatusDeadlineExceeded = "deadline-exceeded"
)

// maxStatusInstances bounds the status table; it is cleared when exceeded because status is
// diagnostic and rebuilds from the next invocations.
const maxStatusInstances = 10000

// PluginStatus is what this node knows about the plugin processes of one instance. It is not
// shared between nodes and resets with the process.
type PluginStatus struct {
	Running         bool
	LastStartedAt   *time.Time
	LastFailureAt   *time.Time
	LastFailureCode string
}

// statusRecord is the host's memory of an instance's last start and failure.
type statusRecord struct {
	startedAt   time.Time
	failedAt    time.Time
	failureCode string
}

// recordStarted notes a successful process start of an instance.
func (h *PluginHost) recordStarted(instance uuid.UUID) {
	h.record(instance, func(r *statusRecord) { r.startedAt = time.Now() })
}

// recordFailure notes a failure of an instance's plugin; an empty code is ignored.
func (h *PluginHost) recordFailure(instance uuid.UUID, code string) {
	if code == "" {
		return
	}
	h.record(instance, func(r *statusRecord) { r.failedAt, r.failureCode = time.Now(), code })
}

func (h *PluginHost) record(instance uuid.UUID, update func(*statusRecord)) {
	if h == nil || instance == uuid.Nil {
		return
	}
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	record := h.statuses[instance]
	if record == nil {
		if h.statuses == nil || len(h.statuses) >= maxStatusInstances {
			h.statuses = make(map[uuid.UUID]*statusRecord)
		}
		record = &statusRecord{}
		h.statuses[instance] = record
	}
	update(record)
}

// Status aggregates the instance's processes on this node: running when any process is alive,
// with the last start and the last failure of any of them.
func (h *PluginHost) Status(instance uuid.UUID) PluginStatus {
	var status PluginStatus
	if h == nil {
		return status
	}
	h.mu.Lock()
	for key, process := range h.processes {
		if key.instance == instance && process.running() {
			status.Running = true
			break
		}
	}
	h.mu.Unlock()
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	record := h.statuses[instance]
	if record == nil {
		return status
	}
	if !record.startedAt.IsZero() {
		started := record.startedAt
		status.LastStartedAt = &started
	}
	if record.failureCode != "" {
		failed := record.failedAt
		status.LastFailureAt, status.LastFailureCode = &failed, record.failureCode
	}
	return status
}

// invocationFailureCode classifies a failed invocation of a started plugin; caller cancellation is
// not the plugin's failure and has no code.
func invocationFailureCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errInvocationDeadline):
		return StatusDeadlineExceeded
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ""
	}
	return StatusInvocationFailed
}
