package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// lifecycleScene is a scene with an instance of the supplied Delivery test plugin whose behaviour
// the Message text selects: the layout renders the text alone, so a Message "hang" makes the plugin
// block and a Message "echo-request" makes it report the revision it served.
type lifecycleScene struct {
	*pluginDeliveryScene
	artifact pluginArtifact
	instance uuid.UUID
}

// newLifecycleScene provisions the fixture plugin, creates an instance with a stored token and
// subscribes a subscriber with the text-only layout.
func newLifecycleScene(t *testing.T) *lifecycleScene {
	t.Helper()
	s := &lifecycleScene{pluginDeliveryScene: newPluginDeliveryScene(t)}
	s.artifact = provisionFixture(t, s.directory)
	s.instance = s.createInstance(s.artifact, lifecycleConfiguration("Title"), map[string]any{"apiToken": plainSecret("token-1")})
	bearer, _ := s.subscriber("payload", "unit")
	layout := s.layoutCreated(s.layoutManager(), layoutBody("text", []any{map[string]any{"kind": "field",
		"field": map[string]any{"namespace": "message", "path": []string{"text"}}}}))
	s.subscribeTarget(bearer, s.instance, map[string]any{"layoutId": layout.ID})
	return s
}

// lifecycleConfiguration is the administrative configuration of the fixture instance.
func lifecycleConfiguration(title string) map[string]any {
	return map[string]any{"title": typedString(title)}
}

// startAttempt runs one runner tick in the background and returns the channel that reports its end.
func (s *lifecycleScene) startAttempt() <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := s.runner.Tick(s.t.Context())
		done <- err
	}()
	return done
}

// awaitRunning waits until the plugin process of the instance is alive and the Delivery's attempt
// is recorded as started, which means the call is in flight.
func (s *lifecycleScene) awaitRunning(id uuid.UUID) {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		view := s.adminRequest(http.MethodGet, "/api/v1/instances/"+s.instance.String(), "", http.StatusOK)
		var decoded struct {
			Status struct {
				Running bool `json:"running"`
			} `json:"status"`
		}
		if err := json.Unmarshal(view.Data, &decoded); err != nil {
			s.t.Fatal(err)
		}
		if decoded.Status.Running && s.attemptStates(id) == "1:started:-:1" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("the plugin call never started: attempts %q, view %s", s.attemptStates(id),
		s.adminRequest(http.MethodGet, "/api/v1/instances/"+s.instance.String(), "", http.StatusOK).Data)
}

// awaitEnd waits for a background tick to return.
func (s *lifecycleScene) awaitEnd(done <-chan error) {
	s.t.Helper()
	select {
	case err := <-done:
		if err != nil {
			s.t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		s.t.Fatal("the runner tick did not end")
	}
}

// TestInstanceUpdateEndsTheRunningAttemptAndTheNextDeliveryUsesTheNewRevision proves that saving a
// new revision while a call is in flight ends that call, which is recorded uncertain, starts nothing
// of the old revision again, and that the next Delivery is served by the new revision.
func TestInstanceUpdateEndsTheRunningAttemptAndTheNextDeliveryUsesTheNewRevision(t *testing.T) {
	t.Parallel()
	s := newLifecycleScene(t)
	hung := s.deliverMessage("hang")[0]
	done := s.startAttempt()
	s.awaitRunning(hung)

	s.updateInstance(s.instance, s.artifact, "1", true, lifecycleConfiguration("Renamed"), everyone)
	s.awaitEnd(done)
	if got := s.status(hung); got != "uncertain/uncertain/1" {
		t.Fatalf("running Delivery = %s", got)
	}
	s.attempt()
	if got := s.attemptStates(hung); got != "1:uncertain:uncertain:1" {
		t.Fatalf("old revision was started again: %s", got)
	}

	next := s.deliverMessage("echo-request")[0]
	s.attempt()
	if got := s.status(next); got != "accepted/accepted/1" {
		t.Fatalf("next Delivery = %s", got)
	}
	if got := s.attemptStates(next); got != "1:accepted:accepted:2" {
		t.Fatalf("next attempt = %s", got)
	}
	if got := s.text(`SELECT receipt->>'instanceRevision' FROM delivery_attempts WHERE delivery_id = $1`, next); got != "2" {
		t.Fatalf("the plugin served revision %s", got)
	}
}

// TestDisabledInstanceStartsNothingAndEndsTheRunningAttemptHonestly proves that disabling an
// instance records the in-flight attempt as uncertain, defers later Deliveries as
// dependency-unavailable without consuming an attempt, and that enabling it again lets them run.
func TestDisabledInstanceStartsNothingAndEndsTheRunningAttemptHonestly(t *testing.T) {
	t.Parallel()
	s := newLifecycleScene(t)
	hung := s.deliverMessage("hang")[0]
	done := s.startAttempt()
	s.awaitRunning(hung)

	s.updateInstance(s.instance, s.artifact, "1", false, lifecycleConfiguration("Title"), everyone)
	s.awaitEnd(done)
	if got := s.status(hung); got != "uncertain/uncertain/1" {
		t.Fatalf("running Delivery = %s", got)
	}

	waiting := s.deliverMessage("echo-request")[0]
	s.overdue(waiting, delivery.PlacementGrace)
	s.attempt()
	if got := s.status(waiting); got != "prepared/"+delivery.CodeDependency+"/0" {
		t.Fatalf("Delivery of a disabled instance = %s", got)
	}
	if got := s.attemptStates(waiting); got != "" {
		t.Fatalf("a disabled instance was started: %s", got)
	}

	s.updateInstance(s.instance, s.artifact, "2", true, lifecycleConfiguration("Title"), everyone)
	s.overdue(waiting, delivery.PlacementGrace)
	s.attempt()
	if got := s.status(waiting); got != "accepted/accepted/1" {
		t.Fatalf("Delivery after enabling = %s", got)
	}
}
