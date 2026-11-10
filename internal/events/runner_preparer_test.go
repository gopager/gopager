package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// preparingConsumer is a fakeConsumer that counts Prepare calls and can fail them.
type preparingConsumer struct {
	fakeConsumer
	mu       sync.Mutex
	prepared int
	failing  bool
}

func (c *preparingConsumer) Prepare(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prepared++
	if c.failing {
		return errors.New("prepare failed")
	}
	return nil
}

func (c *preparingConsumer) state() (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prepared, c.failing
}

func (c *preparingConsumer) setFailing(failing bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failing = failing
}

// TestRunnerPreparesBeforeEachBatchAndSkipsTheBatchWhenPrepareFails proves that a consumer's
// Prepare runs before every batch, and that a failing Prepare leaves the offset untouched until a
// later tick succeeds.
func TestRunnerPreparesBeforeEachBatchAndSkipsTheBatchWhenPrepareFails(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	world.add(1, "known", 1)
	consumer := &preparingConsumer{failing: true}
	wakes := newManualWakes()
	startRunner(t, world, consumer, wakes, time.Hour)
	eventually(t, "failed prepare", func() bool { prepared, _ := consumer.state(); return prepared >= 1 })
	if world.storedOffset().Seq != 0 || len(consumer.handledSeqs()) != 0 {
		t.Fatal("a batch ran although Prepare failed")
	}
	consumer.setFailing(false)
	wakes.wake()
	eventually(t, "offset at 1", func() bool { return world.storedOffset().Seq == 1 })
	if prepared, _ := consumer.state(); prepared < 2 {
		t.Fatalf("Prepare ran %d times, want at least 2", prepared)
	}
}
