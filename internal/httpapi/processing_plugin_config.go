package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

// personalMutation is the body of PUT .../personal-configuration.
type personalMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	processingmanagement.PersonalInput
}

// commandDelete is the body of the DELETE routes that carry a command ID and an expected revision.
type commandDelete struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
}

// unitParametersMutation is the body of PUT .../unit-parameters/{unitId}.
type unitParametersMutation struct {
	SchemaVersion int                         `json:"schemaVersion"`
	CommandID     uuid.UUID                   `json:"commandId"`
	Values        map[string]processing.Value `json:"values"`
}

// unitParametersDelete is the body of DELETE .../unit-parameters/{unitId}.
type unitParametersDelete struct {
	SchemaVersion int       `json:"schemaVersion"`
	CommandID     uuid.UUID `json:"commandId"`
}

func (api processingWorkflowAPI) registerPersonal(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/instances/{id}/personal-configuration", api.route(func(r *http.Request,
		proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementPathID(r, "id")
		if err != nil {
			return nil, err
		}
		return api.personal.Read(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/instances/{id}/personal-configuration", api.route(func(r *http.Request,
		proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input personalMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementPathID(r, "id")
		if err != nil || input.SchemaVersion != 1 || input.ExpectedRevision < 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.personal.Save(r.Context(), proof, input.CommandID, id, input.ExpectedRevision,
			input.PersonalInput, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/instances/{id}/personal-configuration", api.route(func(r *http.Request,
		proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input commandDelete
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementPathID(r, "id")
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.personal.Delete(r.Context(), proof, input.CommandID, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerUnitParameters(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/instances/{id}/unit-parameters", api.route(func(r *http.Request,
		proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementPathID(r, "id")
		if err != nil {
			return nil, err
		}
		page, filters, err := managementPageWith(r, "unitId")
		if err != nil {
			return nil, err
		}
		var unit *uuid.UUID
		if raw, filtered := filters["unitId"]; filtered {
			parsed, err := uuid.Parse(raw)
			if err != nil || parsed == uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			unit = &parsed
		}
		return api.unitParameters.List(r.Context(), proof, id, unit,
			processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/instances/{id}/unit-parameters/{unitId}", api.route(func(r *http.Request,
		proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input unitParametersMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, idErr := managementPathID(r, "id")
		unit, unitErr := managementPathID(r, "unitId")
		if idErr != nil || unitErr != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.unitParameters.Replace(r.Context(), proof, input.CommandID, id, unit, input.Values, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/instances/{id}/unit-parameters/{unitId}", api.route(func(r *http.Request,
		proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input unitParametersDelete
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, idErr := managementPathID(r, "id")
		unit, unitErr := managementPathID(r, "unitId")
		if idErr != nil || unitErr != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.unitParameters.Delete(r.Context(), proof, input.CommandID, id, unit, check)
	}, http.StatusOK))
}
