package processing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func executionManagerWithRun() (*InstanceExecutions, *InstanceExecution) {
	manager := &InstanceExecutions{
		runs:    make(map[*InstanceExecution]map[uuid.UUID]int64),
		changed: make(chan struct{}),
	}
	runContext, cancel := context.WithCancel(context.Background())
	run := &InstanceExecution{
		manager:  manager,
		ctx:      runContext,
		cancel:   cancel,
		prepared: make(map[uuid.UUID]preparedInstance),
	}
	manager.runs[run] = make(map[uuid.UUID]int64)
	return manager, run
}

func TestInstanceExecutionsCloseWaitsForRunCleanup(t *testing.T) {
	manager, run := executionManagerWithRun()
	result := make(chan error, 1)
	go func() {
		result <- manager.Close(context.Background())
	}()

	select {
	case <-run.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("manager close did not cancel the active run")
	}
	select {
	case err := <-result:
		t.Fatalf("manager close returned before run cleanup: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	if err := run.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("manager close did not finish after run cleanup")
	}
}

func TestInstanceExecutionsCloseHonorsContextDeadline(t *testing.T) {
	manager, run := executionManagerWithRun()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := manager.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("manager close error = %v, want deadline exceeded", err)
	}
	if err := run.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInstanceExecutionsCloseReturnsRunCleanupFailure(t *testing.T) {
	manager, run := executionManagerWithRun()
	run.prepared[uuid.New()] = preparedInstance{main: nil}
	result := make(chan error, 1)
	go func() {
		result <- manager.Close(context.Background())
	}()
	<-run.Context().Done()
	if err := run.Close(context.Background()); err == nil {
		t.Fatal("run cleanup failure was not returned")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("manager close discarded run cleanup failure")
		}
	case <-time.After(time.Second):
		t.Fatal("manager close did not finish after run cleanup")
	}
}
