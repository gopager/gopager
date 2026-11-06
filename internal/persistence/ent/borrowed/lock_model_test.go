package borrowed_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gopager/gopager/internal/testutil/passwords"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// holdLock runs reader inside an open transaction and keeps that transaction open until
// the returned release function is called.
func holdLock(t *testing.T, store interface {
	WithinTransaction(context.Context, platform.TxFunc) error
}, reader func(context.Context, platform.TransactionContext) (int64, error)) (release func()) {
	t.Helper()
	held := make(chan struct{})
	free := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			if _, err := reader(tx.Context(), tx); err != nil {
				return err
			}
			close(held)
			<-free
			return nil
		})
	}()
	<-held
	return func() {
		close(free)
		if err := <-done; err != nil {
			t.Errorf("holding transaction: %v", err)
		}
	}
}

// TestControlRowReadersDoNotBlockEachOther expects the control-row locks taken by ordinary
// reads (authority resolution, matching, candidate work) to be shared: a second reader must
// proceed while the first still holds its lock.
func TestControlRowReadersDoNotBlockEachOther(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	identities, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	assignmentRepository, err := assignmentstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	processingRepository, err := processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		reader func(context.Context, platform.TransactionContext) (int64, error)
	}{
		{"identity_control", identities.LockAccess},
		{"assignment_control", assignmentRepository.LockRevision},
		{"processing_control", processingRepository.LockRevision},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			release := holdLock(t, store, c.reader)
			defer release()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
				_, err := c.reader(tx.Context(), tx)
				return err
			})
			if err != nil {
				t.Fatalf("second reader of %s blocked behind a reader: %v", c.name, err)
			}
		})
	}
}

// TestConfigurationChangeDoesNotWaitForAuthorityReaders holds the lock that authority
// resolution takes and expects a configuration change, which bumps the access revision,
// to commit meanwhile: only access-changing commands wait for in-flight readers.
func TestConfigurationChangeDoesNotWaitForAuthorityReaders(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	actor := seedLoginAccount(t, store, "Operator", false)
	identities, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	release := holdLock(t, store, identities.LockAccess)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return identities.RecordConfigurationChange(tx.Context(), tx, actor, uuid.New(), "unit.update", now)
	})
	if err != nil {
		t.Fatalf("configuration change waited for an authority reader: %v", err)
	}
}

// waitForLockWaiters polls until n backends wait on a row lock of the identity control
// row, so a test releases its reader only after every command reached that lock.
func waitForLockWaiters(t *testing.T, store *postgres.Store, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM pg_stat_activity
				WHERE wait_event_type = 'Lock' AND query ILIKE '%identity_control%'`).Scan(&waiting)
		})
		if err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("only %d of %d commands reached the access lock: %v", waiting, n, ctx.Err())
		case <-ticker.C:
		}
	}
}

// TestConcurrentLogoutsWaitForReadersWithoutConflict expects two Logouts that both wait for
// an open authority reader to succeed once it ends: an access-changing command takes the
// exclusive access lock before it resolves its own authority, so two such commands never
// hold the readers' lock while waiting for each other.
func TestConcurrentLogoutsWaitForReadersWithoutConflict(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	identities, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(identities, store, identities)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginService(store, resolver, identities, facts, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	var proofs [2]identity.Proof
	for i, name := range []string{"Alpha", "Bravo"} {
		seedLoginAccount(t, store, name, true)
		session, err := login.AuthenticateLocal(t.Context(), "test-source", name, "correct-password")
		if err != nil {
			t.Fatal(err)
		}
		proofs[i] = identity.Proof{Kind: identity.SessionProof, Secret: session.Secret}
	}
	release := holdLock(t, store, identities.LockAccess)
	results := make(chan error, len(proofs))
	for _, proof := range proofs {
		go func() {
			results <- login.Logout(context.Background(), proof, func(identity.Authentication) error { return nil })
		}()
	}
	waitForLockWaiters(t, store, len(proofs))
	release()
	for range proofs {
		if err := <-results; err != nil {
			t.Fatalf("logout that waited for a reader failed: %v", err)
		}
	}
	for _, proof := range proofs {
		if _, err := login.ReadSession(t.Context(), proof); !errors.Is(err, identity.ErrLoginRequired) {
			t.Fatalf("Session survived its logout: %v", err)
		}
	}
}

type intakeHoldAfterAccept struct {
	ingest.Repository
	held    chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *intakeHoldAfterAccept) AcceptTransmission(ctx context.Context, tx platform.TransactionContext,
	gateway uuid.UUID, submission ingest.ValidatedSubmission, accepted time.Time,
	match assignments.MatchResult,
) (ingest.Receipt, error) {
	receipt, err := r.Repository.AcceptTransmission(ctx, tx, gateway, submission, accepted, match)
	if err != nil {
		return ingest.Receipt{}, err
	}
	r.once.Do(func() {
		close(r.held)
		<-r.release
	})
	return receipt, nil
}

// TestIntakeOfDistinctIdentitiesOverlaps keeps one intake transaction open after it accepted
// a Transmission and expects an intake with a different Payload identity to complete
// meanwhile: Grouping serializes per Payload identity, not on a global row.
func TestIntakeOfDistinctIdentitiesOverlaps(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	holding := &intakeHoldAfterAccept{Repository: repository, held: make(chan struct{}), release: make(chan struct{})}
	slow, err := ingest.NewService(store, store, holding, noTargetMatcher{})
	if err != nil {
		t.Fatal(err)
	}
	fast, err := ingest.NewService(store, store, repository, noTargetMatcher{})
	if err != nil {
		t.Fatal(err)
	}
	// Two Gateways: a Gateway's own concurrent intakes still serialize on its Gateway row.
	network := uuid.New()
	gateways := [2]uuid.UUID{uuid.New(), uuid.New()}
	var secrets [2]string
	var hashes [2][32]byte
	for i := range gateways {
		secret, hash, err := identity.NewSecret()
		if err != nil {
			t.Fatal(err)
		}
		secrets[i], hashes[i] = secret, hash
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'overlap network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		for i, gateway := range gateways {
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
				VALUES($1,'active')`, gateway); err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
				(id,gateway_id,gateway_network_identifier,network_id,priority)
				VALUES($1,$2,'default',$3,0)`, uuid.New(), gateway, network); err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_credentials
				(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,false,$4)`,
				uuid.New(), gateway, hashes[i][:], now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	submission := func(id, payload string) ingest.Submission {
		return ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: now.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(payload), TransmissionTargets: []protocols.Target{}}
	}
	submit := func(ctx context.Context, service *ingest.Service, secret string, input ingest.Submission) (ingest.Receipt, error) {
		encoded, err := json.Marshal(input)
		if err != nil {
			return ingest.Receipt{}, err
		}
		return service.Submit(ctx, secret, input, len(encoded))
	}
	slowDone := make(chan error, 1)
	go func() {
		_, err := submit(context.Background(), slow, secrets[0], submission("alpha", `{"form":"text","value":"alpha"}`))
		slowDone <- err
	}()
	<-holding.held
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	receipt, err := submit(ctx, fast, secrets[1], submission("bravo", `{"form":"text","value":"bravo"}`))
	close(holding.release)
	if err != nil {
		t.Fatalf("intake of a different Payload identity blocked behind an open intake: %v", err)
	}
	if receipt.TransmissionID == uuid.Nil {
		t.Fatal("overlapping intake returned an empty receipt")
	}
	if err := <-slowDone; err != nil {
		t.Fatalf("held intake failed: %v", err)
	}
}
