package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// TestManagementErrorLogsCodeWithRequestIDWithoutCause lets an operator correlate a
// client's requestId with the log while keeping the private cause out of it.
func TestManagementErrorLogsCodeWithRequestIDWithoutCause(t *testing.T) { //nolint:paralleltest // replaces the process-wide slog default logger
	for _, test := range []struct { //nolint:paralleltest // replaces the process-wide slog default logger
		name string
		err  error
		code string
	}{
		{"client error", authz.ErrNotFound, "NOT_FOUND"},
		{"server error", platform.NewFailure(platform.CodeStorageUnavailable, errors.New("secret-cause")), "STORAGE_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
			defer slog.SetDefault(previous)
			recorder := httptest.NewRecorder()
			writeManagementError(recorder, test.err)
			var body struct {
				Code      string `json:"code"`
				RequestID string `json:"requestId"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != test.code || body.RequestID == "" {
				t.Fatalf("response %s, %v", recorder.Body.String(), err)
			}
			var record map[string]any
			if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
				t.Fatalf("no single log record: %q, %v", buffer.String(), err)
			}
			if record["code"] != test.code || record["requestId"] != body.RequestID {
				t.Fatalf("log record %v does not carry code and requestId %s", record, body.RequestID)
			}
			if strings.Contains(buffer.String(), "secret-cause") {
				t.Fatal("private cause was logged")
			}
		})
	}
}
