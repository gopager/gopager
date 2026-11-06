package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

type managementAPI struct {
	transport *identity.HTTPTransport
	units     *units.Service
	rules     *assignments.Service
}

type managementOperation func(*http.Request, identity.Proof, func(identity.Authentication) error) (any, error)

type unitMutation struct {
	SchemaVersion    int                      `json:"schemaVersion"`
	CommandID        uuid.UUID                `json:"commandId"`
	ExpectedRevision int64                    `json:"expectedRevision,string"`
	Name             string                   `json:"name"`
	AgencyID         uuid.UUID                `json:"agencyId"`
	CollectionIDs    *[]uuid.UUID             `json:"collectionIds"`
	OrganizationID   json.RawMessage          `json:"organizationId"`
	Settings         *units.Settings          `json:"settings"`
	NewOrganization  *units.OrganizationInput `json:"newOrganization"`
}

func (input unitMutation) organizationID(required bool) (*uuid.UUID, error) {
	if len(input.OrganizationID) == 0 {
		if required {
			return nil, authz.ErrInvalidInput
		}
		return nil, nil
	}
	if string(input.OrganizationID) == "null" {
		return nil, nil
	}
	var id uuid.UUID
	if err := json.Unmarshal(input.OrganizationID, &id); err != nil || id == uuid.Nil {
		return nil, authz.ErrInvalidInput
	}
	return &id, nil
}

type collectionMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Name             string    `json:"name"`
}

// agencyMutation is the create and rename body of an Agency.
type agencyMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Name             string    `json:"name"`
}

type organizationMutation struct {
	SchemaVersion    int             `json:"schemaVersion"`
	CommandID        uuid.UUID       `json:"commandId"`
	ExpectedRevision int64           `json:"expectedRevision,string"`
	Name             string          `json:"name"`
	Defaults         *units.Defaults `json:"defaults"`
}

type templateMutation struct {
	SchemaVersion    int             `json:"schemaVersion"`
	CommandID        uuid.UUID       `json:"commandId"`
	ExpectedRevision int64           `json:"expectedRevision,string"`
	Name             string          `json:"name"`
	OrganizationID   json.RawMessage `json:"organizationId"`
	Settings         *units.Settings `json:"settings"`
}

func (input templateMutation) organizationID(required bool) (*uuid.UUID, error) {
	if len(input.OrganizationID) == 0 {
		if required {
			return nil, authz.ErrInvalidInput
		}
		return nil, nil
	}
	if string(input.OrganizationID) == "null" {
		return nil, nil
	}
	var id uuid.UUID
	if err := json.Unmarshal(input.OrganizationID, &id); err != nil || id == uuid.Nil {
		return nil, authz.ErrInvalidInput
	}
	return &id, nil
}

type fromTemplateMutation struct {
	SchemaVersion int          `json:"schemaVersion"`
	CommandID     uuid.UUID    `json:"commandId"`
	TemplateID    uuid.UUID    `json:"templateId"`
	AgencyID      uuid.UUID    `json:"agencyId"`
	Name          string       `json:"name"`
	CollectionIDs *[]uuid.UUID `json:"collectionIds"`
}

type bulkUnitMutation struct {
	SchemaVersion int                  `json:"schemaVersion"`
	CommandID     uuid.UUID            `json:"commandId"`
	Operation     string               `json:"operation"`
	Items         []units.BulkUnitItem `json:"items"`
}

type deleteMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

type ruleMutation struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	CommandID        uuid.UUID             `json:"commandId"`
	ID               uuid.UUID             `json:"id"`
	ExpectedRevision int64                 `json:"expectedRevision,string"`
	NetworkID        uuid.UUID             `json:"networkId"`
	UnitID           uuid.UUID             `json:"unitId"`
	Protocol         protocols.Protocol    `json:"protocol"`
	Selector         *assignments.Selector `json:"selector"`
	ValidFrom        *time.Time            `json:"validFrom"`
	ValidUntil       *time.Time            `json:"validUntil"`
	Enabled          *bool                 `json:"enabled"`
}

type createSuppressionMutation struct {
	SchemaVersion int       `json:"schemaVersion"`
	CommandID     uuid.UUID `json:"commandId"`
	assignments.CreateSuppressionInput
}

type liftSuppressionMutation struct {
	SchemaVersion              int   `json:"schemaVersion"`
	ExpectedRevision           int64 `json:"expectedRevision,string"`
	ExpectedAssignmentRevision int64 `json:"expectedAssignmentRevision,string"`
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

// input requires explicit full mutable values and leaves reference/Protocol validation to the owner.
func (input ruleMutation) input() (assignments.RuleInput, error) {
	if input.Selector == nil || input.Enabled == nil {
		return assignments.RuleInput{}, authz.ErrInvalidInput
	}
	return assignments.RuleInput{NetworkID: input.NetworkID, UnitID: input.UnitID, Protocol: input.Protocol,
		Selector: *input.Selector, ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil, Enabled: *input.Enabled}, nil
}

// registerManagementRoutes adds the current master-data and assignment management surfaces.
func registerManagementRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, unitService *units.Service, ruleService *assignments.Service) {
	api := managementAPI{transport: transport, units: unitService, rules: ruleService}
	api.registerAgencies(mux)
	api.registerOrganizations(mux)
	api.registerUnitTemplates(mux)
	api.registerUnits(mux)
	api.registerCollections(mux)
	api.registerRules(mux)
	api.registerCorrections(mux)
}

// route selects transport proof before invoking a domain service with transaction-local CSRF checking.
func (api managementAPI) route(operation managementOperation, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proof, err := api.transport.SelectProof(r)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		result, err := operation(r, proof, func(authentication identity.Authentication) error {
			return api.transport.CheckStateChange(r, authentication)
		})
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeManagementJSON(w, status, struct {
			SchemaVersion int `json:"schemaVersion"`
			Data          any `json:"data"`
		}{1, result})
	}
}

// decodeManagementJSON rejects unknown attributes, oversized inputs and trailing JSON documents.
func decodeManagementJSON(r *http.Request, input any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return authz.ErrInvalidInput
	}
	reader := http.MaxBytesReader(nil, r.Body, 64*1024)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return platform.NewFailure(platform.CodeResourceLimit, err)
		}
		return authz.ErrInvalidInput
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return platform.NewFailure(platform.CodeResourceLimit, err)
		}
		return authz.ErrInvalidInput
	}
	return nil
}

// managementPage parses only the supported bounded page parameters, never ignored filters.
func managementPage(r *http.Request) (units.PageInput, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return units.PageInput{}, authz.ErrInvalidInput
	}
	page := units.PageInput{Limit: 50}
	for name, entries := range values {
		if len(entries) != 1 {
			return units.PageInput{}, authz.ErrInvalidInput
		}
		value, err := strconv.Atoi(entries[0])
		if err != nil {
			return units.PageInput{}, authz.ErrInvalidInput
		}
		switch name {
		case "offset":
			page.Offset = value
		case "limit":
			if value < 1 {
				return units.PageInput{}, authz.ErrInvalidInput
			}
			page.Limit = value
		default:
			return units.PageInput{}, authz.ErrInvalidInput
		}
	}
	return page.Validate()
}

// managementID rejects absent/non-UUID resource paths before a repository is called.
func managementID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, authz.ErrInvalidInput
	}
	return id, nil
}

// registerAgencies binds the revisioned Agency lifecycle: list, create, read, rename, delete.
func (api managementAPI) registerAgencies(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agencies", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListAgencies(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/agencies", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input agencyMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.units.CreateAgency(r.Context(), proof, input.CommandID, units.AgencyInput{Name: input.Name}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/agencies/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadAgency(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/agencies/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input agencyMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ChangeAgency(r.Context(), proof, id, input.ExpectedRevision, units.AgencyInput{Name: input.Name}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/agencies/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
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
		return api.units.DeleteAgency(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

// registerOrganizations binds the revisioned Organization lifecycle.
func (api managementAPI) registerOrganizations(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/organizations", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListOrganizations(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/organizations", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input organizationMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil || input.ExpectedRevision != 0 || input.Defaults == nil {
			return nil, authz.ErrInvalidInput
		}
		return api.units.CreateOrganization(r.Context(), proof, input.CommandID,
			units.OrganizationInput{Name: input.Name, Defaults: *input.Defaults}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/organizations/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadOrganization(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/organizations/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input organizationMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil || input.Defaults == nil {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ChangeOrganization(r.Context(), proof, id, input.ExpectedRevision,
			units.OrganizationInput{Name: input.Name, Defaults: *input.Defaults}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/organizations/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
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
		return api.units.DeleteOrganization(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

// registerUnitTemplates binds standalone template management and snapshot operations.
func (api managementAPI) registerUnitTemplates(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/unit-templates", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListUnitTemplates(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/unit-templates", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input templateMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil || input.ExpectedRevision != 0 || input.Settings == nil {
			return nil, authz.ErrInvalidInput
		}
		organizationID, err := input.organizationID(false)
		if err != nil {
			return nil, err
		}
		return api.units.CreateUnitTemplate(r.Context(), proof, input.CommandID, units.UnitTemplateInput{
			Name: input.Name, OrganizationID: organizationID, Settings: *input.Settings,
		}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/unit-templates/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadUnitTemplate(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/unit-templates/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input templateMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil || input.Settings == nil {
			return nil, authz.ErrInvalidInput
		}
		organizationID, err := input.organizationID(true)
		if err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ChangeUnitTemplate(r.Context(), proof, id, input.ExpectedRevision, units.UnitTemplateInput{
			Name: input.Name, OrganizationID: organizationID, Settings: *input.Settings,
		}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/unit-templates/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
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
		return api.units.DeleteUnitTemplate(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

// registerUnits binds complete Unit replacements and bounded bulk operations.
func (api managementAPI) registerUnits(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/units", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListUnits(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/units/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadUnit(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/units/from-template", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input fromTemplateMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CollectionIDs == nil {
			return nil, authz.ErrInvalidInput
		}
		return api.units.CreateUnitFromTemplate(r.Context(), proof, input.CommandID, input.TemplateID,
			units.UnitInput{Name: input.Name, AgencyID: input.AgencyID, CollectionIDs: *input.CollectionIDs}, check)
	}, http.StatusCreated))
	mux.HandleFunc("POST /api/v1/units/bulk/preview", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input bulkUnitMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.units.PreviewBulkUnits(r.Context(), proof, units.BulkUnitInput{
			Operation: input.Operation, Items: input.Items,
		}, check)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/units/bulk", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input bulkUnitMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		results, err := api.units.BulkUnits(r.Context(), proof, input.CommandID,
			units.BulkUnitInput{Operation: input.Operation, Items: input.Items}, check)
		if err != nil {
			return nil, err
		}
		return struct {
			Items     []units.BulkUnitResult `json:"items"`
			CommandID uuid.UUID              `json:"commandId"`
			Operation string                 `json:"operation"`
		}{Items: results, CommandID: input.CommandID, Operation: input.Operation}, nil
	}, http.StatusOK))
	for _, method := range []string{"POST", "PUT"} {
		status, path := http.StatusCreated, "/api/v1/units"
		if method == "PUT" {
			status, path = http.StatusOK, "/api/v1/units/{id}"
		}
		mux.HandleFunc(method+" "+path, api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
			var input unitMutation
			if err := decodeManagementJSON(r, &input); err != nil {
				return nil, err
			}
			if input.SchemaVersion != 1 || input.CollectionIDs == nil || input.Settings == nil {
				return nil, authz.ErrInvalidInput
			}
			organizationID, err := input.organizationID(r.Method == "PUT")
			if err != nil {
				return nil, err
			}
			values := units.UnitInput{Name: input.Name, AgencyID: input.AgencyID, OrganizationID: organizationID,
				CollectionIDs: *input.CollectionIDs, Settings: *input.Settings, NewOrganization: input.NewOrganization}
			if r.Method == "PUT" && input.NewOrganization != nil {
				return nil, authz.ErrInvalidInput
			}
			if r.Method == "POST" {
				if input.ExpectedRevision != 0 {
					return nil, authz.ErrInvalidInput
				}
				return api.units.CreateUnit(r.Context(), proof, input.CommandID, values, check)
			}
			id, err := managementID(r)
			if err != nil || input.CommandID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			return api.units.ChangeUnit(r.Context(), proof, id, input.ExpectedRevision, values, check)
		}, status))
	}
	mux.HandleFunc("DELETE /api/v1/units/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
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
		return api.units.DeleteUnit(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusAccepted))
}

// registerCollections exposes the approved name/membership lifecycle, not general bulk operations.
func (api managementAPI) registerCollections(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/unit-collections", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListCollections(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/unit-collections/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadCollection(r.Context(), proof, id)
	}, http.StatusOK))
	for _, method := range []string{"POST", "PUT"} {
		status, path := http.StatusCreated, "/api/v1/unit-collections"
		if method == "PUT" {
			status, path = http.StatusOK, "/api/v1/unit-collections/{id}"
		}
		mux.HandleFunc(method+" "+path, api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
			var input collectionMutation
			if err := decodeManagementJSON(r, &input); err != nil {
				return nil, err
			}
			if input.SchemaVersion != 1 {
				return nil, authz.ErrInvalidInput
			}
			if r.Method == "POST" {
				if input.ExpectedRevision != 0 {
					return nil, authz.ErrInvalidInput
				}
				return api.units.CreateCollection(r.Context(), proof, input.CommandID, input.Name, check)
			}
			id, err := managementID(r)
			if err != nil || input.CommandID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			return api.units.ChangeCollection(r.Context(), proof, id, input.ExpectedRevision, input.Name, check)
		}, status))
	}
	mux.HandleFunc("DELETE /api/v1/unit-collections/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
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
		return api.units.DeleteCollection(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

// registerRules exposes current configuration and pure preview, never arbitrary public matching.
func (api managementAPI) registerRules(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/address-rules", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.rules.ListRules(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/address-rules/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.rules.ReadRule(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/address-rules/preview", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input ruleMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		values, err := input.input()
		if err != nil {
			return nil, err
		}
		if err := api.rules.Preview(r.Context(), proof, input.ID, input.ExpectedRevision, values, check); err != nil {
			return nil, err
		}
		return struct {
			Valid bool `json:"valid"`
		}{true}, nil
	}, http.StatusOK))
	for _, method := range []string{"POST", "PUT"} {
		status, path := http.StatusCreated, "/api/v1/address-rules"
		if method == "PUT" {
			status, path = http.StatusOK, "/api/v1/address-rules/{id}"
		}
		mux.HandleFunc(method+" "+path, api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
			var input ruleMutation
			if err := decodeManagementJSON(r, &input); err != nil {
				return nil, err
			}
			if input.SchemaVersion != 1 || input.ID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			values, err := input.input()
			if err != nil {
				return nil, err
			}
			if r.Method == "POST" {
				if input.ExpectedRevision != 0 {
					return nil, authz.ErrInvalidInput
				}
				return api.rules.CreateRule(r.Context(), proof, input.CommandID, values, check)
			}
			id, err := managementID(r)
			if err != nil || input.CommandID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			return api.rules.UpdateRule(r.Context(), proof, id, input.ExpectedRevision, values, check)
		}, status))
	}
	mux.HandleFunc("DELETE /api/v1/address-rules/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
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
		result, err := api.rules.DeleteRule(r.Context(), proof, id, input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return units.Deletion{Result: result, Status: "pending"}, nil
	}, http.StatusAccepted))
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
		result, err := api.rules.LiftSuppression(r.Context(), proof, id, input.ExpectedRevision,
			input.ExpectedAssignmentRevision, check)
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

// writeManagementJSON serializes only explicit domain DTOs and surfaces response-write failures.
func writeManagementJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("management response write failed", "code", platform.CodeInternalError)
	}
}

// writeManagementError maps underlying domain causes before safe infrastructure wrappers.
func writeManagementError(w http.ResponseWriter, err error) {
	code, fallback, status, retryable := "INTERNAL_ERROR", "The operation could not be completed.", http.StatusInternalServerError, false
	var failure *platform.Failure
	switch {
	case errors.Is(err, authz.ErrInvalidInput), errors.Is(err, identity.ErrInvalidInput),
		errors.Is(err, assignments.ErrInvalidRule), errors.Is(err, protocols.ErrInvalidTarget),
		errors.Is(err, ingest.ErrInvalidSubmission), errors.Is(err, ingest.ErrInvalidGatewayStateReport):
		code, fallback, status = "INVALID_INPUT", "Check the supplied values.", http.StatusBadRequest
	case errors.Is(err, ingest.ErrSubmissionTooLarge):
		code, fallback, status = "RESOURCE_LIMIT", "The operation exceeds a configured limit.", http.StatusRequestEntityTooLarge
	case errors.Is(err, ingest.ErrSubmissionConflict), errors.Is(err, commands.ErrIdempotencyConflict):
		code, fallback, status = "IDEMPOTENCY_CONFLICT", "This operation identifier was already used differently.", http.StatusConflict
	case errors.Is(err, identity.ErrLoginRequired):
		code, fallback, status = "LOGIN_REQUIRED", "Sign in to continue.", http.StatusUnauthorized
	case errors.Is(err, authz.ErrForbidden):
		code, fallback, status = "FORBIDDEN", "This action is not permitted.", http.StatusForbidden
	case errors.Is(err, authz.ErrNotFound):
		code, fallback, status = "NOT_FOUND", "The requested item is not available.", http.StatusNotFound
	case errors.Is(err, authz.ErrRevisionConflict), errors.Is(err, assignments.ErrRuleConflict), errors.Is(err, units.ErrAgencyInUse):
		code, fallback, status, retryable = "REVISION_CONFLICT", "This item changed. Reload before saving.", http.StatusConflict, true
	case errors.As(err, &failure):
		code, fallback, retryable = string(failure.Code()), failure.Error(), failure.Retryable()
		switch failure.Code() {
		case platform.CodeInvalidInput:
			status = http.StatusBadRequest
		case platform.CodeRevisionConflict, platform.CodeIncompatibleVersion:
			status = http.StatusConflict
		case platform.CodeResourceLimit:
			status = http.StatusRequestEntityTooLarge
		case platform.CodeAuthorityUnavailable, platform.CodeStorageUnavailable, platform.CodeCommitUncertain:
			status = http.StatusServiceUnavailable
		}
	}
	if status >= 500 {
		slog.Error("management operation failed", "code", code)
	}
	writeManagementJSON(w, status, struct {
		SchemaVersion int            `json:"schemaVersion"`
		Code          string         `json:"code"`
		Parameters    map[string]any `json:"parameters"`
		TextKey       string         `json:"textKey"`
		Fallback      string         `json:"fallback"`
		RequestID     string         `json:"requestId"`
		Retryable     bool           `json:"retryable"`
	}{1, code, map[string]any{}, "error." + code, fallback, uuid.NewString(), retryable})
}
