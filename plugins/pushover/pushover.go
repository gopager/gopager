package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gopager/gopager/pluginsdk"
	"github.com/gopager/gopager/pluginsdk/safehttp"
)

const (
	defaultEndpoint = "https://api.pushover.net/1/messages.json"
	// maxMessageRunes is the Pushover message limit.
	maxMessageRunes = 1024
	maxRequestIDLen = 128
	minPriority     = -2
	maxPriority     = 1
)

// config is the merged instance configuration: administrative values plus the subscriber's.
type config struct {
	APIToken                 string `json:"apiToken"`
	Title                    string `json:"title"`
	Priority                 *int   `json:"priority"`
	Sound                    string `json:"sound"`
	Endpoint                 string `json:"endpoint"`
	AllowPrivateDestinations bool   `json:"allowPrivateDestinations"`
	UserKey                  string `json:"userKey"`
	Device                   string `json:"device"`
}

// message holds the fields of one Pushover notification.
type message struct {
	text, title, url, urlTitle, sound, device string
	priority                                  *int
	truncated                                 bool
}

// pushover delivers notifications through the Pushover API.
type pushover struct {
	// newClient builds the HTTP client for a policy; tests inject their own.
	newClient func(safehttp.Policy) *safehttp.Client
}

// Deliver implements pluginsdk.DeliveryHandler.
func (p *pushover) Deliver(ctx context.Context, request pluginsdk.DeliveryRequest) pluginsdk.DeliveryResult {
	var cfg config
	if err := request.DecodeConfiguration(&cfg); err != nil || cfg.APIToken == "" {
		return failure(pluginsdk.StatusPermanent, "configuration-invalid")
	}
	if cfg.UserKey == "" {
		return failure(pluginsdk.StatusPermanent, "recipient-missing")
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	target, err := safehttp.SecureURL(endpoint, cfg.AllowPrivateDestinations)
	if err != nil {
		if safehttp.IsDenied(err) {
			return safehttp.ErrorResult(err)
		}
		return failure(pluginsdk.StatusPermanent, "configuration-invalid")
	}
	msg, ok := buildMessage(request.Attempt, cfg)
	if !ok {
		return failure(pluginsdk.StatusPermanent, "input-invalid")
	}
	form := msg.form(cfg)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return failure(pluginsdk.StatusPermanent, "configuration-invalid")
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("User-Agent", "GoPager-Pushover/1")

	client := p.newClient(safehttp.Policy{AllowPrivate: cfg.AllowPrivateDestinations})
	response, err := client.Do(httpRequest)
	if err != nil {
		return safehttp.ErrorResult(err)
	}
	return classify(response, msg.truncated, cfg.APIToken, cfg.UserKey)
}

// buildMessage merges the administrative defaults with the attempt's inputs, which win.
// It reports false for an input of the wrong kind or a priority outside -2..1.
func buildMessage(attempt pluginsdk.DeliveryAttempt, cfg config) (message, bool) {
	msg := message{title: cfg.Title, sound: cfg.Sound, device: cfg.Device, priority: cfg.Priority}
	msg.text, msg.truncated = truncateRunes(attempt.Body, maxMessageRunes)
	for name, target := range map[string]*string{
		"title": &msg.title, "url": &msg.url, "urlTitle": &msg.urlTitle, "sound": &msg.sound,
	} {
		value, present := attempt.Inputs[name]
		if !present {
			continue
		}
		text, err := value.String()
		if err != nil {
			return message{}, false
		}
		*target = text
	}
	if value, present := attempt.Inputs["priority"]; present {
		number, err := value.Integer()
		if err != nil {
			return message{}, false
		}
		priority := int(number)
		msg.priority = &priority
	}
	if msg.priority != nil && (*msg.priority < minPriority || *msg.priority > maxPriority) {
		return message{}, false
	}
	return msg, true
}

// form returns the form-encoded request body; empty optional fields are left out.
func (m message) form(cfg config) url.Values {
	form := url.Values{"token": {cfg.APIToken}, "user": {cfg.UserKey}, "message": {m.text}}
	for name, value := range map[string]string{
		"title": m.title, "url": m.url, "url_title": m.urlTitle, "sound": m.sound, "device": m.device,
	} {
		if value != "" {
			form.Set(name, value)
		}
	}
	if m.priority != nil {
		form.Set("priority", strconv.Itoa(*m.priority))
	}
	return form
}

// truncateRunes cuts text to at most limit characters and reports whether it did.
func truncateRunes(text string, limit int) (string, bool) {
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	return string(runes[:limit]), true
}

// apiAnswer is the Pushover response body.
type apiAnswer struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	User    string `json:"user"`
}

// classify maps the Pushover answer to a Delivery result.
func classify(response *safehttp.Response, truncated bool, secrets ...string) pluginsdk.DeliveryResult {
	var answer apiAnswer
	parsed := json.Unmarshal(response.Body, &answer) == nil
	code := response.StatusCode
	var result pluginsdk.DeliveryResult
	switch {
	case code == http.StatusOK && parsed && answer.Status == 1:
		result = failure(pluginsdk.StatusAccepted, "accepted")
	case code == http.StatusTooManyRequests:
		result = safehttp.RateLimitedResult(response.Header)
	case code == http.StatusBadRequest && parsed && answer.Status == 0 && answer.User == "invalid":
		result = failure(pluginsdk.StatusPermanent, "recipient-invalid")
	case code == http.StatusBadRequest && parsed && answer.Status == 0:
		result = failure(pluginsdk.StatusPermanent, "rejected")
	case code >= 500 && code < 600:
		result = failure(pluginsdk.StatusTransient, "provider-unavailable")
	default:
		result = failure(pluginsdk.StatusUncertain, "unexpected-response")
	}
	result.Receipt = receipt(code, answer.Request, truncated, secrets)
	return result
}

// receipt holds the HTTP status, Pushover's request ID when it does not echo a Secret, and
// whether the message was cut.
func receipt(status int, requestID string, truncated bool, secrets []string) json.RawMessage {
	body := struct {
		Status    int    `json:"status"`
		Request   string `json:"request,omitempty"`
		Truncated bool   `json:"truncated,omitempty"`
	}{Status: status, Truncated: truncated}
	if safeRequestID(requestID, secrets) {
		body.Request = requestID
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
