package processing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

func TestDeliveryAttemptOutcomes(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	subscriber := uuid.New()
	cases := []struct {
		body       string
		status     string
		code       string
		retryAfter time.Duration
		receipt    string
	}{
		{body: "accept", status: AttemptAccepted, code: "ok", receipt: `{"ok":true}`},
		{body: "transient", status: AttemptTransient, code: "provider-busy", retryAfter: 30 * time.Second},
		{body: "permanent", status: AttemptPermanent, code: "rejected"},
		{body: "uncertain", status: AttemptUncertain, code: "provider-timeout"},
		{body: "slow", status: AttemptAccepted, code: "ok"},
		{body: "bad-code", status: AttemptAccepted, code: CodeInvalidCode},
	}
	for _, tc := range cases {
		outcome, err := host.Attempt(t.Context(), env.request(instance, subscriber, tc.body))
		if err != nil {
			t.Fatalf("%s: %v", tc.body, err)
		}
		if outcome.Status != tc.status || outcome.Code != tc.code || outcome.RetryAfter != tc.retryAfter ||
			string(outcome.Receipt) != tc.receipt {
			t.Errorf("%s: got %+v", tc.body, outcome)
		}
	}
}

func TestDeliveryAttemptWireRequest(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	request := env.request(instance, uuid.New(), "echo-request")
	correlation := uuid.New()
	request.CorrelationID = &correlation
	request.Inputs = map[string]Value{"priority": {Kind: IntegerValue, Value: []byte(`"2"`)}}
	outcome, err := host.Attempt(t.Context(), request)
	if err != nil || outcome.Status != AttemptAccepted {
		t.Fatalf("%+v %v", outcome, err)
	}
	echoed := mustJSON(t, outcome.Receipt)
	if echoed["kind"] != "delivery" || echoed["invocationId"] != request.AttemptID.String() ||
		echoed["instanceId"] != instance.ID.String() || echoed["instanceRevision"] != "1" ||
		echoed["artifactDigest"] != env.pin.Digest || echoed["capabilityId"] != "send" {
		t.Fatalf("wrong envelope: %v", echoed)
	}
	if _, err := time.Parse(time.RFC3339Nano, echoed["deadline"].(string)); err != nil {
		t.Fatal(err)
	}
	attempt := echoed["attempt"].(map[string]any)
	if attempt["deliveryId"] != request.DeliveryID.String() || attempt["attemptId"] != request.AttemptID.String() ||
		attempt["attemptNumber"] != float64(1) || attempt["subscriberId"] != request.SubscriberID.String() ||
		attempt["correlationId"] != correlation.String() || attempt["causationId"] != request.CausationID.String() ||
		attempt["outputKind"] != "text" || attempt["body"] != "echo-request" {
		t.Fatalf("wrong attempt: %v", attempt)
	}
	if attempt["inputs"].(map[string]any)["priority"] == nil {
		t.Fatalf("inputs missing: %v", attempt)
	}
	versions := echoed["secretVersions"].(map[string]any)
	if len(versions) != 1 || !strings.HasPrefix(versions["apiToken"].(string), "secret:") {
		t.Fatalf("wrong secret versions: %v", versions)
	}
}

func TestDeliveryAttemptMergesPersonalConfiguration(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("Shared", "")
	userA, userB, userC := uuid.New(), uuid.New(), uuid.New()
	for user, secret := range map[uuid.UUID]string{userA: "user-A-key", userB: "user-B-key"} {
		owner := user
		env.setPersonal(instance.ID, user, map[string]Value{"userKey": stringLeaf(t, env.sealed(instance.ID, &owner, secret))})
	}
	summaries := map[uuid.UUID]map[string]any{}
	for _, user := range []uuid.UUID{userA, userB, userC} {
		outcome, err := host.Attempt(t.Context(), env.request(instance, user, "config-summary"))
		if err != nil || outcome.Status != AttemptAccepted {
			t.Fatalf("%+v %v", outcome, err)
		}
		summaries[user] = mustJSON(t, outcome.Receipt)
	}
	for user, want := range map[uuid.UUID]string{userA: "user-A", userB: "user-B"} {
		got := summaries[user]
		if got["title"] != "Shared" || got["apiToken"].(map[string]any)["head"] != "admin-" ||
			got["userKey"].(map[string]any)["head"] != want {
			t.Errorf("user %s: %v", want, got)
		}
	}
	if summaries[userC]["userKey"] != nil || summaries[userC]["title"] != "Shared" {
		t.Errorf("subscriber without personal values: %v", summaries[userC])
	}
}

func TestDeliveryAttemptIgnoresPersonalValuesForAdminProperties(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("Admin", "")
	user := uuid.New()
	env.setPersonal(instance.ID, user, map[string]Value{"title": stringLeaf(t, "Shadow"), "unknown": stringLeaf(t, "x")})
	outcome, err := host.Attempt(t.Context(), env.request(instance, user, "config-summary"))
	if err != nil || mustJSON(t, outcome.Receipt)["title"] != "Admin" {
		t.Fatalf("personal value shadowed an administrative one: %+v %v", outcome, err)
	}
}

func TestDeliveryAttemptRedactsReceiptWithSecret(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	outcome, err := host.Attempt(t.Context(), env.request(instance, uuid.New(), "echo-config"))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != AttemptAccepted || outcome.Code != CodeReceiptRedacted || outcome.Receipt != nil {
		t.Fatalf("receipt with Secret was not dropped: %+v", outcome)
	}
}

func TestReceiptAllowed(t *testing.T) {
	t.Parallel()
	secrets := map[string][]byte{"a": []byte("s3cr\"et/<x>")}
	for name, tc := range map[string]struct {
		receipt string
		allowed bool
	}{
		"clean":         {`{"ok":true}`, true},
		"raw":           {`{"v":"s3cr\"et/<x>"}`, false},
		"escaped quote": {`{"v":"s3cr\"et/<x>"}`, false},
		"html escaped":  {`{"v":"s3cr\"et/<x>"}`, false},
		"slash escaped": {`{"v":"s3cr\"et\/<x>"}`, false},
		"unicode":       {`{"v":"s3cr\"et/<x>"}`, false},
		"as key":        {`{"s3cr\"et/<x>":1}`, false},
		"not json":      {`{nope`, false},
		"too large":     {`"` + strings.Repeat("a", MaxReceiptBytes) + `"`, false},
		"at size bound": {`"` + strings.Repeat("a", MaxReceiptBytes-2) + `"`, true},
	} {
		if got := receiptAllowed([]byte(tc.receipt), secrets); got != tc.allowed {
			t.Errorf("%s: got %v", name, got)
		}
	}
}

func TestDeliveryAttemptTransportFailuresAreErrors(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	subscriber := uuid.New()
	for _, body := range []string{"crash", "huge"} {
		if outcome, err := host.Attempt(t.Context(), env.request(instance, subscriber, body)); err == nil {
			t.Errorf("%s produced outcome %+v instead of an error", body, outcome)
		}
	}
	if outcome, err := host.Attempt(t.Context(), env.request(instance, subscriber, "accept")); err != nil || outcome.Status != AttemptAccepted {
		t.Fatalf("plugin did not restart after failures: %+v %v", outcome, err)
	}
}

func TestDeliveryAttemptHangIsKilledAtDeadline(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	subscriber := uuid.New()
	pidOf := func() float64 {
		outcome, err := host.Attempt(t.Context(), env.request(instance, subscriber, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		return mustJSON(t, outcome.Receipt)["pid"].(float64)
	}
	before := pidOf()
	request := env.request(instance, subscriber, "hang")
	request.Deadline = time.Now().Add(700 * time.Millisecond)
	started := time.Now()
	if _, err := host.Attempt(t.Context(), request); err == nil || platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("hang was not a deadline failure: %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("hung plugin was not killed at its deadline")
	}
	if after := pidOf(); after == before {
		t.Fatal("killed process was reused")
	}
	if status := env.host.Status(instance.ID); status.LastFailureCode != StatusDeadlineExceeded {
		t.Fatalf("status %+v", status)
	}
}

func TestDeliveryAttemptUnavailableBeforeSend(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	instance := env.instance("T", "")
	subscriber := uuid.New()
	expect := func(name string, host *DeliveryHost, request DeliveryAttemptRequest, code string) {
		t.Helper()
		outcome, err := host.Attempt(t.Context(), request)
		if err != nil || outcome.Status != AttemptTransient || outcome.Code != code {
			t.Errorf("%s: got %+v %v, want transient %s", name, outcome, err, code)
		}
	}
	missing := env.request(instance, subscriber, "accept")
	missing.InstanceRevision = 9
	expect("missing revision", env.hostFor(nil), missing, CodeConfigurationUnavailable)
	expect("no plugin directory", env.hostFor(func(c *DeliveryHostConfig) { c.Root = nil }),
		env.request(instance, subscriber, "accept"), CodePluginUnavailable)
	expect("no key", env.hostFor(func(c *DeliveryHostConfig) { c.Cipher = nil }),
		env.request(instance, subscriber, "accept"), CodeConfigurationUnavailable)
	if err := env.root.Remove("plugin"); err != nil {
		t.Fatal(err)
	}
	expect("missing artifact", env.hostFor(nil), env.request(instance, subscriber, "accept"), CodePluginUnavailable)
}

func TestDeliveryAttemptSecretSourcesAndFailures(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	subscriber := uuid.New()
	env.env["GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__TOK"] = "env-token-value"
	if err := env.filesRoot.Mkdir("test-delivery", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := env.filesRoot.WriteFile("test-delivery/tok", []byte("file-token-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for reference, head := range map[string]string{"env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__TOK": "env-to", "file:test-delivery/tok": "file-t"} {
		instance := env.instance("T", reference)
		outcome, err := host.Attempt(t.Context(), env.request(instance, subscriber, "config-summary"))
		if err != nil || mustJSON(t, outcome.Receipt)["apiToken"].(map[string]any)["head"] != head {
			t.Fatalf("%s: %+v %v", reference, outcome, err)
		}
	}
	missing := env.instance("T", "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__NOPE")
	outcome, err := host.Attempt(t.Context(), env.request(missing, subscriber, "accept"))
	if err != nil || outcome.Status != AttemptTransient || outcome.Code != CodeConfigurationUnavailable {
		t.Fatalf("unresolvable Secret: %+v %v", outcome, err)
	}
	foreign := env.instance("T", "secret:"+uuid.New().String())
	outcome, err = host.Attempt(t.Context(), env.request(foreign, subscriber, "accept"))
	if err != nil || outcome.Code != CodeConfigurationUnavailable {
		t.Fatalf("missing stored row: %+v %v", outcome, err)
	}
}

func TestDeliveryAttemptRejectsAnotherOwnersSecret(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	owner, intruder := uuid.New(), uuid.New()
	reference := env.sealed(instance.ID, &owner, "owner-only-key")
	env.setPersonal(instance.ID, owner, map[string]Value{"userKey": stringLeaf(t, reference)})
	env.setPersonal(instance.ID, intruder, map[string]Value{"userKey": stringLeaf(t, reference)})
	outcome, err := host.Attempt(t.Context(), env.request(instance, intruder, "accept"))
	if err != nil || outcome.Code != CodeConfigurationUnavailable {
		t.Fatalf("another owner's row was used: %+v %v", outcome, err)
	}
}

func TestDeliveryAttemptSeparatesInstancesOfOnePlugin(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	first, second := env.instance("First", ""), env.instance("Second", "")
	subscriber := uuid.New()
	pids, titles := map[uuid.UUID]float64{}, map[uuid.UUID]any{}
	for _, instance := range []InstanceConfiguration{first, second} {
		outcome, err := host.Attempt(t.Context(), env.request(instance, subscriber, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		pids[instance.ID] = mustJSON(t, outcome.Receipt)["pid"].(float64)
		outcome, err = host.Attempt(t.Context(), env.request(instance, subscriber, "config-summary"))
		if err != nil {
			t.Fatal(err)
		}
		titles[instance.ID] = mustJSON(t, outcome.Receipt)["title"]
	}
	if pids[first.ID] == pids[second.ID] || titles[first.ID] != "First" || titles[second.ID] != "Second" {
		t.Fatalf("instances share a process or configuration: %v %v", pids, titles)
	}
}

func TestDeliveryAttemptRejectsProcessingManifest(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	manifest, _ := deliveryManifestFixture()
	manifest.Capability, manifest.Delivery, manifest.Pure, manifest.SDKMinor = "", nil, true, 0
	manifest.PluginID, manifest.Version, manifest.CapabilityID = env.pin.PluginID, env.pin.Version, env.pin.CapabilityID
	manifest.Artifact.Digest = env.pin.Digest
	writeManifest(t, env.root, manifest)
	instance := env.instance("T", "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__X")
	host := env.hostFor(nil)
	if _, err := host.Describe(t.Context(), instance); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("Describe accepted a processing manifest: %v", err)
	}
	outcome, err := host.Attempt(t.Context(), env.request(instance, uuid.New(), "accept"))
	if err != nil || outcome.Code != CodePluginUnavailable {
		t.Fatalf("Attempt used a processing manifest: %+v %v", outcome, err)
	}
}

func TestDeliveryAttemptValidatesRequest(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	for name, mutate := range map[string]func(*DeliveryAttemptRequest){
		"no attempt":  func(r *DeliveryAttemptRequest) { r.AttemptID = uuid.Nil },
		"no deadline": func(r *DeliveryAttemptRequest) { r.Deadline = time.Time{} },
		"output kind": func(r *DeliveryAttemptRequest) { r.OutputKind = "html" },
		"number":      func(r *DeliveryAttemptRequest) { r.AttemptNumber = 0 },
	} {
		request := env.request(instance, uuid.New(), "accept")
		mutate(&request)
		if _, err := host.Attempt(t.Context(), request); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func TestDeliveryAttemptCancelledByCaller(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := host.Attempt(ctx, env.request(instance, uuid.New(), "accept")); err == nil {
		t.Fatal("cancelled attempt returned an outcome")
	}
}

func TestDeliveryRejectsAnotherPluginsSecretReference(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	env.env["GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN"] = "operator-secret"
	if err := env.filesRoot.Mkdir("pushover", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := env.filesRoot.WriteFile("pushover/token", []byte("operator-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN", "file:pushover/token"} {
		instance := env.instance("T", reference)
		if _, err := host.Describe(t.Context(), instance); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: Describe accepted a foreign reference: %v", reference, err)
		}
		outcome, err := host.Attempt(t.Context(), env.request(instance, uuid.New(), "config-summary"))
		if err != nil || outcome.Status != AttemptTransient || outcome.Code != CodeConfigurationUnavailable {
			t.Errorf("%s: foreign reference resolved: %+v %v", reference, outcome, err)
		}
	}
}
