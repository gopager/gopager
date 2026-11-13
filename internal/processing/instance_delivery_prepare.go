package processing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// Codes of a transient outcome produced before the request reached the plugin.
const (
	// CodePluginUnavailable means the artifact could not be loaded or the process not started.
	CodePluginUnavailable = "plugin-unavailable"
	// CodeConfigurationUnavailable means configuration or Secrets could not be read or resolved.
	CodeConfigurationUnavailable = "configuration-unavailable"
)

// DeliveryAttemptRequest is one provider call for a Delivery, addressed to an exact instance
// revision on behalf of a subscriber.
type DeliveryAttemptRequest struct {
	InstanceID       uuid.UUID
	InstanceRevision int64
	DeliveryID       uuid.UUID
	AttemptID        uuid.UUID
	AttemptNumber    int
	SubscriberID     uuid.UUID
	CorrelationID    *uuid.UUID
	CausationID      uuid.UUID
	OutputKind       string
	Body             string
	Inputs           map[string]Value
	Deadline         time.Time
}

// validate rejects requests that cannot be sent.
func (r DeliveryAttemptRequest) validate() error {
	if r.InstanceID == uuid.Nil || r.InstanceRevision <= 0 || r.DeliveryID == uuid.Nil || r.AttemptID == uuid.Nil ||
		r.AttemptNumber <= 0 || r.SubscriberID == uuid.Nil || r.CausationID == uuid.Nil || r.Deadline.IsZero() ||
		(r.CorrelationID != nil && *r.CorrelationID == uuid.Nil) || len(r.Inputs) > maxDeliveryInputs {
		return invalid(errors.New("invalid delivery attempt request"))
	}
	if !slices.Contains(deliveryOutputKinds, r.OutputKind) {
		return invalid(errors.New("invalid delivery output kind"))
	}
	return nil
}

// preparedAttempt is everything read inside the transaction that the invocation needs.
type preparedAttempt struct {
	manifest InstalledManifest
	cached   *cachedArtifact
	merged   Value
	requests []SecretRequest
	rows     map[uuid.UUID]StoredSecret
	instance uuid.UUID
}

// prepare reads the exact instance revision, the subscriber's personal values (only when the
// manifest has any) and the referenced stored Secret rows in one transaction. When it fails, code
// is the transient outcome code to report. Nothing here resolves or decrypts a Secret.
func (d *DeliveryHost) prepare(ctx context.Context, request DeliveryAttemptRequest) (prepared preparedAttempt, code string, failure error) {
	failure = d.cfg.Transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		prepared, code = preparedAttempt{}, CodeConfigurationUnavailable
		instance, err := d.cfg.Instances.ReadInstance(tx.Context(), tx, request.InstanceID, request.InstanceRevision)
		if err != nil {
			return err
		}
		code = CodePluginUnavailable
		manifest, cached, err := d.load(ctx, instance)
		if err != nil {
			return err
		}
		code = CodeConfigurationUnavailable
		if err := manifest.validateInstance(instance); err != nil {
			return err
		}
		var personal *Value
		if manifest.HasPersonalProperties() {
			stored, err := d.cfg.Personal.ReadPersonalConfiguration(tx.Context(), tx, instance.ID, request.SubscriberID)
			if err != nil && !errors.Is(err, authz.ErrNotFound) {
				return err
			}
			if err == nil {
				personal = &stored.Configuration
			}
		}
		merged, err := manifest.mergeConfiguration(instance.Configuration, personal)
		if err != nil {
			return err
		}
		requests, err := secretRequests(manifest, merged, request.SubscriberID)
		if err != nil {
			return err
		}
		if len(requests) != 0 && d.cfg.Cipher == nil {
			return capabilityUnavailable(errors.New("secret key is not configured"))
		}
		rows, err := d.readStoredRows(tx, requests)
		if err != nil {
			return err
		}
		prepared = preparedAttempt{manifest: manifest, cached: cached, merged: merged, requests: requests, rows: rows,
			instance: instance.ID}
		return nil
	})
	if failure != nil {
		return preparedAttempt{}, code, failure
	}
	return prepared, "", nil
}

// secretRequests lists the Secrets referenced by the merged configuration; user-scoped ones belong
// to the subscriber, administrative ones to nobody.
func secretRequests(manifest InstalledManifest, merged Value, subscriber uuid.UUID) ([]SecretRequest, error) {
	references, err := manifest.ConfigurationSecretReferences(merged)
	if err != nil {
		return nil, err
	}
	scopes := map[string]string{}
	for _, property := range manifest.SecretProperties() {
		scopes[property.Path] = property.Scope
	}
	requests := make([]SecretRequest, 0, len(references))
	for path, reference := range references {
		request := SecretRequest{Path: path, Reference: reference}
		if scopes[path] == "user" {
			owner := subscriber
			request.Owner = &owner
		}
		requests = append(requests, request)
	}
	return requests, nil
}

// readStoredRows reads the stored rows the requests name.
func (d *DeliveryHost) readStoredRows(tx platform.TransactionContext, requests []SecretRequest) (map[uuid.UUID]StoredSecret, error) {
	var ids []uuid.UUID
	for _, request := range requests {
		if request.Reference.Source == SecretStored {
			ids = append(ids, request.Reference.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return d.cfg.Secrets.ReadSecrets(tx.Context(), tx, ids)
}

// withSecrets replaces every Secret reference in the merged configuration with its plaintext.
func (p preparedAttempt) withSecrets(values map[string][]byte) (Value, error) {
	return rewriteSecrets(p.manifest.ConfigurationSchema, p.manifest.ConfigurationPolicy, p.merged, "",
		func(path string, leaf Value) (Value, error) {
			plain, found := values[path]
			if !found {
				return Value{}, capabilityUnavailable(errors.New("secret unavailable"))
			}
			encoded, err := json.Marshal(string(plain))
			if err != nil {
				return Value{}, invalid(err)
			}
			return Value{Kind: StringValue, Value: encoded}, nil
		})
}
