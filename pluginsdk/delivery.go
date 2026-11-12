package pluginsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

const (
	// SDKMinor is the SDK minor version that adds the Delivery capability; a Delivery
	// plugin's manifest declares "sdkMinor": 1.
	SDKMinor = 1
	// KindDelivery is the "kind" of a Delivery request.
	KindDelivery = "delivery"
	// MaxReceiptBytes is the largest receipt a DeliveryResult may carry.
	MaxReceiptBytes = 4096
	// MaxRetryAfterSeconds is the largest RetryAfterSeconds a DeliveryResult may carry.
	MaxRetryAfterSeconds = 86400
)

// Attempt statuses a DeliveryResult may carry.
const (
	// StatusAccepted means the target accepted the notification.
	StatusAccepted = "accepted"
	// StatusTransient means nothing was delivered and a later attempt may succeed.
	StatusTransient = "transient"
	// StatusPermanent means nothing was delivered and retrying cannot succeed.
	StatusPermanent = "permanent"
	// StatusUncertain means the target may or may not have acted; the Core never resends.
	StatusUncertain = "uncertain"
)

// Result codes the SDK itself answers with.
const (
	// CodeInvalidRequest answers a request the SDK could not decode; nothing was sent.
	CodeInvalidRequest = "invalid-request"
	// CodeInvalidResult answers a result the handler returned but the contract forbids.
	CodeInvalidResult = "invalid-result"
	// CodeHandlerFailed answers a handler that panicked.
	CodeHandlerFailed = "handler-failed"
)

var codePattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// DeliveryRequest is the Core's request for one Delivery attempt (schemaVersion 2, kind
// "delivery").
type DeliveryRequest struct {
	// SchemaVersion is always 2.
	SchemaVersion int `json:"schemaVersion"`
	// Kind is always KindDelivery.
	Kind string `json:"kind"`
	// InvocationID is the attempt ID.
	InvocationID string `json:"invocationId"`
	// InstanceID is the configured plugin instance.
	InstanceID string `json:"instanceId"`
	// InstanceRevision is the instance configuration revision as a decimal string.
	InstanceRevision string `json:"instanceRevision"`
	// ArtifactDigest is the lowercase hex SHA-256 of the running executable.
	ArtifactDigest string `json:"artifactDigest"`
	// CapabilityID is the manifest's capabilityId.
	CapabilityID string `json:"capabilityId"`
	// Deadline is the instant by which the attempt must end.
	Deadline time.Time `json:"deadline"`
	// Configuration is the merged configuration: a typed value of kind object that holds the
	// administrative values plus the subscriber's personal values, Secrets resolved to
	// plaintext. Decode it with DecodeConfiguration or ConfigurationValues.
	Configuration json.RawMessage `json:"configuration"`
	// SecretVersions maps Secret property paths to the version of the value that was resolved.
	SecretVersions map[string]string `json:"secretVersions"`
	// Attempt describes the notification to deliver.
	Attempt DeliveryAttempt `json:"attempt"`
}

// DeliveryAttempt is the notification of a DeliveryRequest.
type DeliveryAttempt struct {
	// DeliveryID is the Delivery this attempt belongs to.
	DeliveryID string `json:"deliveryId"`
	// AttemptID is the unique ID of this attempt.
	AttemptID string `json:"attemptId"`
	// AttemptNumber is the 1-based number of this attempt.
	AttemptNumber int `json:"attemptNumber"`
	// SubscriberID is the account whose Subscription produced the Delivery.
	SubscriberID string `json:"subscriberId"`
	// CorrelationID is the correlation ID of the originating event, when it has one.
	CorrelationID *string `json:"correlationId,omitempty"`
	// CausationID is the ID of the event that caused the Delivery.
	CausationID string `json:"causationId"`
	// OutputKind is the kind of Body: text, markdown or json.
	OutputKind string `json:"outputKind"`
	// Body is the rendered notification; for json it holds the JSON text.
	Body string `json:"body"`
	// Inputs maps the descriptor's input names to the typed values of the Subscription target
	// and the Unit parameters.
	Inputs map[string]Value `json:"inputs"`
}

// ConfigurationValues returns the typed members of the merged configuration.
func (r DeliveryRequest) ConfigurationValues() (map[string]Value, error) {
	var root Value
	if err := json.Unmarshal(r.Configuration, &root); err != nil {
		return nil, fmt.Errorf("pluginsdk: invalid configuration: %w", err)
	}
	if root.Kind != KindObject {
		return nil, fmt.Errorf("pluginsdk: configuration is of kind %q, not %q", root.Kind, KindObject)
	}
	var members map[string]Value
	if err := json.Unmarshal(root.Value, &members); err != nil {
		return nil, fmt.Errorf("pluginsdk: invalid configuration: %w", err)
	}
	return members, nil
}

// DecodeConfiguration unmarshals the merged configuration into target, a pointer to a struct
// (or map) with json tags. Typed values become plain JSON first: integers become numbers,
// objects and lists are converted recursively. Absent optional properties keep their zero value.
func (r DeliveryRequest) DecodeConfiguration(target any) error {
	members, err := r.ConfigurationValues()
	if err != nil {
		return err
	}
	plain, err := plainObject(members)
	if err != nil {
		return fmt.Errorf("pluginsdk: invalid configuration: %w", err)
	}
	if err := json.Unmarshal(plain, target); err != nil {
		return fmt.Errorf("pluginsdk: invalid configuration: %w", err)
	}
	return nil
}

// DeliveryResult is a handler's answer for one attempt.
type DeliveryResult struct {
	// Status is one of StatusAccepted, StatusTransient, StatusPermanent and StatusUncertain.
	Status string
	// Code is a machine-readable reason matching ^[a-z0-9-]{1,64}$. It must never contain
	// configuration, Secrets or input.
	Code string
	// RetryAfterSeconds asks the Core to wait before the next attempt (0..86400); 0 omits it.
	RetryAfterSeconds int
	// Receipt is optional JSON of at most MaxReceiptBytes bytes that identifies the delivery at
	// the target, for example an HTTP status and a provider request ID. It must not hold Secrets.
	Receipt json.RawMessage
}

// DeliveryHandler performs Delivery attempts.
type DeliveryHandler interface {
	// Deliver sends the notification of request and reports the outcome. The context ends at
	// the request's deadline or when the Core cancels the call.
	Deliver(ctx context.Context, request DeliveryRequest) DeliveryResult
}

// DeliveryHandlerFunc adapts a function to DeliveryHandler.
type DeliveryHandlerFunc func(ctx context.Context, request DeliveryRequest) DeliveryResult

// Deliver calls f.
func (f DeliveryHandlerFunc) Deliver(ctx context.Context, request DeliveryRequest) DeliveryResult {
	return f(ctx, request)
}

// ServeDelivery runs a Delivery plugin until the Core stops it. It does not return under
// normal operation. See HandleDeliveryRequest for the request and result rules.
func ServeDelivery(handler DeliveryHandler) {
	Serve(HandlerFunc(func(ctx context.Context, request []byte) ([]byte, error) {
		return HandleDeliveryRequest(ctx, handler, request), nil
	}))
}

// HandleDeliveryRequest decodes a Delivery request strictly (unknown members, a kind other
// than KindDelivery or a schemaVersion other than 2 are rejected), calls handler with a
// context that ends at the request's deadline, validates the result and encodes the response
// {"schemaVersion":2,"attempt":{...}}. A malformed request is answered permanent
// CodeInvalidRequest, because nothing was sent. An invalid result or a panicking handler is
// answered uncertain, because the handler may already have acted.
func HandleDeliveryRequest(ctx context.Context, handler DeliveryHandler, request []byte) []byte {
	decoder := json.NewDecoder(bytes.NewReader(request))
	decoder.DisallowUnknownFields()
	var parsed DeliveryRequest
	if err := decoder.Decode(&parsed); err != nil || decoder.More() ||
		parsed.SchemaVersion != 2 || parsed.Kind != KindDelivery {
		return encodeResult(DeliveryResult{Status: StatusPermanent, Code: CodeInvalidRequest})
	}
	if !parsed.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, parsed.Deadline)
		defer cancel()
	}
	result := safeDeliver(ctx, handler, parsed)
	if !validResult(result) {
		result = DeliveryResult{Status: StatusUncertain, Code: CodeInvalidResult}
	}
	return encodeResult(result)
}

// safeDeliver calls the handler and converts a panic into an uncertain result.
func safeDeliver(ctx context.Context, handler DeliveryHandler, request DeliveryRequest) (result DeliveryResult) {
	defer func() {
		if recover() != nil {
			result = DeliveryResult{Status: StatusUncertain, Code: CodeHandlerFailed}
		}
	}()
	return handler.Deliver(ctx, request)
}

// validResult reports whether result obeys the response contract.
func validResult(result DeliveryResult) bool {
	switch result.Status {
	case StatusAccepted, StatusTransient, StatusPermanent, StatusUncertain:
	default:
		return false
	}
	if !codePattern.MatchString(result.Code) ||
		result.RetryAfterSeconds < 0 || result.RetryAfterSeconds > MaxRetryAfterSeconds {
		return false
	}
	return len(result.Receipt) == 0 || (len(result.Receipt) <= MaxReceiptBytes && json.Valid(result.Receipt))
}

// encodeResult encodes a valid result as the response envelope.
func encodeResult(result DeliveryResult) []byte {
	type attempt struct {
		Status            string          `json:"status"`
		Code              string          `json:"code"`
		RetryAfterSeconds int             `json:"retryAfterSeconds,omitempty"`
		Receipt           json.RawMessage `json:"receipt,omitempty"`
	}
	encoded, err := json.Marshal(struct {
		SchemaVersion int     `json:"schemaVersion"`
		Attempt       attempt `json:"attempt"`
	}{2, attempt(result)})
	if err != nil {
		return []byte(`{"schemaVersion":2,"attempt":{"status":"uncertain","code":"invalid-result"}}`)
	}
	return encoded
}
