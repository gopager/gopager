package worker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestDispatcherFailedCandidatesDoNotStarveLaterWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(7)
		f.failure = errors.New("fixture owner unavailable")
		dispatcher := newTestDispatcher(t, f)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		for range 4 {
			time.Sleep(DefaultDispatchPollInterval)
			synctest.Wait()
		}
		f.mu.Lock()
		seen := make(map[string]bool)
		for _, id := range f.starts {
			seen[id.String()] = true
		}
		f.mu.Unlock()
		if len(seen) != 7 {
			t.Fatalf("rejected early candidates starved later work: distinct=%d", len(seen))
		}
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
