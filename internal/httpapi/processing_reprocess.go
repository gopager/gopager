package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
)

// workflowReprocessMutation is the body of a Reprocess request. The Message-time range is
// half-open: messageTimeFrom is included, messageTimeTo is not.
type workflowReprocessMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	VersionID        uuid.UUID `json:"versionId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	MessageTimeFrom  time.Time `json:"messageTimeFrom"`
	MessageTimeTo    time.Time `json:"messageTimeTo"`
}

// registerReprocess adds the routes that request a historical Reprocess of a Workflow and read
// its requests to mux.
func (api processingWorkflowAPI) registerReprocess(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflows/{id}/reprocess", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowReprocessMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		workflow, err := managementID(r)
		if err != nil {
			return nil, authz.ErrInvalidInput
		}
		if err := checkSchemaVersion(input.SchemaVersion); err != nil {
			return nil, err
		}
		return api.reprocess.Request(r.Context(), proof, workflow, processing.ReprocessInput{
			VersionID: input.VersionID, ExpectedRevision: input.ExpectedRevision,
			MessageTimeFrom: input.MessageTimeFrom, MessageTimeTo: input.MessageTimeTo}, check)
	}, http.StatusAccepted))
	mux.HandleFunc("GET /api/v1/workflows/{id}/reprocesses", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.reprocess.List(r.Context(), proof, workflow, processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/reprocesses/{request}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		request, err := managementPathID(r, "request")
		if err != nil {
			return nil, err
		}
		return api.reprocess.Read(r.Context(), proof, workflow, request)
	}, http.StatusOK))
}
