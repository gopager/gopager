package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// layoutManager seeds an account allowed to maintain layouts and returns its bearer value.
func (f *deliveryFixture) layoutManager() string {
	bearer, _ := f.caller([]string{"presentation.manage"}, authz.Scope{Kind: "all"}, []string{"payload"})
	return bearer
}

// layoutBody returns a create body for a layout of the output kind with the blocks.
func layoutBody(kind string, blocks []any) map[string]any {
	return map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "name": "Layout " + uuid.NewString(),
		"definition": map[string]any{"schemaVersion": 1, "outputKind": kind, "blocks": blocks}}
}

// textBlock is a literal text block.
func textBlock(text string) any { return map[string]any{"kind": "text", "text": text} }

// layoutCreated posts a layout that must be accepted and returns it.
func (f *deliveryFixture) layoutCreated(bearer string, body map[string]any) presentation.Layout {
	f.t.Helper()
	response := f.subCall(bearer, http.MethodPost, "/api/v1/layouts", body)
	if response.status != http.StatusCreated {
		f.t.Fatalf("layout create returned %d/%s", response.status, response.Code)
	}
	var created presentation.Layout
	if err := json.Unmarshal(response.Data, &created); err != nil {
		f.t.Fatal(err)
	}
	return created
}

// layoutUpdate puts a new definition at the expected revision.
func (f *deliveryFixture) layoutUpdate(bearer string, id uuid.UUID, revision int64, body map[string]any) subResponse {
	f.t.Helper()
	update := map[string]any{"schemaVersion": 1, "expectedRevision": strconv.FormatInt(revision, 10),
		"name": body["name"], "definition": body["definition"]}
	return f.subCall(bearer, http.MethodPut, "/api/v1/layouts/"+id.String(), update)
}

// TestLayoutCreateReplayAndVersionedUpdate proves that create is idempotent per command, that a
// stale revision conflicts, and that an update appends a version while the old one stays
// readable unchanged.
func TestLayoutCreateReplayAndVersionedUpdate(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	manager := f.layoutManager()
	body := layoutBody("text", []any{textBlock("first")})
	created := f.layoutCreated(manager, body)
	if created.Revision != 1 || created.OutputKind != "text" {
		t.Fatalf("created = %+v", created)
	}
	if replay := f.layoutCreated(manager, body); replay.ID != created.ID {
		t.Fatalf("replay created a second layout: %v vs %v", replay.ID, created.ID)
	}

	next := layoutBody("text", []any{textBlock("second")})
	next["name"] = body["name"]
	if got := f.layoutUpdate(manager, created.ID, 9, next); got.status != http.StatusConflict || got.Code != "REVISION_CONFLICT" {
		t.Fatalf("stale update = %d/%s", got.status, got.Code)
	}
	if got := f.layoutUpdate(manager, created.ID, 1, next); got.status != http.StatusOK {
		t.Fatalf("update = %d/%s", got.status, got.Code)
	}
	for version, want := range map[int]string{1: "first", 2: "second"} {
		response := f.subCall(manager, http.MethodGet, "/api/v1/layouts/"+created.ID.String()+"/versions/"+strconv.Itoa(version), nil)
		if response.status != http.StatusOK {
			t.Fatalf("version %d = %d/%s", version, response.status, response.Code)
		}
		var read presentation.LayoutVersion
		if err := json.Unmarshal(response.Data, &read); err != nil {
			t.Fatal(err)
		}
		if len(read.Definition.Blocks) != 1 || read.Definition.Blocks[0].Text != want {
			t.Fatalf("version %d definition = %+v, want text %q", version, read.Definition, want)
		}
	}
	current := f.subCall(manager, http.MethodGet, "/api/v1/layouts/"+created.ID.String(), nil)
	var latest presentation.Layout
	if err := json.Unmarshal(current.Data, &latest); err != nil || latest.Revision != 2 || latest.Definition.Blocks[0].Text != "second" {
		t.Fatalf("current layout = %+v, %v", latest, err)
	}
}

// TestLayoutRightsSeparateWritersReadersAndOthers proves that presentation.manage is needed to
// write, that Subscription managers may read, and that other accounts are refused.
func TestLayoutRightsSeparateWritersReadersAndOthers(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	manager := f.layoutManager()
	created := f.layoutCreated(manager, layoutBody("text", []any{textBlock("x")}))
	own, _ := f.subscriber("payload")
	global, _ := f.caller([]string{"subscription.manage-global"}, authz.Scope{Kind: "all"}, []string{"payload"})
	other, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	path := "/api/v1/layouts/" + created.ID.String()

	for name, bearer := range map[string]string{"manage-own": own, "manage-global": global} {
		if got := f.subCall(bearer, http.MethodGet, path, nil); got.status != http.StatusOK {
			t.Errorf("%s read = %d/%s", name, got.status, got.Code)
		}
		if got := f.subCall(bearer, http.MethodGet, "/api/v1/layouts", nil); got.status != http.StatusOK {
			t.Errorf("%s list = %d/%s", name, got.status, got.Code)
		}
		if got := f.subCall(bearer, http.MethodPost, "/api/v1/layouts", layoutBody("text", []any{textBlock("y")})); got.status != http.StatusForbidden {
			t.Errorf("%s write = %d/%s, want 403", name, got.status, got.Code)
		}
	}
	if got := f.subCall(other, http.MethodGet, path, nil); got.status != http.StatusForbidden {
		t.Errorf("unrelated account read = %d/%s, want 403", got.status, got.Code)
	}
	if got := f.subCall(other, http.MethodGet, "/api/v1/layouts", nil); got.status != http.StatusForbidden {
		t.Errorf("unrelated account list = %d/%s, want 403", got.status, got.Code)
	}
	if got := f.subCall(other, http.MethodPost, "/api/v1/layouts", layoutBody("text", []any{textBlock("y")})); got.status != http.StatusForbidden {
		t.Errorf("unrelated account write = %d/%s, want 403", got.status, got.Code)
	}
}

// TestMarkdownLayoutEscapesPayloadMetacharacters proves that a Subscription with a Markdown
// layout sends the Payload with every Markdown metacharacter escaped, so Payload text cannot
// inject links or emphasis.
func TestMarkdownLayoutEscapesPayloadMetacharacters(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	layout := f.layoutCreated(f.layoutManager(), layoutBody("markdown", []any{
		map[string]any{"kind": "field", "label": "Text", "field": map[string]any{"namespace": "message", "path": []string{"text"}}}}))
	f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": layout.ID}))
	message := f.message(f.now().Add(-time.Minute), "*FIRE* [x](javascript:alert(1))", f.unit("Engine 1", nil))
	f.messageFact("message.created", message, nil)

	f.fanOutAll()
	f.attempt()
	calls := f.targets.calls()
	if len(calls) != 1 || calls[0].Input.OutputKind != "markdown" {
		t.Fatalf("invocations = %+v", calls)
	}
	body := calls[0].Input.Body
	if !strings.Contains(body, `\*FIRE\* \[x\]\(javascript:alert\(1\)\)`) {
		t.Fatalf("Payload metacharacters are not escaped: %q", body)
	}
	if strings.Contains(body, "[x](") || strings.Contains(body, "(javascript:") && !strings.Contains(body, `\(javascript:`) {
		t.Fatalf("body still contains active Markdown: %q", body)
	}
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].LayoutID == nil || *listed[0].LayoutID != layout.ID || listed[0].LayoutVersion != 1 {
		t.Fatalf("Delivery does not record the rendered layout: %+v", listed)
	}
}

// TestLayoutInUseCannotBeDeleted proves that a layout referenced by a Subscription cannot be
// deleted until the reference is gone.
func TestLayoutInUseCannotBeDeleted(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	manager := f.layoutManager()
	instance := f.instance(true)
	layout := f.layoutCreated(manager, layoutBody("text", []any{textBlock("x")}))
	subscription := f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": layout.ID}))
	remove := func() subResponse {
		return f.subCall(manager, http.MethodDelete, "/api/v1/layouts/"+layout.ID.String(),
			map[string]any{"schemaVersion": 1, "expectedRevision": "1"})
	}

	if got := remove(); got.status != http.StatusConflict || got.Code != "REVISION_CONFLICT" {
		t.Fatalf("delete of a referenced layout = %d/%s", got.status, got.Code)
	}
	if got := f.subDelete(bearer, subscription.ID, 1); got.status != http.StatusOK {
		t.Fatalf("delete Subscription = %d/%s", got.status, got.Code)
	}
	if got := remove(); got.status != http.StatusOK {
		t.Fatalf("delete of an unreferenced layout = %d/%s", got.status, got.Code)
	}
	if got := f.subCall(manager, http.MethodGet, "/api/v1/layouts/"+layout.ID.String(), nil); got.status != http.StatusNotFound {
		t.Fatalf("deleted layout read = %d/%s", got.status, got.Code)
	}
}

// TestSubscriptionLayoutOutputKindMustBeSupportedByEveryTarget proves that a layout whose output
// kind a target's descriptor does not offer is refused at save time.
func TestSubscriptionLayoutOutputKindMustBeSupportedByEveryTarget(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	manager := f.layoutManager()
	instance := f.instance(true)
	f.targets.mu.Lock()
	f.targets.descriptor = func(configured processing.InstanceConfiguration) delivery.TargetDescriptor {
		return delivery.TargetDescriptor{Version: delivery.DescriptorVersion, InstanceID: configured.ID,
			InstanceRevision: configured.Revision, CapabilityKind: delivery.CapabilityDelivery, ConfigurationAuthority: "admin",
			OutputKinds: []string{processing.LayoutText},
			Inputs:      []delivery.InputField{{Name: "recipient", Kind: processing.StringValue, Required: true, MaxBytes: 256}}}
	}
	f.targets.mu.Unlock()
	markdown := f.layoutCreated(manager, layoutBody("markdown", []any{textBlock("x")}))
	text := f.layoutCreated(manager, layoutBody("text", []any{textBlock("x")}))

	f.subRejected(bearer, subInput(instance, map[string]any{"layoutId": markdown.ID}), http.StatusBadRequest, "INVALID_INPUT")
	f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": text.ID}))
}
