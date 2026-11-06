package identity

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
)

func TestIdentityErrorLogsCodeWithRequestID(t *testing.T) { //nolint:paralleltest // replaces the process-wide slog default logger
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	defer slog.SetDefault(previous)
	recorder := httptest.NewRecorder()
	writeIdentityError(recorder, ErrLoginRequired)
	var body struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.RequestID == "" {
		t.Fatalf("response %s, %v", recorder.Body.String(), err)
	}
	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatalf("no single log record: %q, %v", buffer.String(), err)
	}
	if record["code"] != "LOGIN_REQUIRED" || record["requestId"] != body.RequestID {
		t.Fatalf("log record %v does not carry code and requestId %s", record, body.RequestID)
	}
}
