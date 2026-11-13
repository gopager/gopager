package processingstore

import (
	"strings"
	"testing"
)

// TestMarkPossibleQueryIsOneCheapStatement guards the early exit of candidate admission: a
// single statement that asks only whether the Transmission has a marked candidate or any request
// is still active, so ordinary intake issues no further mark statement. The outcome of admission
// with and without a Reprocess is covered by the Reprocess integration tests.
func TestMarkPossibleQueryIsOneCheapStatement(t *testing.T) {
	t.Parallel()
	query := reprocessMarkPossibleQuery
	for _, part := range []string{
		"SELECT EXISTS",
		"reprocess_id IS NOT NULL",
		"processing_reprocess_requests WHERE state IN ('pending', 'running')",
	} {
		if !strings.Contains(query, part) {
			t.Errorf("query lacks %q: %s", part, query)
		}
	}
	if strings.Contains(query, ";") || strings.Contains(strings.ToUpper(query), "UPDATE") || strings.Contains(strings.ToUpper(query), "FOR UPDATE") {
		t.Errorf("the check must be a single read-only statement: %s", query)
	}
}
