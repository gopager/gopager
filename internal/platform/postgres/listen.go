package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gopager/gopager/internal/platform"
)

// listenApplicationName labels listener sessions so operators can identify them in
// pg_stat_activity.
const listenApplicationName = "gopager-listen"

// Reconnect delays of a lost listener connection: the first retry is quick because a restart
// of the database is usually short; repeated failures back off to the cap.
const (
	listenInitialBackoff = 500 * time.Millisecond
	listenMaxBackoff     = 30 * time.Second
)

// Listen subscribes to the PostgreSQL notification channel on a dedicated connection that is
// not part of the pool, because a pooled connection cannot hold a session-level LISTEN. The
// returned channel receives one coalesced signal per burst of notifications; payloads are
// ignored. After every (re)connect it also receives a signal, since notifications sent while the
// connection was down are lost and the receiver must look for work once. A lost connection is
// re-established with exponential backoff. The channel is closed when ctx ends.
//
// It fails with CodeInvalidInput for an empty channel name or a store without connection
// settings. Connection errors are never returned; they only delay the next signal.
func (s *Store) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	if channel == "" || s.config == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid listen request"))
	}
	signals := make(chan struct{}, 1)
	go s.listenLoop(ctx, pgx.Identifier{channel}.Sanitize(), signals)
	return signals, nil
}

// listenLoop keeps one LISTEN session alive until ctx ends and then closes signals.
func (s *Store) listenLoop(ctx context.Context, channel string, signals chan<- struct{}) {
	defer close(signals)
	backoff := listenInitialBackoff
	for ctx.Err() == nil {
		connected, _ := s.listenSession(ctx, channel, signals)
		if connected {
			backoff = listenInitialBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, listenMaxBackoff)
	}
}

// listenSession runs one connection from connect to failure. It reports whether the LISTEN
// succeeded, so the caller can reset its backoff, and the error that ended the session.
func (s *Store) listenSession(ctx context.Context, channel string, signals chan<- struct{}) (bool, error) {
	config := s.config.Copy()
	config.RuntimeParams["application_name"] = listenApplicationName
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return false, err
	}
	// The session ends with ctx, so cleanup must not depend on it.
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return false, err
	}
	signal(signals)
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return true, err
		}
		signal(signals)
	}
}

// signal coalesces: a pending signal already tells the receiver to look for work.
func signal(signals chan<- struct{}) {
	select {
	case signals <- struct{}{}:
	default:
	}
}
