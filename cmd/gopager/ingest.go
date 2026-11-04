package main

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
)

type ingestAPI struct {
	service *ingest.Service
}

func registerIngestRoutes(mux *http.ServeMux, service *ingest.Service) {
	api := ingestAPI{service: service}
	mux.HandleFunc("POST /api/v1/transmissions", api.submitTransmission)
	mux.HandleFunc("POST /api/v1/gateway/state", api.reportGatewayState)
}

func (api ingestAPI) submitTransmission(w http.ResponseWriter, r *http.Request) {
	bearer, err := gatewayBearer(r)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeManagementError(w, ingest.ErrInvalidSubmission)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, ingest.MaxSubmissionBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeManagementError(w, platform.NewFailure(platform.CodeResourceLimit, err))
			return
		}
		writeManagementError(w, ingest.ErrInvalidSubmission)
		return
	}
	var submission ingest.Submission
	if err := json.Unmarshal(body, &submission); err != nil {
		writeManagementError(w, ingest.ErrInvalidSubmission)
		return
	}
	receipt, err := api.service.Submit(r.Context(), bearer, submission, len(body))
	if err != nil {
		writeManagementError(w, err)
		return
	}
	status := http.StatusAccepted
	if receipt.ProcessingStatus == "completed" {
		status = http.StatusOK
	}
	writeManagementJSON(w, status, struct {
		SchemaVersion int            `json:"schemaVersion"`
		Data          ingest.Receipt `json:"data"`
	}{1, receipt})
}

func (api ingestAPI) reportGatewayState(w http.ResponseWriter, r *http.Request) {
	bearer, err := gatewayBearer(r)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeManagementError(w, ingest.ErrInvalidGatewayStateReport)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4*1024))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeManagementError(w, platform.NewFailure(platform.CodeResourceLimit, err))
			return
		}
		writeManagementError(w, ingest.ErrInvalidGatewayStateReport)
		return
	}
	var report ingest.GatewayStateReport
	if err := json.Unmarshal(body, &report); err != nil {
		writeManagementError(w, ingest.ErrInvalidGatewayStateReport)
		return
	}
	if err := api.service.ReportState(r.Context(), bearer, report); err != nil {
		writeManagementError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func gatewayBearer(r *http.Request) (string, error) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", identity.ErrLoginRequired
	}
	value := values[0]
	kind, secret, found := strings.Cut(value, " ")
	if !found || !strings.EqualFold(kind, "Bearer") || secret == "" ||
		strings.ContainsAny(secret, " \t\r\n") {
		return "", identity.ErrLoginRequired
	}
	if _, err := identity.SecretHash(secret); err != nil {
		return "", identity.ErrLoginRequired
	}
	return secret, nil
}
