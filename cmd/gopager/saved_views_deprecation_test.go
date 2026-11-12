package main

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/testutil/attributeschemas"
)

// TestSavedViewsReportADeprecatedFilterFieldAsUnavailable expects a saved view whose filter
// references a field that was deprecated after saving to be read successfully with the filter
// marked unavailable: running the view compiles the predicate strictly and would be refused.
func TestSavedViewsReportADeprecatedFilterFieldAsUnavailable(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	attributeschemas.Insert(t, f.store, "incident", map[string]attributes.Field{"level": {Kind: processing.IntegerValue}})
	all := authz.Scope{Kind: "all"}
	reader, _ := f.keyCaller([]string{"message.read"}, all, []string{"payload", "derived"})
	author, _ := f.keyCaller([]string{"saved-view.manage-global"}, all, []string{"payload", "derived"})
	predicate := `{"op":"gte","field":{"namespace":"attributes","path":["incident","level"]},"value":{"kind":"integer","value":"2"}}`
	definition := `{"schemaVersion":1,"filters":{"filter":` + predicate + `}}`
	created := f.view(author, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "global", "G", definition), http.StatusCreated)
	path := "/api/v1/saved-views/" + created.Data.ID.String()
	if got := f.view(reader, http.MethodGet, path, "", http.StatusOK); len(got.Data.UnavailableFilters) != 0 {
		t.Fatalf("unavailable before the deprecation = %v", got.Data.UnavailableFilters)
	}

	f.exec(`UPDATE attribute_schemas SET deprecated = true, revision = revision + 1 WHERE key = 'incident'`)
	f.exec(`UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
	got := f.view(reader, http.MethodGet, path, "", http.StatusOK)
	if len(got.Data.UnavailableFilters) != 1 || got.Data.UnavailableFilters[0] != "filter" {
		t.Fatalf("unavailable after the deprecation = %v, want [filter]", got.Data.UnavailableFilters)
	}
}
