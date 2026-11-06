package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

type workflowPreviewMutation struct {
	SchemaVersion        int                    `json:"schemaVersion"`
	VersionID            uuid.UUID              `json:"versionId"`
	Definition           *processing.Definition `json:"definition"`
	SyntheticJSON        json.RawMessage        `json:"syntheticJson"`
	StoredTransmissionID uuid.UUID              `json:"storedTransmissionId"`
}

type workflowRetryMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

type workflowRetryResponse struct {
	CandidateID uuid.UUID `json:"candidateId"`
	Revision    int64     `json:"revision,string"`
}

func (api processingWorkflowAPI) registerReads(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workflows/{id}/candidates", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.reads.ListCandidateStatuses(r.Context(), proof, workflow,
			processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/candidates/{candidate}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		candidate, err := managementPathID(r, "candidate")
		if err != nil {
			return nil, err
		}
		return api.reads.ReadCandidateStatus(r.Context(), proof, workflow, candidate)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/candidates/{candidate}/result", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		candidate, err := managementPathID(r, "candidate")
		if err != nil {
			return nil, err
		}
		return api.reads.ReadPrivateResultMetadata(r.Context(), proof, workflow, candidate)
	}, http.StatusOK))
}
func (api processingWorkflowAPI) registerRetry(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflows/{id}/candidates/{candidate}/retry", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowRetryMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		parent, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		candidate, err := uuid.Parse(r.PathValue("candidate"))
		if err != nil || candidate == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		retried, err := api.retries.RetryFailed(r.Context(), proof, parent, candidate, input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return workflowRetryResponse{CandidateID: retried.ID, Revision: retried.Revision}, nil
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerPreview(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflows/{id}/preview", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowPreviewMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		workflow, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.preview.Preview(r.Context(), proof, processingmanagement.PreviewInput{
			WorkflowID: workflow, VersionID: input.VersionID, Definition: input.Definition,
			SyntheticJSON: input.SyntheticJSON, StoredTransmissionID: input.StoredTransmissionID,
		}, check)
	}, http.StatusOK))
}
