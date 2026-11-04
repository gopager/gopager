package protocols

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTransmissionPayloadIdentityUsesClosedProtocolForms(t *testing.T) {
	tests := []struct {
		name     string
		protocol Protocol
		payload  string
		want     PayloadIdentity
	}{
		{"text canonicalization", POCSAG, `{"form":"text","value":"  e\u0301\r\n "}`,
			PayloadIdentity{Form: "text", Value: []byte("é")}},
		{"numeric padding", FLEX, `{"form":"numeric","value":" 12) "}`,
			PayloadIdentity{Form: "numeric", Value: []byte("12)")}},
		{"tone only", POCSAG, `{"form":"tone-only"}`,
			PayloadIdentity{Form: "tone-only", Value: []byte{}}},
		{"binary subtype", FLEX, `{"form":"binary","subtype":"secure","encoding":"base64","data":"AAE="}`,
			PayloadIdentity{Form: "binary", Subtype: "secure", Value: []byte{0, 1}}},
		{"short signal", FLEX, `{"form":"flex-short-signal","sourceCode":7}`,
			PayloadIdentity{Form: "flex-short-signal", Value: []byte("7")}},
		{"zvei no tone", ZVEI, `{"form":"zvei-signal","double_tone":null}`,
			PayloadIdentity{Form: "zvei-signal", Value: []byte("null")}},
		{"eas header", EAS, `{"form":"eas-header","originator":"WXR","eventCode":"TOR","purgeTime":"0130","issueTime":"1230456"}`,
			PayloadIdentity{Form: "eas-header", Value: []byte(`["WXR","TOR","0130","1230456"]`)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload TransmissionPayload
			if err := json.Unmarshal([]byte(test.payload), &payload); err != nil {
				t.Fatal(err)
			}
			identity, err := payload.Identity(test.protocol)
			if err != nil || identity.Form != test.want.Form || identity.Subtype != test.want.Subtype ||
				!bytes.Equal(identity.Value, test.want.Value) {
				t.Fatalf("identity=%+v err=%v, want %+v", identity, err, test.want)
			}
		})
	}
}

func TestTransmissionPayloadRejectsWrongFormAndMalformedJSON(t *testing.T) {
	tests := []struct {
		protocol Protocol
		payload  string
	}{
		{POCSAG, `{"form":"text","value":"x","subtype":""}`},
		{POCSAG, `{"form":"tone-only","value":""}`},
		{FLEX, `{"form":"binary","subtype":"secure","encoding":"base64","data":"AB=="}`},
		{FLEX, `{"form":"flex-short-signal","sourceCode":8}`},
		{ZVEI, `{"form":"zvei-signal","double_tone":" "}`},
		{EAS, `{"form":"eas-header","originator":"WXR","eventCode":"TOR","purgeTime":"0160","issueTime":"1230456"}`},
		{POCSAG, `{"form":"text","value":"first","value":"second"}`},
		{POCSAG, `{"form":"text","value":null}`},
		{FLEX, `{"form":"flex-short-signal","sourceCode":null}`},
		{FLEX, `{"form":"binary","subtype":"secure","encoding":"base64","data":null}`},
		{POCSAG, `{"form":"text","value":"x","unknown":true}`},
		{POCSAG, `{"form":"text","value":"x"} {}`},
	}
	for _, test := range tests {
		var payload TransmissionPayload
		if err := json.Unmarshal([]byte(test.payload), &payload); err == nil {
			if _, err := payload.Identity(test.protocol); err == nil {
				t.Fatalf("accepted protocol=%s payload=%s", test.protocol, test.payload)
			}
		}
	}
}

func TestValidateTransmissionTargetsPreservesProtocolLimits(t *testing.T) {
	address := TextAddress("12345")
	if err := ValidateTransmissionTargets(ZVEI, []Target{{Address: address}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransmissionTargets(ZVEI, nil); err == nil {
		t.Fatal("accepted targetless ZVEI")
	}
	if err := ValidateTransmissionTargets(POCSAG, nil); err != nil {
		t.Fatalf("rejected targetless POCSAG: %v", err)
	}
	targets := make([]Target, MaxTransmissionTargets)
	for i := range targets {
		targets[i] = Target{Address: NumericAddress(1)}
	}
	if err := ValidateTransmissionTargets(FLEX, targets); err != nil {
		t.Fatalf("rejected maximum target count: %v", err)
	}
	if err := ValidateTransmissionTargets(FLEX, append(targets, Target{Address: NumericAddress(1)})); err == nil {
		t.Fatal("accepted target count above maximum")
	}
}
