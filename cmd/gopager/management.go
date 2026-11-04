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
	SchemaVersion    int          `json:"schemaVersion"`
	CommandID        uuid.UUID    `json:"commandId"`
	ExpectedRevision int64        `json:"expectedRevision,string"`
	Name             string       `json:"name"`
	CollectionIDs    *[]uuid.UUID `json:"collectionIds"`
}

type collectionMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Name             string    `json:"name"`
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

// input requires explicit full mutable values and leaves reference/Protocol validation to the owner.
func (input ruleMutation) input() (assignments.RuleInput, error) {
	if input.Selector == nil || input.Enabled == nil {
		return assignments.RuleInput{}, authz.ErrInvalidInput
	}
	return assignments.RuleInput{NetworkID: input.NetworkID, UnitID: input.UnitID, Protocol: input.Protocol,
		Selector: *input.Selector, ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil, Enabled: *input.Enabled}, nil
}

// registerManagementRoutes adds only the approved early master-data and current-rule surfaces.
func registerManagementRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, unitService *units.Service, ruleService *assignments.Service) {
	api := managementAPI{transport: transport, units: unitService, rules: ruleService}
	api.registerUnits(mux)
	api.registerCollections(mux)
	api.registerRules(mux)
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

// registerUnits binds complete Unit replacement values, never an implicit membership clear.
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
			if input.SchemaVersion != 1 || input.CollectionIDs == nil {
				return nil, authz.ErrInvalidInput
			}
			values := units.UnitInput{Name: input.Name, CollectionIDs: *input.CollectionIDs}
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
	mux.HandleFunc("POST /api/v1/address-rules/preview", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
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
		if err := api.rules.Preview(r.Context(), proof, input.ID, input.ExpectedRevision, values); err != nil {
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
	case errors.Is(err, ingest.ErrSubmissionConflict):
		code, fallback, status = "IDEMPOTENCY_CONFLICT", "This Submission ID was already used with different content.", http.StatusConflict
	case errors.Is(err, identity.ErrLoginRequired):
		code, fallback, status = "LOGIN_REQUIRED", "Sign in to continue.", http.StatusUnauthorized
	case errors.Is(err, authz.ErrForbidden):
		code, fallback, status = "FORBIDDEN", "This action is not permitted.", http.StatusForbidden
	case errors.Is(err, authz.ErrNotFound):
		code, fallback, status = "NOT_FOUND", "This item is not available.", http.StatusNotFound
	case errors.Is(err, authz.ErrRevisionConflict), errors.Is(err, assignments.ErrRuleConflict):
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
