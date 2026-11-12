package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gopager/gopager/pluginsdk"
	"github.com/gopager/gopager/pluginsdk/safehttp"
)

const (
	userAgent       = "GoPager-Webhook/1"
	maxRequestIDLen = 128
)

// config is the merged instance configuration.
type config struct {
	URL                      string `json:"url"`
	BearerToken              string `json:"bearerToken"`
	HMACSecret               string `json:"hmacSecret"`
	FollowRedirects          bool   `json:"followRedirects"`
	AllowPrivateDestinations bool   `json:"allowPrivateDestinations"`
}

// webhook delivers notifications as signed JSON POST requests.
type webhook struct {
	// newClient builds the HTTP client for a policy; tests inject their own.
	newClient func(safehttp.Policy) *safehttp.Client
	// now supplies the signature timestamp; nil means time.Now.
	now func() time.Time
}

// Deliver implements pluginsdk.DeliveryHandler.
func (w *webhook) Deliver(ctx context.Context, request pluginsdk.DeliveryRequest) pluginsdk.DeliveryResult {
	var cfg config
	if err := request.DecodeConfiguration(&cfg); err != nil {
		return failure(pluginsdk.StatusPermanent, "configuration-invalid")
	}
	target, err := safehttp.SecureURL(cfg.URL, cfg.AllowPrivateDestinations)
	if err != nil {
		if safehttp.IsDenied(err) {
			return safehttp.ErrorResult(err)
		}
		return failure(pluginsdk.StatusPermanent, "configuration-invalid")
	}
	payload, ok := buildPayload(request)
	if !ok {
		return failure(pluginsdk.StatusPermanent, "input-invalid")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return failure(pluginsdk.StatusPermanent, "configuration-invalid")
	}
	w.setHeaders(httpRequest, request.Attempt, cfg, payload)

	client := w.newClient(safehttp.Policy{
		AllowPrivate:     cfg.AllowPrivateDestinations,
		FollowRedirects:  cfg.FollowRedirects,
		SensitiveHeaders: []string{"X-GoPager-Signature", "X-GoPager-Timestamp"},
	})
	response, err := client.Do(httpRequest)
	if err != nil {
		return safehttp.ErrorResult(err)
	}
	return classify(response, cfg)
}

// setHeaders sets the content, identification, authorization and signature headers.
func (w *webhook) setHeaders(r *http.Request, attempt pluginsdk.DeliveryAttempt, cfg config, payload []byte) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", userAgent)
	r.Header.Set("X-GoPager-Delivery", attempt.DeliveryID)
	r.Header.Set("X-GoPager-Attempt", attempt.AttemptID)
	if cfg.BearerToken != "" {
		r.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
	}
	if cfg.HMACSecret == "" {
		return
	}
	now := w.now
	if now == nil {
		now = time.Now
	}
	timestamp := strconv.FormatInt(now().Unix(), 10)
	r.Header.Set("X-GoPager-Timestamp", timestamp)
	r.Header.Set("X-GoPager-Signature", sign(cfg.HMACSecret, timestamp, payload))
}

// sign returns "sha256=" and the lowercase hex HMAC-SHA256 of timestamp, ".", and body.
func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// buildPayload encodes the request body. It reports false when the body of a json
// notification is not valid JSON or an input cannot be converted.
func buildPayload(request pluginsdk.DeliveryRequest) ([]byte, bool) {
	attempt := request.Attempt
	var body json.RawMessage
	if attempt.OutputKind == "json" {
		if !json.Valid([]byte(attempt.Body)) {
			return nil, false
		}
		body = json.RawMessage(attempt.Body)
	} else {
		encoded, err := json.Marshal(attempt.Body)
		if err != nil {
			return nil, false
		}
		body = encoded
	}
	inputs := make(map[string]json.RawMessage, len(attempt.Inputs))
	for name, value := range attempt.Inputs {
		plain, err := value.Plain()
		if err != nil {
			return nil, false
		}
		inputs[name] = plain
	}
	payload, err := json.Marshal(struct {
		DeliveryID    string                     `json:"deliveryId"`
		AttemptID     string                     `json:"attemptId"`
		AttemptNumber int                        `json:"attemptNumber"`
		InstanceID    string                     `json:"instanceId"`
		CorrelationID *string                    `json:"correlationId,omitempty"`
		CausationID   string                     `json:"causationId"`
		OutputKind    string                     `json:"outputKind"`
		Body          json.RawMessage            `json:"body"`
		Inputs        map[string]json.RawMessage `json:"inputs"`
	}{attempt.DeliveryID, attempt.AttemptID, attempt.AttemptNumber, request.InstanceID,
		attempt.CorrelationID, attempt.CausationID, attempt.OutputKind, body, inputs})
	return payload, err == nil
}

// classify maps the receiver's answer to a Delivery result.
func classify(response *safehttp.Response, cfg config) pluginsdk.DeliveryResult {
	status := response.StatusCode
	var result pluginsdk.DeliveryResult
	switch {
	case status >= 200 && status < 300:
		result = failure(pluginsdk.StatusAccepted, "accepted")
	case status == http.StatusTooManyRequests:
		result = safehttp.RateLimitedResult(response.Header)
	case status == http.StatusRequestTimeout, status == http.StatusServiceUnavailable:
		result = failure(pluginsdk.StatusTransient, "provider-unavailable")
	case status >= 300 && status < 400 && !cfg.FollowRedirects:
		result = failure(pluginsdk.StatusPermanent, "redirect-refused")
	case status >= 400 && status < 500:
		result = failure(pluginsdk.StatusPermanent, "rejected")
	default:
		result = failure(pluginsdk.StatusUncertain, "unexpected-response")
	}
	result.Receipt = receipt(status, response.Header.Get("X-Request-Id"), cfg.BearerToken, cfg.HMACSecret)
	return result
}

// receipt holds only the HTTP status and a provider request ID that does not echo a Secret.
func receipt(status int, requestID string, secrets ...string) json.RawMessage {
	body := struct {
		Status    int    `json:"status"`
		RequestID string `json:"requestId,omitempty"`
	}{Status: status}
	if safeRequestID(requestID, secrets) {
		body.RequestID = requestID
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	return encoded
}

// safeRequestID reports whether id is short, printable and free of every non-empty secret.
func safeRequestID(id string, secrets []string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, r := range id {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(id, secret) {
			return false
		}
	}
	return true
}

// failure returns a result without a receipt.
func failure(status, code string) pluginsdk.DeliveryResult {
	return pluginsdk.DeliveryResult{Status: status, Code: code}
}
