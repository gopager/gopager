package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limits of a Delivery response.
const (
	// MaxReceiptBytes bounds the receipt a plugin may return.
	MaxReceiptBytes = 4096
	// MaxRetryAfterSeconds bounds the retry hint of a plugin.
	MaxRetryAfterSeconds = 86400
)

// Codes the host substitutes for unusable plugin answers.
const (
	// CodeInvalidCode replaces a code outside [a-z0-9-]{1,64}.
	CodeInvalidCode = "invalid-code"
	// CodeReceiptRedacted replaces the code when the receipt was dropped.
	CodeReceiptRedacted = "receipt-redacted"
)

var attemptCodePattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// Delivery attempt statuses a plugin may report.
const (
	AttemptAccepted  = "accepted"
	AttemptTransient = "transient"
	AttemptPermanent = "permanent"
	AttemptUncertain = "uncertain"
)

// DeliveryAttemptOutcome is the classified answer of one attempt. Receipt is bounded JSON the
// plugin deems safe to show, never containing a Secret of the invocation.
type DeliveryAttemptOutcome struct {
	Status     string
	Code       string
	RetryAfter time.Duration
	Receipt    json.RawMessage
}

type deliveryWireAttempt struct {
	DeliveryID    uuid.UUID        `json:"deliveryId"`
	AttemptID     uuid.UUID        `json:"attemptId"`
	AttemptNumber int              `json:"attemptNumber"`
	SubscriberID  uuid.UUID        `json:"subscriberId"`
	CorrelationID *uuid.UUID       `json:"correlationId,omitempty"`
	CausationID   uuid.UUID        `json:"causationId"`
	OutputKind    string           `json:"outputKind"`
	Body          string           `json:"body"`
	Inputs        map[string]Value `json:"inputs"`
}

type deliveryWireRequest struct {
	SchemaVersion    int                 `json:"schemaVersion"`
	Kind             string              `json:"kind"`
	InvocationID     uuid.UUID           `json:"invocationId"`
	InstanceID       uuid.UUID           `json:"instanceId"`
	InstanceRevision int64               `json:"instanceRevision,string"`
	ArtifactDigest   string              `json:"artifactDigest"`
	CapabilityID     string              `json:"capabilityId"`
	Deadline         time.Time           `json:"deadline"`
	Configuration    Value               `json:"configuration"`
	SecretVersions   map[string]string   `json:"secretVersions"`
	Attempt          deliveryWireAttempt `json:"attempt"`
}

type deliveryWireResponse struct {
	SchemaVersion int `json:"schemaVersion"`
	Attempt       *struct {
		Status            string          `json:"status"`
		Code              string          `json:"code"`
		RetryAfterSeconds int             `json:"retryAfterSeconds"`
		Receipt           json.RawMessage `json:"receipt"`
	} `json:"attempt"`
}

// unavailable is the transient outcome of a failure before the request reached the plugin.
func unavailable(code string) DeliveryAttemptOutcome {
	return DeliveryAttemptOutcome{Status: AttemptTransient, Code: code}
}

// Attempt performs one provider call. Configuration and Secrets are read in one transaction and
// resolved outside it; the plugin receives the merged configuration with plaintext Secrets only in
// this request, and the host keeps none afterwards.
//
// A failure before the request reached the plugin (artifact, configuration, Secrets, process start)
// is a transient outcome with a nil error. A transport failure, an expired deadline or a malformed
// or oversized response after the request was sent is returned as an error, which the Core
// records as uncertain.
func (d *DeliveryHost) Attempt(ctx context.Context, request DeliveryAttemptRequest) (DeliveryAttemptOutcome, error) {
	if d == nil || ctx == nil {
		return DeliveryAttemptOutcome{}, invalid(errors.New("missing delivery host/context"))
	}
	if err := request.validate(); err != nil {
		return DeliveryAttemptOutcome{}, err
	}
	prepared, code, err := d.prepare(ctx, request)
	if err != nil {
		if ctx.Err() != nil {
			return DeliveryAttemptOutcome{}, ctx.Err()
		}
		return unavailable(code), nil
	}
	resolver := SecretResolver{Cipher: d.cfg.Cipher, Files: d.cfg.SecretFiles, LookupEnv: d.cfg.LookupEnv}
	resolved, err := resolver.Resolve(prepared.instance, prepared.requests, prepared.rows)
	if err != nil {
		return unavailable(CodeConfigurationUnavailable), nil
	}
	defer resolved.Wipe()
	encoded, outcome, err := d.encode(prepared, request, resolved)
	if err != nil {
		return DeliveryAttemptOutcome{}, err
	}
	defer clear(encoded)
	if encoded == nil {
		return outcome, nil
	}
	response, err := d.send(ctx, prepared, request, encoded)
	if err != nil {
		if isUnsent(err) && ctx.Err() == nil {
			return unavailable(CodePluginUnavailable), nil
		}
		return DeliveryAttemptOutcome{}, err
	}
	outcome, err = parseDeliveryResponse(response, resolved.Values)
	if err != nil {
		d.cfg.Host.recordFailure(prepared.instance, StatusInvocationFailed)
	}
	return outcome, err
}

// encode builds the wire request. A configuration that does not conform to the schema is a
// configuration failure and a request over the input bound a permanent outcome; both return nil
// bytes with that outcome.
func (d *DeliveryHost) encode(prepared preparedAttempt, request DeliveryAttemptRequest,
	resolved ResolvedSecrets) ([]byte, DeliveryAttemptOutcome, error) {
	configuration, err := prepared.withSecrets(resolved.Values)
	if err != nil {
		return nil, unavailable(CodeConfigurationUnavailable), nil
	}
	if _, err := configuration.Native(prepared.manifest.ConfigurationSchema); err != nil {
		return nil, unavailable(CodeConfigurationUnavailable), nil
	}
	versions := resolved.Versions
	if versions == nil {
		versions = map[string]string{}
	}
	inputs := request.Inputs
	if inputs == nil {
		inputs = map[string]Value{}
	}
	encoded, err := json.Marshal(deliveryWireRequest{SchemaVersion: 2, Kind: CapabilityDelivery, InvocationID: request.AttemptID,
		InstanceID: request.InstanceID, InstanceRevision: request.InstanceRevision,
		ArtifactDigest: prepared.manifest.Artifact.Digest, CapabilityID: prepared.manifest.CapabilityID,
		Deadline: request.Deadline.UTC(), Configuration: configuration, SecretVersions: versions,
		Attempt: deliveryWireAttempt{DeliveryID: request.DeliveryID, AttemptID: request.AttemptID,
			AttemptNumber: request.AttemptNumber, SubscriberID: request.SubscriberID, CorrelationID: request.CorrelationID,
			CausationID: request.CausationID, OutputKind: request.OutputKind, Body: request.Body, Inputs: inputs}})
	if err != nil {
		return nil, DeliveryAttemptOutcome{}, invalid(err)
	}
	if len(encoded) > MaxInputBytes {
		clear(encoded)
		return nil, DeliveryAttemptOutcome{Status: AttemptPermanent, Code: "request-too-large"}, nil
	}
	return encoded, DeliveryAttemptOutcome{}, nil
}

// send invokes the pooled live process of the instance revision until the request deadline.
func (d *DeliveryHost) send(ctx context.Context, prepared preparedAttempt, request DeliveryAttemptRequest,
	encoded []byte) ([]byte, error) {
	digest := prepared.manifest.Artifact.Digest
	process, err := d.cfg.Host.acquire(processKey{instance: request.InstanceID, revision: request.InstanceRevision,
		digest: digest, mode: InvocationModeLive}, prepared.cached)
	if err != nil {
		return nil, &unsentError{cause: err}
	}
	defer process.release()
	ctx, cancel := context.WithDeadlineCause(ctx, request.Deadline, errInvocationDeadline)
	defer cancel()
	response, err := process.invoke(ctx, encoded)
	if err != nil {
		return nil, err
	}
	if err := invocationErr(ctx); err != nil {
		return nil, err
	}
	return response, nil
}

// parseDeliveryResponse validates the plugin's answer and applies the code and receipt rules. A
// malformed answer is an error; secrets are the plaintext values of this invocation.
func parseDeliveryResponse(response []byte, secrets map[string][]byte) (DeliveryAttemptOutcome, error) {
	if len(response) > MaxOutputBytes || !utf8.Valid(response) {
		return DeliveryAttemptOutcome{}, resource(errors.New("delivery response boundary"))
	}
	var decoded deliveryWireResponse
	if err := Decode(response, &decoded); err != nil {
		return DeliveryAttemptOutcome{}, err
	}
	answer := decoded.Attempt
	if decoded.SchemaVersion != 2 || answer == nil || answer.RetryAfterSeconds < 0 || answer.RetryAfterSeconds > MaxRetryAfterSeconds {
		return DeliveryAttemptOutcome{}, invalid(errors.New("malformed delivery response"))
	}
	switch answer.Status {
	case AttemptAccepted, AttemptTransient, AttemptPermanent, AttemptUncertain:
	default:
		return DeliveryAttemptOutcome{}, invalid(errors.New("malformed delivery response"))
	}
	outcome := DeliveryAttemptOutcome{Status: answer.Status, Code: answer.Code,
		RetryAfter: time.Duration(answer.RetryAfterSeconds) * time.Second, Receipt: answer.Receipt}
	if !attemptCodePattern.MatchString(outcome.Code) {
		outcome.Code = CodeInvalidCode
	}
	if bytes.Equal(bytes.TrimSpace(outcome.Receipt), []byte("null")) {
		outcome.Receipt = nil
	}
	if len(outcome.Receipt) != 0 && !receiptAllowed(outcome.Receipt, secrets) {
		outcome.Receipt, outcome.Code = nil, CodeReceiptRedacted
	}
	return outcome, nil
}

// receiptAllowed reports whether a receipt is JSON within the size bound that contains no
// plaintext Secret of the invocation, neither in its raw bytes nor in any decoded string or key
// (which catches escape sequences).
func receiptAllowed(receipt json.RawMessage, secrets map[string][]byte) bool {
	if len(receipt) > MaxReceiptBytes || !json.Valid(receipt) {
		return false
	}
	var decoded any
	if err := json.Unmarshal(receipt, &decoded); err != nil {
		return false
	}
	for _, secret := range secrets {
		if len(secret) == 0 {
			continue
		}
		if bytes.Contains(receipt, secret) || containsSecretString(decoded, string(secret)) {
			return false
		}
	}
	return true
}

// containsSecretString searches every string and object key of a decoded JSON value.
func containsSecretString(value any, secret string) bool {
	switch current := value.(type) {
	case string:
		return strings.Contains(current, secret)
	case []any:
		for _, item := range current {
			if containsSecretString(item, secret) {
				return true
			}
		}
	case map[string]any:
		for key, item := range current {
			if strings.Contains(key, secret) || containsSecretString(item, secret) {
				return true
			}
		}
	}
	return false
}
