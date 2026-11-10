package identity

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/platform"
)

// NewHTTPHandler registers local login and current-Session endpoints.
// The platform owns readiness/draining; domain decisions still use primary transactions.
func NewHTTPHandler(transport *HTTPTransport, login *LoginService) (http.Handler, error) {
	if transport == nil || login == nil {
		return nil, ErrInvalidInput
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		proof, err := transport.SelectProof(r)
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		session, err := login.ReadSession(r.Context(), proof)
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		writeIdentityJSON(w, http.StatusOK, struct {
			SchemaVersion int    `json:"schemaVersion"`
			PrincipalID   string `json:"principalId"`
			SessionID     string `json:"sessionId"`
			CSRFToken     string `json:"csrfToken"`
			ExpiresAt     string `json:"expiresAt"`
		}{1, session.PrincipalID, session.SessionID.String(), session.CSRFToken, session.ExpiresAt.Format(time.RFC3339Nano)})
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if err := transport.CheckBrowserOrigin(r); err != nil {
			writeIdentityError(w, err)
			return
		}
		var input struct {
			SchemaVersion int    `json:"schemaVersion"`
			Username      string `json:"username"`
			Password      string `json:"password"`
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeIdentityError(w, ErrInvalidInput)
			return
		}
		reader := http.MaxBytesReader(w, r.Body, 16*1024)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		var tooLarge *http.MaxBytesError
		if err := decoder.Decode(&input); errors.As(err, &tooLarge) {
			writeIdentityError(w, platform.NewFailure(platform.CodeResourceLimit, err))
			return
		} else if err != nil || input.SchemaVersion != 1 {
			writeIdentityError(w, ErrInvalidInput)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); errors.As(err, &tooLarge) {
			writeIdentityError(w, platform.NewFailure(platform.CodeResourceLimit, err))
			return
		} else if !errors.Is(err, io.EOF) {
			writeIdentityError(w, ErrInvalidInput)
			return
		}
		session, err := login.AuthenticateLocal(r.Context(), loginSource(r), input.Username, input.Password)
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		cookie, err := transport.SessionCookie(session.Secret, session.ExpiresAt)
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		http.SetCookie(w, cookie)
		writeIdentityJSON(w, http.StatusOK, struct {
			SchemaVersion int    `json:"schemaVersion"`
			SessionID     string `json:"sessionId"`
			CSRFToken     string `json:"csrfToken"`
			ExpiresAt     string `json:"expiresAt"`
		}{1, session.SessionID.String(), session.CSRFToken, session.ExpiresAt.Format(time.RFC3339Nano)})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		proof, err := transport.SelectProof(r)
		if err == nil {
			err = login.Logout(r.Context(), proof, func(authentication Authentication) error {
				return transport.CheckStateChange(r, authentication)
			})
		}
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		http.SetCookie(w, transport.ClearSessionCookie())
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})
	return mux, nil
}

// loginSource returns the host part of the connection's remote address for the
// per-source login throttle. Forwarding headers are deliberately ignored: any client
// can set them, which would let it pick its own bucket. A reverse proxy must therefore
// preserve the client address at the connection level, or all clients behind it share
// one source bucket.
func loginSource(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeIdentityJSON emits only explicit DTOs and surfaces transport write failures.
func writeIdentityJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("identity response write failed", "code", platform.CodeInternalError)
	}
}

// writeIdentityError maps domain outcomes to the API error response (code, message,
// retry information) without exposing wrapped infrastructure causes.
func writeIdentityError(w http.ResponseWriter, err error) {
	code, fallback, status := "INTERNAL_ERROR", "The operation could not be completed.", http.StatusInternalServerError
	retryable := false
	var retryAfter *int64
	var limited *RateLimited
	var failure *platform.Failure
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, authz.ErrInvalidInput):
		code, fallback, status = "INVALID_INPUT", "Check the supplied values.", http.StatusBadRequest
	case errors.Is(err, commands.ErrIdempotencyConflict):
		code, fallback, status = "IDEMPOTENCY_CONFLICT", "This operation identifier was already used differently.", http.StatusConflict
	case errors.Is(err, ErrLoginRequired):
		code, fallback, status = "LOGIN_REQUIRED", "Sign in to continue.", http.StatusUnauthorized
	case errors.Is(err, authz.ErrForbidden):
		code, fallback, status = "FORBIDDEN", "This action is not permitted.", http.StatusForbidden
	case errors.As(err, &limited):
		code, fallback, status = "RATE_LIMITED", "Try again later.", http.StatusTooManyRequests
		seconds := limited.Seconds()
		retryAfter, retryable = &seconds, true
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	case errors.As(err, &failure):
		code, fallback, retryable = string(failure.Code()), failure.Error(), failure.Retryable()
		status = platform.HTTPStatus(failure.Code())
	}
	requestID := uuid.NewString()
	platform.LogErrorResponse(code, status, requestID)
	writeIdentityJSON(w, status, struct {
		SchemaVersion     int            `json:"schemaVersion"`
		Code              string         `json:"code"`
		Parameters        map[string]any `json:"parameters"`
		TextKey           string         `json:"textKey"`
		Fallback          string         `json:"fallback"`
		RequestID         string         `json:"requestId"`
		Retryable         bool           `json:"retryable"`
		RetryAfterSeconds *int64         `json:"retryAfterSeconds,omitempty"`
	}{1, code, map[string]any{}, "error." + code, fallback, requestID, retryable, retryAfter})
}
