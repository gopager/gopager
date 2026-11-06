package platform

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
)

// captureLogs routes the default slog logger to a buffer for the test and returns the
// JSON records written so far.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return func() []map[string]any {
		var records []map[string]any
		decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
		for decoder.More() {
			var record map[string]any
			if err := decoder.Decode(&record); err != nil {
				t.Fatal(err)
			}
			records = append(records, record)
		}
		return records
	}
}

func TestAPIUnavailableLogsCodeWithRequestID(t *testing.T) {
	records := captureLogs(t)
	recorder := httptest.NewRecorder()
	writeAPIUnavailable(recorder)
	var body struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.RequestID == "" {
		t.Fatalf("response without requestId: %s, %v", recorder.Body.String(), err)
	}
	logged := records()
	if len(logged) != 1 || logged[0]["code"] != "AUTHORITY_UNAVAILABLE" || logged[0]["requestId"] != body.RequestID {
		t.Fatalf("error response not correlated in the log: %v", logged)
	}
}
