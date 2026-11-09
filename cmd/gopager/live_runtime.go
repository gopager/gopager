package main

import (
	"net/http"
	"time"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/readmodel"
)

// liveConfig is what the Message live stream needs from the process.
type liveConfig struct {
	// wake delivers the signal that new facts may be available; the process-wide hub.
	wake events.WakeSubscriber
	// replayWindow is how old a position may be when a stream resumes from it.
	replayWindow time.Duration
	// timing overrides the stream heartbeat, batch and write timeouts (zero keeps the default).
	timing struct{ heartbeat, batch, write time.Duration }
}

// registerLiveRoutes composes the Message live stream on the read service and the fact log and
// binds its route. Without a live configuration (no event runtime) no stream route exists.
func registerLiveRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	if c.live == nil {
		return nil
	}
	side, err := composeReadSide(c)
	if err != nil {
		return err
	}
	live, err := readmodel.NewLive(side.service, c.facts, c.reads, c.live.replayWindow)
	if err != nil {
		return err
	}
	timing := c.live.timing
	httpapi.RegisterMessageStreamRoutes(mux, transport, authenticator, httpapi.LiveSessions{Live: live}, c.live.wake,
		httpapi.WithStreamTiming(timing.heartbeat, timing.batch, timing.write), httpapi.WithStreamAdmission(side.admission))
	return nil
}
