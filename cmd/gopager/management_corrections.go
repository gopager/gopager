package main

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

type createSuppressionMutation struct {
	SchemaVersion int       `json:"schemaVersion"`
	CommandID     uuid.UUID `json:"commandId"`
	assignments.CreateSuppressionInput
}

type liftSuppressionMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

type retryCorrectionMutation struct {
	SchemaVersion int       `json:"schemaVersion"`
	CommandID     uuid.UUID `json:"commandId"`
}

type suppressionCommandResponse struct {
	ID                 uuid.UUID `json:"id"`
	Revision           int64     `json:"revision,string"`
	Status             string    `json:"status"`
	CorrectionID       uuid.UUID `json:"correctionId"`
	AssignmentRevision int64     `json:"assignmentRevision,string"`
}

func (api managementAPI) registerCorrections(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/suppressions", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.rules.ListSuppressions(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/suppressions", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input createSuppressionMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		result, err := api.rules.CreateSuppression(r.Context(), proof, input.CommandID, input.CreateSuppressionInput, check)
		if err != nil {
			return nil, err
		}
		return suppressionCommandResponse{ID: result.Suppression.ID, Revision: result.Suppression.Revision,
			Status: "pending", CorrectionID: result.Correction.ID,
			AssignmentRevision: result.Correction.AssignmentRev}, nil
	}, http.StatusAccepted))
	mux.HandleFunc("GET /api/v1/suppressions/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		result, err := api.rules.ReadSuppression(r.Context(), proof, id)
		if err != nil {
			return nil, err
		}
		return result.Suppression, nil
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/suppressions/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input liftSuppressionMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		result, err := api.rules.LiftSuppression(r.Context(), proof, id, input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return suppressionCommandResponse{ID: result.Suppression.ID, Revision: result.Suppression.Revision,
			Status: "pending", CorrectionID: result.Correction.ID,
			AssignmentRevision: result.Correction.AssignmentRev}, nil
	}, http.StatusAccepted))
	mux.HandleFunc("GET /api/v1/assignment-corrections/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.rules.ReadCorrection(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/assignment-corrections/{id}/retry", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input retryCorrectionMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.rules.RetryCorrection(r.Context(), proof, id, input.CommandID, check)
	}, http.StatusAccepted))
}
