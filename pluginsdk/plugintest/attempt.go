package plugintest

import (
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/processing"
)

// DefaultTimeout is the attempt timeout used when AttemptInput names neither Timeout nor Deadline.
const DefaultTimeout = 10 * time.Second

// Result is the outcome of an attempt as the Core classified it: status, code, retry hint and
// the receipt after the Core's size, JSON and Secret checks.
type Result = processing.DeliveryAttemptOutcome

// AttemptInput describes one Delivery attempt. Zero fields take defaults.
type AttemptInput struct {
	// SubscriberID is the account whose Subscription produced the Delivery; zero selects
	// Harness.Subscriber.
	SubscriberID uuid.UUID
	// OutputKind is "text", "markdown" or "json"; empty selects "text".
	OutputKind string
	// Body is the rendered notification.
	Body string
	// Inputs are the descriptor inputs of the Subscription target and Unit parameters, converted
	// to typed values by Go type (strings, booleans, integers, floats, time.Time).
	Inputs map[string]any
	// AttemptNumber is the 1-based attempt number; zero selects 1.
	AttemptNumber int
	// Deadline is the instant by which the attempt must end. It wins over Timeout.
	Deadline time.Time
	// Timeout bounds the attempt when Deadline is zero; zero selects DefaultTimeout.
	Timeout time.Duration
}

// Describe verifies the current instance revision for use as a Delivery target and reports what
// it offers, exactly as the Core does when a Subscription is saved. The error is the Core's, for
// example for a configuration that does not satisfy the manifest.
func (h *Harness) Describe() (processing.DeliveryDescription, error) {
	h.t.Helper()
	return h.delivery.Describe(h.t.Context(), h.current())
}

// Attempt performs one Delivery attempt against the current instance revision through the Core's
// Delivery host. It returns the classified outcome and the error exactly as the Core sees them:
// an error (transport failure, expired deadline, malformed or oversized answer after the request
// was sent) means the Core records the attempt as uncertain and never sends it again.
func (h *Harness) Attempt(input AttemptInput) (Result, error) {
	h.t.Helper()
	instance := h.current()
	inputs := make(map[string]processing.Value, len(input.Inputs))
	for name, value := range input.Inputs {
		typed, err := typedValue(value)
		if err != nil {
			h.t.Fatalf("plugintest: input %s: %v", name, err)
		}
		inputs[name] = typed
	}
	request := processing.DeliveryAttemptRequest{InstanceID: instance.ID, InstanceRevision: instance.Revision,
		DeliveryID: uuid.New(), AttemptID: uuid.New(), AttemptNumber: input.AttemptNumber,
		SubscriberID: input.SubscriberID, CausationID: uuid.New(), OutputKind: input.OutputKind, Body: input.Body,
		Inputs: inputs, Deadline: input.Deadline}
	if request.AttemptNumber == 0 {
		request.AttemptNumber = 1
	}
	if request.SubscriberID == uuid.Nil {
		request.SubscriberID = h.subscriber
	}
	if request.OutputKind == "" {
		request.OutputKind = processing.LayoutText
	}
	if request.Deadline.IsZero() {
		timeout := input.Timeout
		if timeout == 0 {
			timeout = DefaultTimeout
		}
		request.Deadline = time.Now().Add(timeout)
	}
	return h.delivery.Attempt(h.t.Context(), request)
}
