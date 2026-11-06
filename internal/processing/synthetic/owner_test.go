package synthetic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

type testTx struct{ ctx context.Context }

func (t testTx) Context() context.Context { return t.ctx }

type testClock struct{ now time.Time }

func (c testClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return c.now, nil
}

type testNetworks struct {
	network Network
	err     error
}

func (n testNetworks) ReadSyntheticNetwork(_ context.Context, _ platform.TransactionContext, id uuid.UUID) (Network, error) {
	if n.err != nil {
		return Network{}, n.err
	}
	if id != n.network.ID {
		return Network{}, authz.ErrNotFound
	}
	return n.network, nil
}

type testMatcher struct {
	unitID *uuid.UUID
	err    error
}

func (m testMatcher) MatchTargets(_ context.Context, _ platform.TransactionContext, _ uuid.UUID,
	_ protocols.Protocol, _ time.Time, targets []protocols.Target) (assignments.MatchResult, error) {
	if m.err != nil {
		return assignments.MatchResult{}, m.err
	}
	result := assignments.MatchResult{Revision: 7, UnitContext: make(map[uuid.UUID]units.Context)}
	for i := range targets {
		resolution := assignments.Resolution{TargetPosition: i, Status: "unassigned"}
		if m.unitID != nil {
			id := *m.unitID
			rule := uuid.New()
			resolution.Status, resolution.UnitID, resolution.AddressRuleID = "assigned", &id, &rule
			result.UnitContext[id] = units.Context{ID: id, Revision: 3, CollectionIDs: []uuid.UUID{uuid.New()}}
		}
		result.Resolutions = append(result.Resolutions, resolution)
	}
	return result, nil
}

type testScope struct {
	calls int
	now   time.Time
}

func (s *testScope) CheckSyntheticPreviewScope(_ context.Context, _ platform.TransactionContext, _ authz.Authority,
	_ uuid.UUID, _ uuid.UUID, _ uuid.UUID, now time.Time) error {
	s.calls++
	s.now = now
	return nil
}

func testAuthority(groups ...string) authz.Authority {
	principal := uuid.New()
	return authz.Authority{PrincipalID: principal.String(), Revision: 1, Active: true,
		FieldPermissions: []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
			SubjectID: principal.String(), SubjectKind: "principal", Fields: groups}}}
}

func testOwner(protocol protocols.Protocol, unitID *uuid.UUID) (*Owner, Network, testTx) {
	network := Network{ID: uuid.New(), Name: "Dispatch", Protocol: protocol}
	tx := testTx{ctx: context.Background()}
	owner, err := NewOwner(testMatcher{unitID: unitID}, testNetworks{network: network},
		testClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}, &testScope{})
	if err != nil {
		panic(err)
	}
	return owner, network, tx
}

func testWorkflow(scope authz.Scope) processing.WorkflowConfiguration {
	return processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Name: "preview", Scope: scope, Enabled: true}
}

func capture(t *testing.T, owner *Owner, network Network, tx testTx, authority authz.Authority,
	workflow processing.WorkflowConfiguration, raw string) (processing.SourceProjection, processing.EvaluationScope) {
	t.Helper()
	result, err := owner.CaptureSyntheticPreview(context.Background(), tx, authority, workflow,
		json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return result.Projection, result.Scope
}

func findField(fields []processing.CapturedField, path ...string) (processing.CapturedField, bool) {
	want := strings.Join(path, "\x00")
	for _, field := range fields {
		if field.Reference.Namespace == "source" && strings.Join(field.Reference.Path, "\x00") == want {
			return field, true
		}
	}
	return processing.CapturedField{}, false
}

func TestSyntheticOwnerCapturesEveryFrozenForm(t *testing.T) {
	unit := uuid.New()
	allowed := testAuthority("payload", "gateway", "address", "unit")
	cases := []struct {
		name, raw, path string
		kind            processing.ValueKind
		wantNull        bool
	}{
		{"pocsag text", `{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"hello"},"targets":[{"address":123,"function":1}]}`, "payload.text.value", processing.StringValue, false},
		{"pocsag numeric", `{"schemaVersion":1,"protocol":"pocsag","form":"numeric","networkId":"NETWORK","payload":{"value":"12U -"},"targets":[{"address":123,"function":1}]}`, "payload.numeric.value", processing.StringValue, false},
		{"pocsag tone-only", `{"schemaVersion":1,"protocol":"pocsag","form":"tone-only","networkId":"NETWORK","targets":[]}`, "", "", false},
		{"flex text", `{"schemaVersion":1,"protocol":"flex","form":"text","networkId":"NETWORK","payload":{"value":"hello"},"targets":[{"address":1}]}`, "payload.text.value", processing.StringValue, false},
		{"flex numeric", `{"schemaVersion":1,"protocol":"flex","form":"numeric","networkId":"NETWORK","payload":{"value":"123"},"targets":[{"address":1}]}`, "payload.numeric.value", processing.StringValue, false},
		{"flex tone-only", `{"schemaVersion":1,"protocol":"flex","form":"tone-only","networkId":"NETWORK","targets":[]}`, "", "", false},
		{"flex binary", `{"schemaVersion":1,"protocol":"flex","form":"binary","subtype":"secure","networkId":"NETWORK","payload":{"encoding":"base64","data":"AAEC/w=="},"targets":[{"address":1}]}`, "payload.binary", processing.BytesValue, false},
		{"flex short signal", `{"schemaVersion":1,"protocol":"flex","form":"flex-short-signal","networkId":"NETWORK","payload":{"sourceCode":7},"targets":[{"address":1}]}`, "payload.flex.shortSignal.sourceCode", processing.IntegerValue, false},
		{"zvei explicit no-tone", `{"schemaVersion":1,"protocol":"zvei","form":"zvei-signal","networkId":"NETWORK","payload":{"double_tone":null},"targets":[{"address":"12345"}]}`, "payload.zvei.double_tone", processing.NullValue, true},
		{"zvei named tone", `{"schemaVersion":1,"protocol":"zvei","form":"zvei-signal","networkId":"NETWORK","payload":{"double_tone":"ABC"},"targets":[{"address":"12345"}]}`, "payload.zvei.double_tone", processing.StringValue, false},
		{"eas header", `{"schemaVersion":1,"protocol":"eas","form":"eas-header","networkId":"NETWORK","payload":{"originator":"WXR","eventCode":"TOR","purgeTime":"0430","issueTime":"3081205"},"targets":[{"address":"010001"}]}`, "payload.eas.eventCode", processing.StringValue, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			protocol := protocols.Protocol("pocsag")
			switch test.name[:4] {
			case "flex":
				protocol = protocols.FLEX
			case "zvei":
				protocol = protocols.ZVEI
			case "eas ":
				protocol = protocols.EAS
			}
			owner, network, tx := testOwner(protocol, &unit)
			raw := replaceNetworkID(test.raw, network.ID)
			projection, _ := capture(t, owner, network, tx, allowed,
				testWorkflow(authz.Scope{Kind: "all"}), raw)
			if !projection.Synthetic || projection.SourceRevision != 1 || projection.ContextRevision != 7 {
				t.Fatalf("synthetic identity was not server-owned: %+v", projection)
			}
			if test.path == "" {
				return
			}
			path := strings.Split(test.path, ".")
			field, ok := findField(projection.Fields, path...)
			if !ok || field.Field.State != processing.FieldPresent || field.Field.Value == nil ||
				field.Field.Value.Kind != test.kind {
				t.Fatalf("missing typed field %s: %+v", test.path, field)
			}
			if test.wantNull && (!field.Schema.Nullable || field.Schema.Optional == false) {
				t.Fatalf("explicit null declaration lost: %+v", field.Schema)
			}
		})
	}
}

func TestSyntheticOwnerUsesMatcherAndWorkflowUnitScope(t *testing.T) {
	unit := uuid.New()
	owner, network, tx := testOwner(protocols.POCSAG, &unit)
	authority := testAuthority("payload", "gateway", "address", "unit")
	workflow := testWorkflow(authz.Scope{Kind: "unit", References: []string{unit.String()}})
	raw := replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"x"},"targets":[{"address":7,"function":0},{"address":8,"function":1}]}`, network.ID)
	projection, scope := capture(t, owner, network, tx, authority, workflow, raw)
	if !scope.SourceEligible || len(scope.TargetPositions) != 2 || scope.TargetPositions[0] != 0 ||
		projection.Targets[1].Position != 1 {
		t.Fatalf("Unit scope did not retain original target positions: %+v", scope)
	}

	unassigned, network, tx := testOwner(protocols.POCSAG, nil)
	raw = replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"x"},"targets":[{"address":7,"function":0}]}`, network.ID)
	projection, scope = capture(t, unassigned, network, tx, authority, workflow, raw)
	if scope.SourceEligible || len(scope.TargetPositions) != 0 ||
		projection.Targets[0].Fields[3].Field.State != processing.FieldAbsent {
		t.Fatalf("unassigned sample fabricated Unit scope: %+v %+v", scope, projection.Targets[0])
	}
}

func TestSyntheticOwnerCapturesEveryFieldRegardlessOfEditorRightsAndFailsClosedWithoutNetwork(t *testing.T) {
	owner, network, tx := testOwner(protocols.POCSAG, nil)
	authority := testAuthority("payload")
	raw := replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"x"},"targets":[]}`, network.ID)
	projection, _ := capture(t, owner, network, tx, authority, testWorkflow(authz.Scope{Kind: "all"}), raw)
	name, ok := findField(projection.Fields, "network", "name")
	text, textOK := findField(projection.Fields, "payload", "text", "value")
	if !ok || name.Field.State != processing.FieldPresent || !textOK || text.Field.State != processing.FieldPresent {
		t.Fatalf("capture depended on the editor's field rights: %+v %+v", name, text)
	}
	missing, err := NewOwner(testMatcher{}, nil, testClock{now: time.Now()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = missing.CaptureSyntheticPreview(context.Background(), tx, authority,
		testWorkflow(authz.Scope{Kind: "all"}), json.RawMessage(raw))
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("missing Network owner was not explicit: %v", err)
	}
	_, err = owner.CaptureStoredPreview(context.Background(), tx, authority,
		testWorkflow(authz.Scope{Kind: "all"}), uuid.New())
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("stored source was substituted: %v", err)
	}
}

func TestSyntheticOwnerRejectsUnfrozenInputAndOutOfScopeNetwork(t *testing.T) {
	owner, network, tx := testOwner(protocols.POCSAG, nil)
	authority := testAuthority("payload", "gateway", "address", "unit")
	workflow := testWorkflow(authz.Scope{Kind: "network", References: []string{uuid.NewString()}})
	cases := []string{
		`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"x","schema":{} }}`,
		`{"schemaVersion":1,"protocol":"pocsag","form":"numeric","networkId":"NETWORK","payload":{"value":"12A"},"targets":[]}`,
		`{"schemaVersion":1,"protocol":"flex","form":"binary","subtype":"transparent","networkId":"NETWORK","payload":{"encoding":"base64","data":"__8="},"targets":[]}`,
		`{"schemaVersion":1,"protocol":"zvei","form":"zvei-signal","networkId":"NETWORK","payload":{},"targets":[{"address":"12345"}]}`,
		`{"schemaVersion":1,"protocol":"eas","form":"eas-header","networkId":"NETWORK","payload":{"originator":"BAD","eventCode":"TOR","purgeTime":"0010","issueTime":"0001200"},"targets":[{"address":"010001"}]}`,
		`{"schemaVersion":1,"protocol":"pocsag","form":"unknown","networkId":"NETWORK","payload":{"value":"x"},"targets":[]}`,
	}
	for _, raw := range cases {
		raw = replaceNetworkID(raw, network.ID)
		if _, err := owner.CaptureSyntheticPreview(context.Background(), tx, authority, testWorkflow(authz.Scope{Kind: "all"}),
			json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid synthetic request accepted: %s", raw)
		}
	}
	raw := replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"x"},"targets":[]}`, network.ID)
	if _, err := owner.CaptureSyntheticPreview(context.Background(), tx, authority, workflow, json.RawMessage(raw)); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Network outside configured scope accepted: %v", err)
	}
}

func replaceNetworkID(raw string, id uuid.UUID) string {
	return strings.ReplaceAll(raw, "NETWORK", id.String())
}

type fixedMatcher struct{ result assignments.MatchResult }

// MatchTargets returns the current mutable fixture snapshot without synthesizing names or identities.
func (m *fixedMatcher) MatchTargets(context.Context, platform.TransactionContext, uuid.UUID,
	protocols.Protocol, time.Time, []protocols.Target) (assignments.MatchResult, error) {
	return m.result, nil
}

func TestSyntheticUnionDeclarationsRemainAvailableAcrossForms(t *testing.T) {
	owner, network, tx := testOwner(protocols.POCSAG, nil)
	projection, _ := capture(t, owner, network, tx, testAuthority("payload", "gateway"),
		testWorkflow(authz.Scope{Kind: "all"}),
		replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"tone-only","networkId":"NETWORK","targets":[]}`, network.ID))
	source, err := processing.CaptureSource(t.Context(), projection)
	if err != nil {
		t.Fatal(err)
	}
	declarations, err := processing.NewSyntheticDeclarationOwner().ProcessingDeclarations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range processing.SyntheticSourceDeclarationRefs() {
		expected, err := declarations.FieldSchema(ref)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := source.FieldSchema(ref)
		if err != nil {
			t.Fatalf("form switching lost %v: %v", ref.Path, err)
		}
		left, _ := json.Marshal(actual)
		right, _ := json.Marshal(expected)
		if string(left) != string(right) {
			t.Fatalf("declaration changed for %v", ref.Path)
		}
	}
	ref := processing.FieldRef{Namespace: "source", Path: []string{"payload", "binary"}}
	yes, _ := processing.Literal(processing.BooleanValue, true)
	definition := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "present", Field: &ref, Value: &yes},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
	program, err := processing.CompileWorkflow(t.Context(), definition, declarations, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(t.Context(), source)
	if err != nil || result.Selected {
		t.Fatalf("unrelated form did not produce readable absence: %+v %v", result, err)
	}
	dependencies, err := result.Dependencies()
	if err != nil || len(dependencies) != 1 || !strings.Contains(strings.Join(dependencies[0].Groups, ","), "payload") {
		t.Fatalf("false selector lost final recheck evidence: %+v %v", dependencies, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || string(encoded) != `{"selected":false}` {
		t.Fatalf("host-only selector evidence leaked into returned context: %s %v", encoded, err)
	}
}

func TestSyntheticRecheckRejectsNetworkUnitAndFieldChanges(t *testing.T) {
	owner, network, tx := testOwner(protocols.POCSAG, nil)
	unit := uuid.New()
	matcher := &fixedMatcher{result: assignments.MatchResult{Revision: 7,
		Resolutions: []assignments.Resolution{{TargetPosition: 0, Status: "assigned", UnitID: &unit}},
		UnitContext: map[uuid.UUID]units.Context{unit: {ID: unit, Revision: 1}}}}
	owner.matcher = matcher
	authority := testAuthority("payload", "gateway", "address", "unit")
	workflow := testWorkflow(authz.Scope{Kind: "all"})
	projection, _ := capture(t, owner, network, tx, authority, workflow,
		replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"text","observedAt":"2020-01-01T10:00:00.123456789+01:00","networkId":"NETWORK","payload":{"value":"x"},"targets":[{"address":7,"function":0}]}`, network.ID))
	source, err := processing.CaptureSource(t.Context(), projection)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := source.Identity()
	if err != nil {
		t.Fatal(err)
	}
	binding := processing.ExecutionBinding{WorkflowID: workflow.ID, Source: identity}
	field, _ := findField(projection.Fields, "payload", "text", "value")
	check := func(authority authz.Authority) error {
		return owner.CheckInstalledSource(t.Context(), tx, binding, authority,
			processing.InvocationContext{Source: identity}, field.Field.Dependencies, true)
	}
	if err := check(authority); err != nil {
		t.Fatalf("unchanged capture rejected: %v", err)
	}
	_, observationOffset := projection.ObservedAt.Zone()
	if projection.ObservedAt.Nanosecond() != 0 || projection.ObservedAt.Second() != 0 || observationOffset != 3600 ||
		!owner.scope.(*testScope).now.Equal(owner.clock.(testClock).now) {
		t.Fatal("observation truncation/offset changed or observation time substituted for current authorization time")
	}
	if err := check(testAuthority("gateway")); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("revoked field authority accepted: %v", err)
	}
	changed := matcher.result.UnitContext[unit]
	changed.Revision++
	matcher.result.UnitContext[unit] = changed
	if err := check(authority); platform.FailureCode(err) != platform.CodeRevisionConflict {
		t.Fatalf("changed Unit accepted with unchanged matcher revision: %v", err)
	}
	changed.Revision--
	matcher.result.UnitContext[unit] = changed
	renamed := network
	renamed.Name = "changed"
	owner.networks = testNetworks{network: renamed}
	if err := check(authority); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("changed Network accepted: %v", err)
	}
	owner.networks = testNetworks{network: network}
	owner.ReleaseSyntheticPreview(identity.TransmissionID)
	if err := check(authority); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("released preview source accepted: %v", err)
	}
}

func TestSyntheticUnitScopeDoesNotReturnSiblingTargetContext(t *testing.T) {
	owner, network, tx := testOwner(protocols.POCSAG, nil)
	unit, sibling := uuid.New(), uuid.New()
	owner.matcher = &fixedMatcher{result: assignments.MatchResult{Revision: 7,
		Resolutions: []assignments.Resolution{
			{TargetPosition: 0, Status: "assigned", UnitID: &unit},
			{TargetPosition: 1, Status: "assigned", UnitID: &sibling}},
		UnitContext: map[uuid.UUID]units.Context{unit: {ID: unit, Revision: 1}, sibling: {ID: sibling, Revision: 1}}}}
	projection, scope := capture(t, owner, network, tx, testAuthority("payload", "gateway", "unit", "address"),
		testWorkflow(authz.Scope{Kind: "unit", References: []string{unit.String()}}),
		replaceNetworkID(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK","payload":{"value":"x"},"targets":[{"address":7,"function":0},{"address":8,"function":0}]}`, network.ID))
	source, err := processing.CaptureSource(t.Context(), projection)
	if err != nil {
		t.Fatal(err)
	}
	declarations, err := processing.NewSyntheticDeclarationOwner().ProcessingDeclarations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ref := processing.FieldRef{Namespace: "target", Path: []string{"unit", "id"}}
	yes, _ := processing.Literal(processing.BooleanValue, true)
	definition := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "present", Field: &ref, Value: &yes},
		OutputSchema: map[string]processing.FieldSchema{"unit": {Kind: processing.StringValue, Optional: true}},
		Outputs:      map[string]processing.Assignment{"unit": {Field: &ref}}}
	program, err := processing.CompileWorkflow(t.Context(), definition, declarations, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.EvaluateScoped(t.Context(), source, scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(result.TargetResults) != 1 || result.TargetResults[0].TargetPosition != 0 ||
		strings.Contains(string(encoded), sibling.String()) {
		t.Fatalf("sibling target context escaped scope: %s %v", encoded, err)
	}
}
