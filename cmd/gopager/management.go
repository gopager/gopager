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

type deleteMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
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
