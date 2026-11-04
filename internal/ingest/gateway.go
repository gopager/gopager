package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
)

var ErrInvalidGatewayStateReport = errors.New("invalid Gateway state report")

type GatewayStateReport struct {
	SchemaVersion int    `json:"schemaVersion"`
	Operation     string `json:"operation"`
	ReasonCode    string `json:"reasonCode,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

func (report *GatewayStateReport) UnmarshalJSON(data []byte) error {
	members, err := jsonObjectMembers(data)
	if err != nil {
		return ErrInvalidGatewayStateReport
	}
	for name := range members {
		if !slices.Contains([]string{"schemaVersion", "operation", "reasonCode", "detail"}, name) {
			return ErrInvalidGatewayStateReport
		}
	}
	for _, name := range []string{"reasonCode", "detail"} {
		if value, found := members[name]; found && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrInvalidGatewayStateReport
		}
	}
	type plain GatewayStateReport
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return ErrInvalidGatewayStateReport
	}
	*report = GatewayStateReport(decoded)
	return report.Validate()
}

func (report GatewayStateReport) Validate() error {
	if report.SchemaVersion != 1 || !slices.Contains([]string{"heartbeat", "degraded", "recovered", "terminated"}, report.Operation) {
		return ErrInvalidGatewayStateReport
	}
	if report.Operation != "degraded" {
		if report.ReasonCode != "" || report.Detail != "" {
			return ErrInvalidGatewayStateReport
		}
		return nil
	}
	if len(report.ReasonCode) == 0 || len(report.ReasonCode) > 64 || !utf8.ValidString(report.ReasonCode) ||
		strings.TrimSpace(report.ReasonCode) != report.ReasonCode {
		return ErrInvalidGatewayStateReport
	}
	for _, character := range []byte(report.ReasonCode) {
		if character < 0x21 || character > 0x7e {
			return ErrInvalidGatewayStateReport
		}
	}
	if len([]byte(report.Detail)) > 512 || !utf8.ValidString(report.Detail) {
		return ErrInvalidGatewayStateReport
	}
	return nil
}

func jsonObjectMembers(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalidGatewayStateReport
	}
	members := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidGatewayStateReport
		}
		name, ok := token.(string)
		if !ok {
			return nil, ErrInvalidGatewayStateReport
		}
		if _, exists := members[name]; exists {
			return nil, ErrInvalidGatewayStateReport
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidGatewayStateReport
		}
		members[name] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalidGatewayStateReport
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidGatewayStateReport
	}
	return members, nil
}
