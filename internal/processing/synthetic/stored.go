package synthetic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/protocols"
)

// storedGroups are the field groups every stored preview projection carries: the Payload, the
// target addresses and the Units they resolve to. The groups of further fields a definition reads
// are checked by the preview service once the definition is compiled.
var storedGroups = []string{"payload", "address", "unit"}

// StoredTransmission is the stored evidence of one accepted Transmission that a preview evaluates.
type StoredTransmission struct {
	// MessageID is the Message the Transmission supports.
	MessageID uuid.UUID
	// Protocol is the Transmission's Protocol.
	Protocol protocols.Protocol
	// Payload is the stored Payload document, including its form.
	Payload json.RawMessage
	// ObservedAt is the observation time.
	ObservedAt time.Time
	// NetworkID is the Network the Transmission was received on.
	NetworkID uuid.UUID
	// Targets are the original targets in order.
	Targets []protocols.Target
}

// StoredTransmissionReader reads the evidence of an accepted Transmission without side effects,
// inside the caller's transaction. It does no authorization.
type StoredTransmissionReader interface {
	// ReadStoredTransmission returns the Transmission, or authz.ErrNotFound when it does not exist.
	ReadStoredTransmission(context.Context, platform.TransactionContext, uuid.UUID) (StoredTransmission, error)
}

// TransmissionAccess decides whether an authority may read a Transmission of a Message.
type TransmissionAccess interface {
	// CheckTransmissionReadable returns nil when the Transmission is a supporting readable
	// Transmission of the Message for the authority, and authz.ErrNotFound otherwise, so a hidden
	// Transmission is indistinguishable from a missing one.
	CheckTransmissionReadable(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
		now time.Time, messageID, transmissionID uuid.UUID) error
}

// WithStoredTransmissions makes previews of stored Transmissions available. Without it
// CaptureStoredPreview fails as unavailable.
func WithStoredTransmissions(reader StoredTransmissionReader, access TransmissionAccess) Option {
	return func(o *Owner) { o.stored, o.access = reader, access }
}

// CaptureStoredPreview reads the stored Transmission, checks that the authority may read it and
// every field group the projection carries (authz.ErrNotFound for a Transmission it cannot read,
// authz.ErrForbidden for a missing field group), and evaluates it like a synthetic sample against
// the current Network, Assignment context and Unit attributes. Nothing is persisted. The captured
// context is retained until ReleaseSyntheticPreview is called with the projection's TransmissionID.
func (o *Owner) CaptureStoredPreview(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	workflow processing.WorkflowConfiguration, definition processing.Definition, transmission uuid.UUID,
) (management.PreviewCapture, error) {
	if o == nil || ctx == nil || tx == nil || o.matcher == nil || o.clock == nil {
		return management.PreviewCapture{}, invalid(errors.New("missing stored capture participant"))
	}
	if o.stored == nil || o.access == nil || o.networks == nil {
		return management.PreviewCapture{}, unavailable(errors.New("stored Transmission owner unavailable"))
	}
	if transmission == uuid.Nil {
		return management.PreviewCapture{}, authz.ErrInvalidInput
	}
	record, err := o.stored.ReadStoredTransmission(ctx, tx, transmission)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	now, err := o.clock.Now(ctx, tx)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	if err := o.access.CheckTransmissionReadable(ctx, tx, authority, now, record.MessageID, transmission); err != nil {
		return management.PreviewCapture{}, err
	}
	if err := authz.AuthorizeFields(authority, storedGroups); err != nil {
		return management.PreviewCapture{}, err
	}
	input, err := record.input()
	if err != nil {
		return management.PreviewCapture{}, err
	}
	// Matching works on whole seconds, like the observation time of a synthetic sample.
	return o.captureInput(ctx, tx, workflow, definition, input, record.ObservedAt.UTC().Truncate(time.Second))
}

// input converts the stored evidence into the synthetic input it is evaluated as: the form and
// subtype of the stored Payload document and the rest of it as the Payload.
func (t StoredTransmission) input() (Input, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(t.Payload, &document); err != nil {
		return Input{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("stored Payload is not a document"))
	}
	input := Input{SchemaVersion: 1, Protocol: t.Protocol, NetworkID: t.NetworkID, Targets: t.Targets}
	for _, member := range []struct {
		name string
		into *string
	}{{"form", &input.Form}, {"subtype", &input.Subtype}} {
		raw, present := document[member.name]
		if !present {
			continue
		}
		if err := json.Unmarshal(raw, member.into); err != nil {
			return Input{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("stored Payload is invalid"))
		}
		delete(document, member.name)
	}
	if len(document) > 0 {
		payload, err := json.Marshal(document)
		if err != nil {
			return Input{}, err
		}
		input.Payload = bytes.Clone(payload)
	}
	if input.Targets == nil {
		input.Targets = []protocols.Target{}
	}
	if err := input.validate(); err != nil {
		return Input{}, err
	}
	return input, nil
}
