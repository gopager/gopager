package plugintest_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/pluginsdk/plugintest"
)

// newEcho opens a harness around the supplied echo plugin.
func newEcho(t *testing.T) *plugintest.Harness {
	t.Helper()
	return plugintest.New(t, plugintest.Options{
		Executable:           plugintest.BuildPlugin(t, "../..", "./pluginsdk/plugintest/testdata/echo"),
		ManifestTemplatePath: "testdata/echo/manifest.template.json",
	})
}

// receiptOf decodes the receipt of an accepted outcome.
func receiptOf(t *testing.T, outcome processing.DeliveryAttemptOutcome) map[string]any {
	t.Helper()
	var receipt map[string]any
	if err := json.Unmarshal(outcome.Receipt, &receipt); err != nil {
		t.Fatalf("receipt %q: %v", outcome.Receipt, err)
	}
	return receipt
}

func TestDescribeReportsTheManifestDeclaration(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	h.Configure(map[string]any{"title": "T"}, map[string]plugintest.Secret{"token": plugintest.SecretValue("tok-secret")})
	description, err := h.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if !description.Personal || description.SendsPerMinute != 30 || !description.Idempotent ||
		strings.Join(description.OutputKinds, ",") != "text,json" || len(description.Inputs) != 1 {
		t.Fatalf("unexpected description %+v", description)
	}
	if h.Manifest().PluginID != "echo" {
		t.Fatalf("unexpected manifest %+v", h.Manifest())
	}
}

func TestAttemptMergesAdministrativeAndPersonalValuesPerSubscriber(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	h.Configure(map[string]any{"title": "Hello"}, map[string]plugintest.Secret{"token": plugintest.SecretValue("tok-secret")})
	alice, bob := h.NewSubscriber(), h.NewSubscriber()
	h.Personal(alice, nil, map[string]plugintest.Secret{"userKey": plugintest.SecretValue("aaa-key")})
	h.Personal(bob, nil, map[string]plugintest.Secret{"userKey": plugintest.SecretValue("bbb-key")})
	for subscriber, head := range map[string]string{alice.String(): "aaa", bob.String(): "bbb"} {
		outcome, err := h.Attempt(plugintest.AttemptInput{SubscriberID: mustUUID(t, subscriber), Body: "echo",
			Inputs: map[string]any{"priority": 2}})
		if err != nil || outcome.Status != processing.AttemptAccepted {
			t.Fatalf("%+v %v", outcome, err)
		}
		receipt := receiptOf(t, outcome)
		if receipt["userKeyHead"] != head || receipt["tokenHead"] != "tok" || receipt["title"] != "Hello" ||
			receipt["subscriber"] != subscriber || receipt["priorityKind"] != "integer" || receipt["outputKind"] != "text" {
			t.Fatalf("subscriber %s got %v", subscriber, receipt)
		}
	}
}

func TestAttemptWithoutPersonalValuesIsReportedByThePlugin(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	h.Configure(map[string]any{"title": "T"}, map[string]plugintest.Secret{"token": plugintest.SecretValue("tok-secret")})
	outcome, err := h.Attempt(plugintest.AttemptInput{Body: "echo"})
	if err != nil || outcome.Status != processing.AttemptPermanent || outcome.Code != "recipient-missing" {
		t.Fatalf("%+v %v", outcome, err)
	}
}

func TestReconfigurationIncrementsTheRevisionAndChangesSecretVersions(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	subscriber := h.Subscriber()
	h.Personal(subscriber, nil, map[string]plugintest.Secret{"userKey": plugintest.SecretValue("uuu-key")})
	versions := make([]string, 0, 2)
	for _, token := range []string{"one-token", "two-token"} {
		h.Configure(map[string]any{"title": "T"}, map[string]plugintest.Secret{"token": plugintest.SecretValue(token)})
		outcome, err := h.Attempt(plugintest.AttemptInput{Body: "echo"})
		if err != nil {
			t.Fatal(err)
		}
		receipt := receiptOf(t, outcome)
		versions = append(versions, receipt["versions"].(map[string]any)["token"].(string))
		if receipt["tokenHead"] != token[:3] {
			t.Fatalf("got %v", receipt)
		}
	}
	if versions[0] == versions[1] || h.Revision() != 2 {
		t.Fatalf("versions %v revision %d", versions, h.Revision())
	}
}

func TestFileAndEnvironmentSecretsAreReadPerAttempt(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	h.WriteSecretFile("token.txt", "first-token\n")
	h.SetEnv("GOPAGER_PLUGIN_SECRET_USER", "env-user")
	h.Configure(map[string]any{"title": "T"}, map[string]plugintest.Secret{"token": plugintest.SecretFile("token.txt")})
	subscriber := h.Subscriber()
	h.Personal(subscriber, nil, map[string]plugintest.Secret{"userKey": plugintest.SecretEnv("GOPAGER_PLUGIN_SECRET_USER")})
	var versions []string
	for _, content := range []string{"first-token\n", "other-token\n"} {
		h.WriteSecretFile("token.txt", content)
		outcome, err := h.Attempt(plugintest.AttemptInput{Body: "echo"})
		if err != nil {
			t.Fatal(err)
		}
		receipt := receiptOf(t, outcome)
		if receipt["tokenHead"] != content[:3] || receipt["userKeyHead"] != "env" {
			t.Fatalf("got %v", receipt)
		}
		versions = append(versions, receipt["versions"].(map[string]any)["token"].(string))
	}
	if versions[0] == versions[1] || !strings.HasPrefix(versions[0], "file:") {
		t.Fatalf("versions %v", versions)
	}
}

func TestReceiptContainingASecretIsRedactedByTheCore(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	h.Configure(map[string]any{"title": "T"}, map[string]plugintest.Secret{"token": plugintest.SecretValue("tok-secret")})
	outcome, err := h.Attempt(plugintest.AttemptInput{Body: "leak"})
	if err != nil || outcome.Code != processing.CodeReceiptRedacted || len(outcome.Receipt) != 0 {
		t.Fatalf("%+v %v", outcome, err)
	}
}

func TestAttemptPastItsDeadlineReturnsAnError(t *testing.T) {
	t.Parallel()
	h := newEcho(t)
	h.Configure(map[string]any{"title": "T"}, map[string]plugintest.Secret{"token": plugintest.SecretValue("tok-secret")})
	_, err := h.Attempt(plugintest.AttemptInput{Body: "hang", Timeout: 500 * time.Millisecond})
	if err == nil {
		t.Fatal("an attempt that outlives its deadline must surface as an error (uncertain)")
	}
}

func TestOpenRejectsManifestsTheCoreRejects(t *testing.T) {
	t.Parallel()
	template := `{"schemaVersion":2,"sdkMajor":2,"sdkMinor":1,"capability":"delivery","pluginId":"bad",` +
		`"version":"1","capabilityId":"bad","artifact":{"path":"bad/1/bad","digest":"@DIGEST@"},` +
		`"requiredOperations":["reports.read"],` +
		`"delivery":{"outputKinds":["text"],"inputs":[],"dependencies":[],"sendsPerMinute":0,"idempotent":false},` +
		`"configurationSchema":{"kind":"object","properties":{}},` +
		`"configurationPolicy":{"configurationScope":"admin","secret":false}}`
	_, err := plugintest.Open(t, plugintest.Options{
		Executable:       plugintest.BuildPlugin(t, "../..", "./pluginsdk/plugintest/testdata/echo"),
		ManifestTemplate: []byte(template),
	})
	if !errors.Is(err, processing.ErrCapabilityUndeclared) {
		t.Fatalf("got %v", err)
	}
}

// mustUUID parses an identifier the test itself produced.
func mustUUID(t *testing.T, text string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
