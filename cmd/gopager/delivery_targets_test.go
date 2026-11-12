package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/processing"
)

type fakeDeliveryHost struct {
	description processing.DeliveryDescription
	outcome     processing.DeliveryAttemptOutcome
	err         error
	request     processing.DeliveryAttemptRequest
}

func (f *fakeDeliveryHost) Describe(context.Context, processing.InstanceConfiguration) (processing.DeliveryDescription, error) {
	return f.description, f.err
}

func (f *fakeDeliveryHost) Attempt(_ context.Context, request processing.DeliveryAttemptRequest) (processing.DeliveryAttemptOutcome, error) {
	f.request = request
	return f.outcome, f.err
}

func TestPluginDeliveryTargetsDescribe(t *testing.T) {
	instance := processing.InstanceConfiguration{ID: uuid.New(), Revision: 3}
	for name, personal := range map[string]bool{"admin": false, "admin-and-personal": true} {
		t.Run(name, func(t *testing.T) {
			host := &fakeDeliveryHost{description: processing.DeliveryDescription{OutputKinds: []string{"text"},
				Inputs:         []processing.DeliveryInputDeclaration{{Name: "to", Kind: processing.StringValue, Required: true, MaxBytes: 9}},
				Dependencies:   []processing.DeliveryDependency{{Kind: "network", Description: "smtp"}},
				SendsPerMinute: 5, Idempotent: true, Personal: personal}}
			got, err := pluginDeliveryTargets{host: host}.Describe(t.Context(), instance)
			if err != nil {
				t.Fatal(err)
			}
			want := delivery.TargetDescriptor{Version: delivery.DescriptorVersion, InstanceID: instance.ID, InstanceRevision: 3,
				CapabilityKind: delivery.CapabilityDelivery, ConfigurationAuthority: name, OutputKinds: []string{"text"},
				Inputs:       []delivery.InputField{{Name: "to", Kind: processing.StringValue, Required: true, MaxBytes: 9}},
				Dependencies: []delivery.TargetDependency{{Kind: "network", Description: "smtp"}}, SendsPerMinute: 5, Idempotent: true}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("descriptor = %+v, want %+v", got, want)
			}
		})
	}
	failure := errors.New("unavailable")
	if _, err := (pluginDeliveryTargets{host: &fakeDeliveryHost{err: failure}}).Describe(t.Context(), instance); !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
}

func TestPluginDeliveryTargetsAttempt(t *testing.T) {
	correlation := uuid.New()
	invocation := delivery.AttemptInvocation{DeliveryID: uuid.New(), AttemptID: uuid.New(), AttemptNumber: 2,
		InstanceID: uuid.New(), SubscriberID: uuid.New(), InstanceRevision: 4, CorrelationID: &correlation,
		CausationID: uuid.New(), Deadline: time.Now().Add(time.Minute),
		Input: delivery.DeliveryInput{OutputKind: "text", Body: "hi", Inputs: map[string]processing.Value{}}}
	for name, outcome := range map[string]processing.DeliveryAttemptOutcome{
		"accepted":  {Status: processing.AttemptAccepted, Code: "ok", Receipt: json.RawMessage(`{"id":1}`)},
		"transient": {Status: processing.AttemptTransient, Code: "busy", RetryAfter: 30 * time.Second},
		"permanent": {Status: processing.AttemptPermanent, Code: "rejected"},
		"uncertain": {Status: processing.AttemptUncertain, Code: "timeout"},
	} {
		t.Run(name, func(t *testing.T) {
			host := &fakeDeliveryHost{outcome: outcome}
			got, err := pluginDeliveryTargets{host: host}.Attempt(t.Context(), invocation)
			if err != nil {
				t.Fatal(err)
			}
			want := delivery.AttemptResult{Status: delivery.AttemptStatus(name), Code: outcome.Code,
				RetryAfter: outcome.RetryAfter, Receipt: outcome.Receipt}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("result = %+v, want %+v", got, want)
			}
			request := host.request
			if request.SubscriberID != invocation.SubscriberID || request.DeliveryID != invocation.DeliveryID ||
				request.AttemptID != invocation.AttemptID || request.AttemptNumber != 2 || request.InstanceRevision != 4 ||
				request.CorrelationID != &correlation || request.CausationID != invocation.CausationID ||
				request.OutputKind != "text" || request.Body != "hi" || !request.Deadline.Equal(invocation.Deadline) {
				t.Fatalf("request = %+v", request)
			}
		})
	}
	failure := errors.New("transport")
	if _, err := (pluginDeliveryTargets{host: &fakeDeliveryHost{err: failure}}).Attempt(t.Context(), invocation); err != failure {
		t.Fatalf("error = %v, want unchanged", err)
	}
}
