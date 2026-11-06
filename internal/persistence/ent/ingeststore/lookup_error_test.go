package ingeststore

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// TestNotFoundOnlyForMissingRow expects only a missing row to become NOT_FOUND; a storage
// failure must stay a failure the Gateway can retry.
func TestNotFoundOnlyForMissingRow(t *testing.T) {
	t.Parallel()
	if err := notFoundIfNoRows(sql.ErrNoRows); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("missing row mapped to %v, want NOT_FOUND", err)
	}
	storage := platform.NewFailure(platform.CodeStorageUnavailable, errors.New("connection reset"))
	err := notFoundIfNoRows(storage)
	if errors.Is(err, authz.ErrNotFound) || !errors.Is(err, storage) {
		t.Fatalf("storage failure mapped to %v, want it unchanged", err)
	}
}
