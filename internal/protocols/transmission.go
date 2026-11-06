package protocols

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxTransmissionTargets is the maximum number of targets per Transmission.
const MaxTransmissionTargets = 1024

// ErrInvalidTransmission is returned for any malformed or unsupported
// Transmission payload or target set.
var ErrInvalidTransmission = errors.New("invalid Transmission")

// TransmissionPayload is the closed, finalized Payload representation accepted by intake.
type TransmissionPayload struct {
	Form       string
	Value      string
	Subtype    string
	SourceCode *uint8
	DoubleTone json.RawMessage
	Originator string
	EventCode  string
	PurgeTime  string
	IssueTime  string
	Binary     []byte
	fields     map[string]bool
}

// UnmarshalJSON decodes only the explicitly supported form-specific fields.
func (p *TransmissionPayload) UnmarshalJSON(data []byte) error {
	members, err := transmissionPayloadMembers(data)
	if err != nil {
		return ErrInvalidTransmission
	}
	var raw struct {
		Form       string          `json:"form"`
		Value      json.RawMessage `json:"value"`
		Subtype    json.RawMessage `json:"subtype"`
		SourceCode json.RawMessage `json:"sourceCode"`
		DoubleTone json.RawMessage `json:"double_tone"`
		Originator string          `json:"originator"`
		EventCode  string          `json:"eventCode"`
		PurgeTime  string          `json:"purgeTime"`
		IssueTime  string          `json:"issueTime"`
		Encoding   json.RawMessage `json:"encoding"`
		Data       json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return ErrInvalidTransmission
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ErrInvalidTransmission
	}
	*p = TransmissionPayload{Form: raw.Form, DoubleTone: bytes.Clone(raw.DoubleTone),
		Originator: raw.Originator, EventCode: raw.EventCode,
		PurgeTime: raw.PurgeTime, IssueTime: raw.IssueTime, fields: make(map[string]bool)}
	if raw.Value != nil {
		var value string
		if isJSONNull(raw.Value) || json.Unmarshal(raw.Value, &value) != nil {
			return ErrInvalidTransmission
		}
		p.Value = value
		p.fields["value"] = true
	}
	if raw.Subtype != nil {
		if isJSONNull(raw.Subtype) || json.Unmarshal(raw.Subtype, &p.Subtype) != nil {
			return ErrInvalidTransmission
		}
		p.fields["subtype"] = true
	}
	if raw.SourceCode != nil {
		var sourceCode uint8
		if isJSONNull(raw.SourceCode) || json.Unmarshal(raw.SourceCode, &sourceCode) != nil {
			return ErrInvalidTransmission
		}
		p.SourceCode = &sourceCode
		p.fields["sourceCode"] = true
	}
	if raw.DoubleTone != nil {
		p.fields["double_tone"] = true
	}
	if raw.Encoding != nil {
		var encoding string
		if isJSONNull(raw.Encoding) || json.Unmarshal(raw.Encoding, &encoding) != nil || encoding != "base64" {
			return ErrInvalidTransmission
		}
		p.fields["encoding"] = true
	}
	if raw.Data != nil {
		var data string
		if isJSONNull(raw.Data) || json.Unmarshal(raw.Data, &data) != nil {
			return ErrInvalidTransmission
		}
		if !p.fields["encoding"] {
			return ErrInvalidTransmission
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(data)
		if err != nil || base64.StdEncoding.EncodeToString(decoded) != data {
			return ErrInvalidTransmission
		}
		p.Binary = decoded
		p.fields["data"] = true
	}
	for field := range members {
		p.fields[field] = true
	}
	return nil
}

func isJSONNull(value []byte) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func transmissionPayloadMembers(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalidTransmission
	}
	members := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidTransmission
		}
		name, ok := token.(string)
		if !ok {
			return nil, ErrInvalidTransmission
		}
		if _, exists := members[name]; exists {
			return nil, ErrInvalidTransmission
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidTransmission
		}
		members[name] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalidTransmission
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidTransmission
	}
	return members, nil
}

func (p TransmissionPayload) hasExactFields(expected ...string) bool {
	if p.fields == nil || len(p.fields) != len(expected) {
		return false
	}
	for _, field := range expected {
		if !p.fields[field] {
			return false
		}
	}
	return true
}

// PayloadIdentity is a comparison-only identity; submitted Payload bytes remain unchanged.
type PayloadIdentity struct {
	Form    string
	Subtype string
	Value   []byte
}

// ValidateTransmissionTargets preserves the original ordered list and rejects unsupported targets.
func ValidateTransmissionTargets(protocol Protocol, targets []Target) error {
	if !protocol.Valid() || len(targets) > MaxTransmissionTargets ||
		(protocol == ZVEI || protocol == EAS) && len(targets) == 0 ||
		protocol == EAS && len(targets) > 31 {
		return ErrInvalidTransmission
	}
	for _, target := range targets {
		if err := ValidateTarget(protocol, target); err != nil {
			return ErrInvalidTransmission
		}
	}
	return nil
}

// Identity validates form-specific data and builds exact server-owned comparison identity.
func (p TransmissionPayload) Identity(protocol Protocol) (PayloadIdentity, error) {
	invalid := func() (PayloadIdentity, error) { return PayloadIdentity{}, ErrInvalidTransmission }
	if !protocol.Valid() || p.Form == "" || !utf8.ValidString(p.Value) {
		return invalid()
	}
	switch p.Form {
	case "text":
		if (protocol != POCSAG && protocol != FLEX) || !p.hasExactFields("form", "value") ||
			p.Subtype != "" || p.SourceCode != nil ||
			p.DoubleTone != nil || p.Originator != "" || p.EventCode != "" || p.PurgeTime != "" ||
			p.IssueTime != "" || p.Binary != nil {
			return invalid()
		}
		value := norm.NFC.String(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(p.Value, "\r\n", "\n"), "\r", "\n")))
		return PayloadIdentity{Form: p.Form, Value: []byte(value)}, nil
	case "numeric":
		if (protocol != POCSAG && protocol != FLEX) || !p.hasExactFields("form", "value") ||
			p.Subtype != "" || p.SourceCode != nil ||
			p.DoubleTone != nil || p.Originator != "" || p.EventCode != "" || p.PurgeTime != "" ||
			p.IssueTime != "" || p.Binary != nil || !numericPayload(p.Value) {
			return invalid()
		}
		return PayloadIdentity{Form: p.Form, Value: []byte(strings.Trim(p.Value, " "))}, nil
	case "tone-only":
		if (protocol != POCSAG && protocol != FLEX) || !p.hasExactFields("form") || p.Value != "" || p.Subtype != "" ||
			p.SourceCode != nil || p.DoubleTone != nil || p.Originator != "" || p.EventCode != "" ||
			p.PurgeTime != "" || p.IssueTime != "" || p.Binary != nil {
			return invalid()
		}
		return PayloadIdentity{Form: p.Form, Value: []byte{}}, nil
	case "binary":
		if protocol != FLEX || !p.hasExactFields("form", "subtype", "encoding", "data") ||
			(p.Subtype != "transparent" && p.Subtype != "secure") ||
			p.Value != "" || p.SourceCode != nil || p.DoubleTone != nil || p.Originator != "" ||
			p.EventCode != "" || p.PurgeTime != "" || p.IssueTime != "" || p.Binary == nil {
			return invalid()
		}
		return PayloadIdentity{Form: p.Form, Subtype: p.Subtype, Value: bytes.Clone(p.Binary)}, nil
	case "flex-short-signal":
		if protocol != FLEX || !p.hasExactFields("form", "sourceCode") ||
			p.SourceCode == nil || *p.SourceCode > 7 || p.Value != "" ||
			p.Subtype != "" || p.DoubleTone != nil || p.Originator != "" || p.EventCode != "" ||
			p.PurgeTime != "" || p.IssueTime != "" || p.Binary != nil {
			return invalid()
		}
		return PayloadIdentity{Form: p.Form, Value: []byte(strconv.Itoa(int(*p.SourceCode)))}, nil
	case "zvei-signal":
		if protocol != ZVEI || !p.hasExactFields("form", "double_tone") ||
			p.DoubleTone == nil || p.Value != "" || p.Subtype != "" ||
			p.SourceCode != nil || p.Originator != "" || p.EventCode != "" || p.PurgeTime != "" ||
			p.IssueTime != "" || p.Binary != nil {
			return invalid()
		}
		var tone *string
		if !bytes.Equal(bytes.TrimSpace(p.DoubleTone), []byte("null")) {
			var value string
			if json.Unmarshal(p.DoubleTone, &value) != nil || strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
				return invalid()
			}
			tone = &value
		}
		return PayloadIdentity{Form: p.Form, Value: canonicalJSON(tone)}, nil
	case "eas-header":
		if protocol != EAS || !p.hasExactFields("form", "originator", "eventCode", "purgeTime", "issueTime") ||
			!asciiCode(p.Originator) || !asciiCode(p.EventCode) ||
			!validPurgeTime(p.PurgeTime) || !validIssueTime(p.IssueTime) || p.Value != "" ||
			p.Subtype != "" || p.SourceCode != nil || p.DoubleTone != nil || p.Binary != nil {
			return invalid()
		}
		return PayloadIdentity{Form: p.Form, Value: canonicalJSON([]string{
			p.Originator, p.EventCode, p.PurgeTime, p.IssueTime,
		})}, nil
	default:
		return invalid()
	}
}

func numericPayload(value string) bool {
	for _, character := range value {
		if !strings.ContainsRune("0123456789.U -)(", character) {
			return false
		}
	}
	return true
}

func asciiCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validPurgeTime(value string) bool {
	if len(value) != 4 || !asciiDigits(value) {
		return false
	}
	minutes, _ := strconv.Atoi(value[2:])
	return minutes <= 59
}

func validIssueTime(value string) bool {
	if len(value) != 7 || !asciiDigits(value) {
		return false
	}
	day, _ := strconv.Atoi(value[:3])
	hour, _ := strconv.Atoi(value[3:5])
	minute, _ := strconv.Atoi(value[5:])
	return day >= 1 && day <= 366 && hour <= 23 && minute <= 59
}

func asciiDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func canonicalJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
