package processing

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

func TestPublicationRejectsSyntheticSourceBinding(t *testing.T) {
	t.Parallel()
	binding := PublicationBinding{Source: SourceIdentity{TransmissionID: uuid.New(), NetworkID: uuid.New(),
		SourceRevision: 1, ContextRevision: 1, Synthetic: true}, MessageID: uuid.New(), MessageTime: time.Unix(1, 0), SourceFence: 1}
	if err := binding.Validate(); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("synthetic publication binding accepted: %v", err)
	}
	binding.Source.Synthetic = false
	if err := binding.Validate(); err != nil {
		t.Fatalf("otherwise-valid real source binding rejected: %v", err)
	}
}
