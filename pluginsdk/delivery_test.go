package pluginsdk

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const validRequest = `{"schemaVersion":2,"kind":"delivery","invocationId":"a1","instanceId":"i1","instanceRevision":"3",
"artifactDigest":"abc","capabilityId":"webhook","deadline":"2026-11-12T10:00:30Z",
"configuration":{"kind":"object","value":{"url":{"kind":"string","value":"https://x.example"},
"priority":{"kind":"integer","value":"-1"},"flag":{"kind":"boolean","value":true},"ratio":{"kind":"number","value":1.5}}},
"secretVersions":{"bearerToken":"secret:1"},
"attempt":{"deliveryId":"d1","attemptId":"a1","attemptNumber":2,"subscriberId":"s1","correlationId":"c1","causationId":"e1",
"outputKind":"json","body":"{\"level\":2}","inputs":{"topic":{"kind":"string","value":"alarm"}}}}`

func deliverWith(t *testing.T, request string, f DeliveryHandlerFunc) map[string]any {
	t.Helper()
	var response struct {
		SchemaVersion int            `json:"schemaVersion"`
		Attempt       map[string]any `json:"attempt"`
	}
	if err := json.Unmarshal(HandleDeliveryRequest(context.Background(), f, []byte(request)), &response); err != nil {
		t.Fatal(err)
	}
	if response.SchemaVersion != 2 {
		t.Fatalf("schemaVersion %d", response.SchemaVersion)
	}
	return response.Attempt
}

func TestHandleDeliveryRequestPassesDecodedRequest(t *testing.T) {
	t.Parallel()
	var got DeliveryRequest
	var deadline time.Time
	attempt := deliverWith(t, validRequest, func(ctx context.Context, request DeliveryRequest) DeliveryResult {
		got = request
		deadline, _ = ctx.Deadline()
		return DeliveryResult{Status: StatusAccepted, Code: "ok", RetryAfterSeconds: 7, Receipt: json.RawMessage(`{"status": 204}`)}
	})
	if got.InstanceRevision != "3" || got.SecretVersions["bearerToken"] != "secret:1" ||
		got.Attempt.AttemptNumber != 2 || got.Attempt.CorrelationID == nil || *got.Attempt.CorrelationID != "c1" ||
		got.Attempt.Body != `{"level":2}` {
		t.Fatalf("decoded %+v", got)
	}
	topic, err := got.Attempt.Inputs["topic"].String()
	if err != nil || topic != "alarm" {
		t.Fatalf("topic %q %v", topic, err)
	}
	if !deadline.Equal(time.Date(2026, 11, 12, 10, 0, 30, 0, time.UTC)) {
		t.Fatalf("context deadline %v", deadline)
	}
	if attempt["status"] != "accepted" || attempt["code"] != "ok" || attempt["retryAfterSeconds"] != float64(7) ||
		attempt["receipt"].(map[string]any)["status"] != float64(204) {
		t.Fatalf("response %v", attempt)
	}
}

func TestHandleDeliveryRequestRejectsMalformedRequests(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"not json":               `nope`,
		"wrong schema":           strings.Replace(validRequest, `"schemaVersion":2`, `"schemaVersion":1`, 1),
		"wrong kind":             strings.Replace(validRequest, `"kind":"delivery"`, `"kind":"other"`, 1),
		"missing kind":           strings.Replace(validRequest, `"kind":"delivery",`, ``, 1),
		"unknown member":         strings.Replace(validRequest, `"capabilityId"`, `"surprise":1,"capabilityId"`, 1),
		"unknown attempt member": strings.Replace(validRequest, `"causationId"`, `"extra":true,"causationId"`, 1),
		"trailing data":          validRequest + `{}`,
	}
	for name, request := range cases {
		called := false
		attempt := deliverWith(t, request, func(context.Context, DeliveryRequest) DeliveryResult {
			called = true
			return DeliveryResult{Status: StatusAccepted, Code: "ok"}
		})
		if called || attempt["status"] != "permanent" || attempt["code"] != "invalid-request" {
			t.Errorf("%s: called=%v %v", name, called, attempt)
		}
	}
}

func TestHandleDeliveryRequestRejectsInvalidResults(t *testing.T) {
	t.Parallel()
	cases := map[string]DeliveryResult{
		"unknown status":  {Status: "done", Code: "ok"},
		"empty code":      {Status: StatusAccepted},
		"upper code":      {Status: StatusAccepted, Code: "OK"},
		"long code":       {Status: StatusAccepted, Code: strings.Repeat("a", 65)},
		"negative retry":  {Status: StatusTransient, Code: "x", RetryAfterSeconds: -1},
		"large retry":     {Status: StatusTransient, Code: "x", RetryAfterSeconds: 86401},
		"invalid receipt": {Status: StatusAccepted, Code: "ok", Receipt: json.RawMessage(`{broken`)},
		"large receipt":   {Status: StatusAccepted, Code: "ok", Receipt: json.RawMessage(`"` + strings.Repeat("a", 4096) + `"`)},
	}
	for name, result := range cases {
		attempt := deliverWith(t, validRequest, func(context.Context, DeliveryRequest) DeliveryResult { return result })
		if attempt["status"] != "uncertain" || attempt["code"] != "invalid-result" {
			t.Errorf("%s: %v", name, attempt)
		}
	}
}

func TestHandleDeliveryRequestAcceptsBoundaryResults(t *testing.T) {
	t.Parallel()
	receipt := json.RawMessage(`"` + strings.Repeat("a", 4094) + `"`)
	attempt := deliverWith(t, validRequest, func(context.Context, DeliveryRequest) DeliveryResult {
		return DeliveryResult{Status: StatusTransient, Code: strings.Repeat("a", 64), RetryAfterSeconds: 86400, Receipt: receipt}
	})
	if attempt["status"] != "transient" || attempt["retryAfterSeconds"] != float64(86400) {
		t.Fatalf("%v", attempt)
	}
}

func TestHandleDeliveryRequestPanicIsUncertain(t *testing.T) {
	t.Parallel()
	attempt := deliverWith(t, validRequest, func(context.Context, DeliveryRequest) DeliveryResult { panic("boom") })
	if attempt["status"] != "uncertain" || attempt["code"] != "handler-failed" {
		t.Fatalf("%v", attempt)
	}
}

func TestValueAccessors(t *testing.T) {
	t.Parallel()
	if s, err := (Value{Kind: "string", Value: json.RawMessage(`"a"`)}).String(); err != nil || s != "a" {
		t.Errorf("string %q %v", s, err)
	}
	if n, err := (Value{Kind: "integer", Value: json.RawMessage(`"-9223372036854775808"`)}).Integer(); err != nil || n != -9223372036854775808 {
		t.Errorf("integer %d %v", n, err)
	}
	if b, err := (Value{Kind: "boolean", Value: json.RawMessage(`true`)}).Boolean(); err != nil || !b {
		t.Errorf("boolean %v %v", b, err)
	}
	if f, err := (Value{Kind: "number", Value: json.RawMessage(`1.25`)}).Number(); err != nil || f != 1.25 {
		t.Errorf("number %v %v", f, err)
	}
	mismatches := []error{
		func() error { _, err := (Value{Kind: "integer", Value: json.RawMessage(`"1"`)}).String(); return err }(),
		func() error { _, err := (Value{Kind: "string", Value: json.RawMessage(`"1"`)}).Integer(); return err }(),
		func() error { _, err := (Value{Kind: "string", Value: json.RawMessage(`"1"`)}).Boolean(); return err }(),
		func() error { _, err := (Value{Kind: "string", Value: json.RawMessage(`"1"`)}).Number(); return err }(),
		func() error { _, err := (Value{Kind: "integer", Value: json.RawMessage(`"x"`)}).Integer(); return err }(),
		func() error { _, err := (Value{Kind: "string", Value: json.RawMessage(`1`)}).String(); return err }(),
	}
	for i, err := range mismatches {
		if err == nil {
			t.Errorf("mismatch %d accepted", i)
		}
	}
}

func TestDecodeConfiguration(t *testing.T) {
	t.Parallel()
	var parsed DeliveryRequest
	if err := json.Unmarshal([]byte(validRequest), &parsed); err != nil {
		t.Fatal(err)
	}
	var config struct {
		URL      string  `json:"url"`
		Priority int     `json:"priority"`
		Flag     bool    `json:"flag"`
		Ratio    float64 `json:"ratio"`
		Optional string  `json:"optional"`
	}
	if err := parsed.DecodeConfiguration(&config); err != nil {
		t.Fatal(err)
	}
	if config.URL != "https://x.example" || config.Priority != -1 || !config.Flag || config.Ratio != 1.5 || config.Optional != "" {
		t.Fatalf("%+v", config)
	}
	members, err := parsed.ConfigurationValues()
	if err != nil || len(members) != 4 {
		t.Fatalf("%v %v", members, err)
	}
	for name, raw := range map[string]string{
		"not typed":     `{"url":"x"}`,
		"not an object": `{"kind":"string","value":"x"}`,
		"bad member":    `{"kind":"object","value":{"n":{"kind":"integer","value":"x"}}}`,
	} {
		bad := DeliveryRequest{Configuration: json.RawMessage(raw)}
		if err := bad.DecodeConfiguration(&config); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
