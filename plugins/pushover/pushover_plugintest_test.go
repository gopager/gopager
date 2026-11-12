package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/pluginsdk/plugintest"
)

// fakeMessagesAPI is a local Pushover endpoint that records the forms it receives and answers
// according to the message text.
type fakeMessagesAPI struct {
	*httptest.Server
	mu    sync.Mutex
	forms []url.Values
}

// newFakeMessagesAPI starts the endpoint: the messages "reject", "limited" and "down" select the
// provider's error answers, anything else is accepted.
func newFakeMessagesAPI(t *testing.T) *fakeMessagesAPI {
	t.Helper()
	api := &fakeMessagesAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		api.mu.Lock()
		api.forms = append(api.forms, r.PostForm)
		api.mu.Unlock()
		switch r.PostForm.Get("message") {
		case "reject":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":0,"errors":["message cannot be blank"],"request":"r"}`))
		case "limited":
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
		case "down":
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`{"status":1,"request":"req-1"}`))
		}
	}))
	t.Cleanup(api.Close)
	return api
}

// received returns a copy of the forms seen so far.
func (a *fakeMessagesAPI) received() []url.Values {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]url.Values(nil), a.forms...)
}

// newPushoverHarness runs the real pushover executable against the fake endpoint.
func newPushoverHarness(t *testing.T, api *fakeMessagesAPI) *plugintest.Harness {
	t.Helper()
	h := plugintest.New(t, plugintest.Options{
		Executable:           plugintest.BuildPlugin(t, ".", "."),
		ManifestTemplatePath: "manifest.template.json",
	})
	h.Configure(map[string]any{"endpoint": api.URL, "allowPrivateDestinations": true},
		map[string]plugintest.Secret{"apiToken": plugintest.SecretValue("app-token-secret")})
	return h
}

func TestPushoverEachSubscriberAttemptCarriesOnlyTheirOwnUserKey(t *testing.T) {
	t.Parallel()
	api := newFakeMessagesAPI(t)
	h := newPushoverHarness(t, api)
	alice, bob := h.NewSubscriber(), h.NewSubscriber()
	h.Personal(alice, map[string]any{"device": "phone"}, map[string]plugintest.Secret{"userKey": plugintest.SecretValue("alice-user-key")})
	h.Personal(bob, nil, map[string]plugintest.Secret{"userKey": plugintest.SecretValue("bob-user-key")})
	for _, recipient := range []struct {
		id  uuid.UUID
		key string
	}{{alice, "alice-user-key"}, {bob, "bob-user-key"}} {
		before := len(api.received())
		outcome, err := h.Attempt(plugintest.AttemptInput{SubscriberID: recipient.id, Body: "hello"})
		if err != nil || outcome.Status != processing.AttemptAccepted || outcome.Code != "accepted" {
			t.Fatalf("%+v %v", outcome, err)
		}
		forms := api.received()
		if len(forms) != before+1 {
			t.Fatalf("expected exactly one more request, got %d", len(forms)-before)
		}
		form := forms[before]
		if form.Get("user") != recipient.key || form.Get("token") != "app-token-secret" {
			t.Fatalf("subscriber %s sent user %q", recipient.id, form.Get("user"))
		}
		for _, other := range []string{"alice-user-key", "bob-user-key"} {
			if other != recipient.key && strings.Contains(form.Encode(), other) {
				t.Fatalf("the request carries the other subscriber's key %q", other)
			}
		}
	}
}

func TestPushoverSubscriberWithoutPersonalValuesHasNoRecipient(t *testing.T) {
	t.Parallel()
	api := newFakeMessagesAPI(t)
	h := newPushoverHarness(t, api)
	outcome, err := h.Attempt(plugintest.AttemptInput{Body: "hello"})
	if err != nil || outcome.Status != processing.AttemptPermanent || outcome.Code != "recipient-missing" {
		t.Fatalf("%+v %v", outcome, err)
	}
	if got := len(api.received()); got != 0 {
		t.Fatalf("%d requests reached the provider", got)
	}
}

func TestPushoverProviderAnswersAreMappedByTheCore(t *testing.T) {
	t.Parallel()
	api := newFakeMessagesAPI(t)
	h := newPushoverHarness(t, api)
	h.Personal(h.Subscriber(), nil, map[string]plugintest.Secret{"userKey": plugintest.SecretValue("some-user-key")})
	cases := []struct {
		body       string
		status     string
		code       string
		retryAfter time.Duration
	}{
		{"hello", processing.AttemptAccepted, "accepted", 0},
		{"reject", processing.AttemptPermanent, "rejected", 0},
		{"limited", processing.AttemptTransient, "rate-limited", 90 * time.Second},
		{"down", processing.AttemptTransient, "provider-unavailable", 0},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			outcome, err := h.Attempt(plugintest.AttemptInput{Body: tc.body})
			if err != nil || outcome.Status != tc.status || outcome.Code != tc.code || outcome.RetryAfter != tc.retryAfter {
				t.Fatalf("%+v %v", outcome, err)
			}
			for _, secret := range []string{"app-token-secret", "some-user-key"} {
				if strings.Contains(string(outcome.Receipt), secret) {
					t.Fatalf("a Secret leaked into the receipt %s", outcome.Receipt)
				}
			}
		})
	}
}
