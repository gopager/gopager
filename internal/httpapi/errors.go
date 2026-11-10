package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// writeManagementError maps underlying domain causes before safe infrastructure wrappers.
func writeManagementError(w http.ResponseWriter, err error) {
	code, fallback, status, retryable := "INTERNAL_ERROR", "The operation could not be completed.", http.StatusInternalServerError, false
	var failure *platform.Failure
	var limited *identity.RateLimited
	var retryAfter *int64
	var unavailableTarget *delivery.TargetUnavailableError
	var dependency *delivery.DependencyError
	var uncertain *delivery.UncertainError
	parameters := map[string]any{}
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
	case errors.Is(err, authz.ErrRevisionConflict), errors.Is(err, assignments.ErrRuleConflict), errors.Is(err, units.ErrAgencyInUse),
		errors.Is(err, ingest.ErrNetworkInUse):
		code, fallback, status, retryable = "REVISION_CONFLICT", "This item changed. Reload before saving.", http.StatusConflict, true
	case errors.As(err, &limited):
		seconds := limited.Seconds()
		code, fallback, status, retryable, retryAfter = "RATE_LIMITED", "Try again later.", http.StatusTooManyRequests, true, &seconds
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	case errors.Is(err, delivery.ErrDependencyUnavailable):
		code, fallback, status = "DEPENDENCY_UNAVAILABLE", "Required data or a required service is unavailable.", http.StatusConflict
		if errors.As(err, &dependency) {
			parameters["dependency"] = dependency.Dependency
		}
	case errors.Is(err, delivery.ErrExternalUncertain):
		code, fallback, status = "EXTERNAL_UNCERTAIN", "The external outcome is uncertain; send it again only deliberately.", http.StatusConflict
		if errors.As(err, &uncertain) {
			parameters["deliveryId"] = uncertain.DeliveryID.String()
		}
	case errors.As(err, &unavailableTarget):
		code, fallback = string(platform.CodeAuthorityUnavailable), platform.NewFailure(platform.CodeAuthorityUnavailable, nil).Error()
		status = platform.HTTPStatus(platform.CodeAuthorityUnavailable)
		parameters["status"], parameters["instanceId"] = unavailableTarget.Status(), unavailableTarget.InstanceID.String()
	case errors.As(err, &failure):
		code, fallback, retryable = string(failure.Code()), failure.Error(), failure.Retryable()
		status = platform.HTTPStatus(failure.Code())
	}
	requestID := uuid.NewString()
	platform.LogErrorResponse(code, status, requestID)
	writeManagementJSON(w, status, struct {
		SchemaVersion     int            `json:"schemaVersion"`
		Code              string         `json:"code"`
		Parameters        map[string]any `json:"parameters"`
		TextKey           string         `json:"textKey"`
		Fallback          string         `json:"fallback"`
		RequestID         string         `json:"requestId"`
		Retryable         bool           `json:"retryable"`
		RetryAfterSeconds *int64         `json:"retryAfterSeconds,omitempty"`
	}{1, code, parameters, "error." + code, fallback, requestID, retryable, retryAfter})
}
