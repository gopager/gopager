package borrowed_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// seedPendingCandidates stores one active Workflow and admits count due candidates for it.
func seedPendingCandidates(t *testing.T, primary *postgres.Store, repository *processingstore.Store, count int) map[uuid.UUID]bool {
	t.Helper()
	actor := uuid.New()
	authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	configuration := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Name: "selection",
		Scope: authz.Scope{Kind: "all"}, Enabled: true}
	version := processing.Version{ID: uuid.New(), WorkflowID: configuration.ID,
		CreatedBy: actor, Definition: preparedProcessingDefinition(t)}
	admitted := make(map[uuid.UUID]bool, count)
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := repository.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		configuration.CreatedAt, configuration.UpdatedAt, version.CreatedAt = now, now, now
		if err := repository.SaveWorkflow(tx.Context(), tx, authority, "workflow.create", configuration, 0, now); err != nil {
			return err
		}
		if err := repository.SaveVersion(tx.Context(), tx, version); err != nil {
			return err
		}
		configuration.Revision, configuration.ActiveVersionID = 2, &version.ID
		if err := repository.SaveWorkflow(tx.Context(), tx, authority, "workflow.activate", configuration, 1, now); err != nil {
			return err
		}
		revision, err := repository.LockRevision(tx.Context(), tx)
		if err != nil {
			return err
		}
		for range count {
			candidate, err := repository.AdmitCandidate(tx.Context(), tx, processing.CandidateBinding{
				TransmissionID: uuid.New(), WorkflowID: configuration.ID, WorkflowRevision: configuration.Revision,
				DefinitionVersionID: version.ID, ConfigurationRevision: revision, SourceRevision: 1, ContextRevision: 1})
			if err != nil {
				return err
			}
			admitted[candidate.ID] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return admitted
}

func TestSelectDueWorkGivesConcurrentOpenSelectionsDisjointRows(t *testing.T) {
	primary := newStore(t)
	repository, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	admitted := seedPendingCandidates(t, primary, repository, 4)
	firstSelected, release := make(chan []processing.Candidate, 1), make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			rows, err := repository.SelectDueWork(tx.Context(), tx, 2)
			if err != nil {
				return err
			}
			firstSelected <- rows
			<-release
			return nil
		})
	}()
	first := <-firstSelected
	var second []processing.Candidate
	err = primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		second, err = repository.SelectDueWork(tx.Context(), tx, 2)
		return err
	})
	close(release)
	if firstErr := <-firstDone; firstErr != nil {
		t.Fatal(firstErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("selection sizes first=%d second=%d want 2 and 2", len(first), len(second))
	}
	seen := make(map[uuid.UUID]bool)
	for _, candidate := range append(first, second...) {
		if seen[candidate.ID] || !admitted[candidate.ID] {
			t.Fatalf("candidate %s selected twice or unknown while both selections were open", candidate.ID)
		}
		seen[candidate.ID] = true
	}
}

func TestSelectDueWorkDoesNotWaitForExclusiveControlLock(t *testing.T) {
	primary := newStore(t)
	repository, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	admitted := seedPendingCandidates(t, primary, repository, 1)
	locked, release := make(chan struct{}), make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			if _, err := repository.LockRevisionForChange(tx.Context(), tx); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	timeout, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var rows []processing.Candidate
	err = primary.WithinTransaction(timeout, func(tx platform.TransactionContext) error {
		var err error
		rows, err = repository.SelectDueWork(tx.Context(), tx, 10)
		return err
	})
	close(release)
	if writerErr := <-writerDone; writerErr != nil {
		t.Fatal(writerErr)
	}
	if err != nil {
		t.Fatalf("selection waited for the exclusive control lock: %v", err)
	}
	if len(rows) != 1 || !admitted[rows[0].ID] {
		t.Fatalf("selection=%+v want the admitted candidate", rows)
	}
}
