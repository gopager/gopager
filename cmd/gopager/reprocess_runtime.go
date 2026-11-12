package main

import (
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/reprocess"
)

// composeReprocessRunner binds the historical Reprocess runner to the Processing store and to the
// source owner that re-admits Transmissions, and wakes it when a request is made.
func composeReprocessRunner(c *core, readmission reprocess.Readmission) (notifiedDispatcher, error) {
	runner, err := reprocess.New(c.store, c.store, c.processing, readmission, c.facts)
	if err != nil {
		return notifiedDispatcher{}, err
	}
	return notifiedDispatcher{dispatcher: runner, listener: c.store, channel: processing.ReprocessNotificationChannel}, nil
}
