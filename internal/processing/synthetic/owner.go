// Package synthetic supplies operator-authored synthetic Transmissions as the trusted
// source for Workflow previews. Owner (built with NewOwner) validates a synthetic
// Input against its Protocol form, resolves its targets against the current Network
// and Assignment context, masks fields the caller may not read, and keeps the captured
// context in memory so installed plugin calls made during the preview can be rechecked
// via CheckInstalledSource. Owner implements management.PreviewSourceOwner and
// management.SyntheticPreviewReleaser; nothing it captures is persisted.
package synthetic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// Network is the current Network context a synthetic preview is captured against.
type Network struct {
	ID       uuid.UUID
	Name     string
	Protocol protocols.Protocol
	AgencyID uuid.UUID
	Deleted  bool
}

// NetworkOwner reads the current Network for a synthetic preview inside the caller's
// transaction. Networks reported as Deleted, or whose Protocol or name do not match the
// synthetic input, are rejected as unavailable.
type NetworkOwner interface {
	ReadSyntheticNetwork(context.Context, platform.TransactionContext, uuid.UUID) (Network, error)
}

// Matcher resolves synthetic targets against the current Assignment context of a
// Network at the observation time. It must return one Resolution per target and a
// positive context revision.
type Matcher interface {
	MatchTargets(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol,
		time.Time, []protocols.Target) (assignments.MatchResult, error)
}

// ScopeAuthorizer checks that an authority may preview a Workflow against the given
// Network and Unit (uuid.Nil for non-Unit scopes) at the given time. It returns a
// non-nil error to deny.
type ScopeAuthorizer interface {
	CheckSyntheticPreviewScope(context.Context, platform.TransactionContext, authz.Authority,
		uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
}

// UnitAttributeOwner reads the Unit attributes a preview captures for its targets: the declared
// target fields the previewed definition reads and the effective attributes of the given Units,
// inside the caller's transaction.
type UnitAttributeOwner interface {
	ReadUnitAttributes(context.Context, platform.TransactionContext, processing.Definition, []uuid.UUID,
	) (declarations.UnitAttributes, error)
}

// Option configures an Owner.
type Option func(*Owner)

// WithUnitAttributes makes previews capture Unit attributes like a real run does. Without it a
// preview of a Workflow reading a Unit attribute fails, because the field is not captured.
func WithUnitAttributes(attributes UnitAttributeOwner) Option {
	return func(o *Owner) { o.unitAttributes = attributes }
}

// Input is the JSON document describing one synthetic Transmission. SchemaVersion must
// be 1; Form (and Subtype for FLEX binary) must be declared for Protocol; Payload is
// required for every form except tone-only, where it must be absent; ObservedAt is an
// optional RFC 3339 instant and defaults to the transaction clock. Targets are limited
// to processing.MaxArguments entries (31 for EAS) and ZVEI/EAS require at least one.
type Input struct {
	SchemaVersion int                `json:"schemaVersion"`
	Protocol      protocols.Protocol `json:"protocol"`
	Form          string             `json:"form"`
	Subtype       string             `json:"subtype,omitempty"`
	ObservedAt    *string            `json:"observedAt,omitempty"`
	NetworkID     uuid.UUID          `json:"networkId"`
	Payload       json.RawMessage    `json:"payload,omitempty"`
	Targets       []protocols.Target `json:"targets"`
}

// Owner captures synthetic preview sources and rechecks them for installed plugin calls.
// It is safe for concurrent use. Captured contexts are held in memory until released
// with ReleaseSyntheticPreview.
type Owner struct {
	matcher  Matcher
	networks NetworkOwner
	clock    platform.Clock
	scope    ScopeAuthorizer
	// unitAttributes is nil when previews capture no Unit attributes.
	unitAttributes UnitAttributeOwner
	// stored and access are nil when previews of stored Transmissions are unavailable.
	stored StoredTransmissionReader
	access TransmissionAccess
	mu     sync.Mutex
	active map[uuid.UUID]captured
}

type captured struct {
	workflow      processing.WorkflowConfiguration
	network       Network
	observed      time.Time
	targets       []protocols.Target
	resolutions   []assignments.Resolution
	units         map[uuid.UUID]units.Context
	identity      processing.SourceIdentity
	eligibleUnits []uuid.UUID
	// stored is set for a preview of a stored Transmission.
	stored *storedOrigin
}

// NewOwner returns an Owner. matcher and clock are required and an invalid-input
// failure is returned without them. networks and scope may be nil; operations that need
// them then fail as unavailable.
func NewOwner(matcher Matcher, networks NetworkOwner, clock platform.Clock, scope ScopeAuthorizer,
	options ...Option,
) (*Owner, error) {
	if matcher == nil || clock == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing synthetic source dependency"))
	}
	owner := &Owner{matcher: matcher, networks: networks, clock: clock, scope: scope,
		active: make(map[uuid.UUID]captured)}
	for _, option := range options {
		option(owner)
	}
	return owner, nil
}

// CaptureSyntheticPreview decodes and validates raw as an Input, resolves it against the
// current Network and Assignment context in tx, and evaluates the workflow's scope (a
// "network" scope that excludes the Network yields authz.ErrForbidden). The projection holds
// every field with its owner classification: evaluation runs as the system, and the editor's
// field permissions are applied to the definition at save time and to the result when it is
// rechecked, never by hiding inputs. The captured context is retained under the projection's
// TransmissionID until ReleaseSyntheticPreview is called; the caller must release it. Only the Unit
// attributes that definition reads are captured.
func (o *Owner) CaptureSyntheticPreview(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, workflow processing.WorkflowConfiguration, definition processing.Definition,
	raw json.RawMessage) (management.PreviewCapture, error) {
	if o == nil || ctx == nil || tx == nil || o.matcher == nil || o.clock == nil {
		return management.PreviewCapture{}, invalid(errors.New("missing synthetic capture participant"))
	}
	if o.networks == nil {
		return management.PreviewCapture{}, unavailable(errors.New("Network owner unavailable"))
	}
	if err := ctx.Err(); err != nil {
		return management.PreviewCapture{}, err
	}
	var input Input
	if err := processing.Decode(raw, &input); err != nil {
		return management.PreviewCapture{}, err
	}
	if err := input.validate(); err != nil {
		return management.PreviewCapture{}, err
	}
	now, err := o.clock.Now(ctx, tx)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	observed, err := observation(input.ObservedAt, now)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	return o.captureInput(ctx, tx, workflow, definition, input, observed, nil)
}

// captureInput resolves a validated input against the current Network and Assignment context,
// projects it and retains its captured context. Synthetic and stored previews share it, so a
// stored sample is evaluated exactly like a synthetic one. A stored sample passes the Units its
// targets are persisted as assigned to: when the fresh matching differs, an address rule changed
// since intake and no correction has caught up, so the editor's check against the persisted
// context would not hold for what is evaluated; the preview is refused as a revision conflict.
func (o *Owner) captureInput(ctx context.Context, tx platform.TransactionContext,
	workflow processing.WorkflowConfiguration, definition processing.Definition, input Input, observed time.Time,
	persisted *[]*uuid.UUID,
) (management.PreviewCapture, error) {
	network, err := o.networks.ReadSyntheticNetwork(ctx, tx, input.NetworkID)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	if network.ID != input.NetworkID || network.Deleted || !network.Protocol.Valid() ||
		network.Protocol != input.Protocol || !utf8.ValidString(network.Name) || strings.TrimSpace(network.Name) == "" {
		return management.PreviewCapture{}, unavailable(errors.New("current Network context does not match synthetic input"))
	}
	matched, err := o.matcher.MatchTargets(ctx, tx, network.ID, input.Protocol, observed, input.Targets)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	if matched.Revision <= 0 || len(matched.Resolutions) != len(input.Targets) {
		return management.PreviewCapture{}, unavailable(errors.New("current target matcher returned incomplete context"))
	}
	if persisted != nil && !sameAssignedUnits(matched.Resolutions, *persisted) {
		return management.PreviewCapture{}, authz.ErrRevisionConflict
	}
	var unitAttributes declarations.UnitAttributes
	if o.unitAttributes != nil {
		unitAttributes, err = o.unitAttributes.ReadUnitAttributes(ctx, tx, definition, assignedUnits(matched.Resolutions))
		if err != nil {
			return management.PreviewCapture{}, err
		}
	}
	projection, err := project(input, network, observed, matched, unitAttributes)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	scope, eligibleUnits, err := evaluateScope(workflow, network, input.Targets, matched)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	source, err := processing.CaptureSource(ctx, projection)
	if err != nil {
		return management.PreviewCapture{}, err
	}
	identity, err := source.Identity()
	if err != nil {
		return management.PreviewCapture{}, err
	}
	o.mu.Lock()
	o.active[identity.TransmissionID] = captured{workflow: workflow, network: network, observed: observed,
		targets: slices.Clone(input.Targets), resolutions: slices.Clone(matched.Resolutions),
		units: cloneUnits(matched.UnitContext), identity: identity, eligibleUnits: eligibleUnits}
	o.mu.Unlock()
	return management.PreviewCapture{Projection: projection, Scope: scope}, nil
}

// ReleaseSyntheticPreview forgets the captured context for a synthetic TransmissionID.
// It is a no-op for a nil Owner, uuid.Nil or an unknown id.
func (o *Owner) ReleaseSyntheticPreview(id uuid.UUID) {
	if o == nil || id == uuid.Nil {
		return
	}
	o.mu.Lock()
	delete(o.active, id)
	o.mu.Unlock()
}

// CheckInstalledSource verifies, for an installed plugin call during a preview, that the
// bound synthetic source is still captured and that its Network and Assignment context
// are unchanged (a changed context yields a revision-conflict failure). When editor is
// true it additionally rechecks the Workflow scope for every eligible Unit through the
// ScopeAuthorizer and requires every dependency to belong to the same source and be
// readable by the authority.
func (o *Owner) CheckInstalledSource(ctx context.Context, tx platform.TransactionContext,
	binding processing.ExecutionBinding, authority authz.Authority, call processing.InvocationContext,
	dependencies []processing.Dependency, editor bool) error {
	if o == nil || ctx == nil || tx == nil || o.networks == nil {
		return unavailable(errors.New("synthetic execution source owner unavailable"))
	}
	if !binding.Source.Synthetic || call.Source != binding.Source {
		return unavailable(errors.New("synthetic source identity is not current"))
	}
	o.mu.Lock()
	record, ok := o.active[binding.Source.TransmissionID]
	o.mu.Unlock()
	if !ok || record.identity != binding.Source {
		return unavailable(errors.New("synthetic preview source expired"))
	}
	network, err := o.networks.ReadSyntheticNetwork(ctx, tx, record.network.ID)
	if err != nil {
		return err
	}
	if network.ID != record.network.ID || network.Deleted || network.Name != record.network.Name ||
		network.Protocol != record.network.Protocol {
		return unavailable(errors.New("synthetic Network context changed"))
	}
	matched, err := o.matcher.MatchTargets(ctx, tx, network.ID, network.Protocol, record.observed, record.targets)
	if err != nil {
		return err
	}
	if matched.Revision != record.identity.ContextRevision || !sameResolutions(record.resolutions, matched.Resolutions) ||
		!sameUnits(record.units, matched.UnitContext) {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("synthetic Assignment context changed"))
	}
	if !editor {
		return nil
	}
	if o.scope == nil {
		return unavailable(errors.New("current Workflow scope owner unavailable"))
	}
	now, err := o.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	unitIDs := record.eligibleUnits
	if record.workflow.Scope.Narrowing == nil || record.workflow.Scope.Narrowing.Kind != "unit" {
		unitIDs = []uuid.UUID{uuid.Nil}
	}
	for _, unitID := range unitIDs {
		if err := o.scope.CheckSyntheticPreviewScope(ctx, tx, authority, binding.WorkflowID,
			network.ID, unitID, now); err != nil {
			return err
		}
	}
	if record.stored != nil {
		if err := o.checkStoredVisible(ctx, tx, authority, now, record.stored.message, record.stored.transmission,
			record.stored.targets); err != nil {
			return err
		}
	}
	for _, dependency := range dependencies {
		if dependency.TransmissionID != binding.Source.TransmissionID ||
			dependency.NetworkID != binding.Source.NetworkID ||
			dependency.SourceRevision != binding.Source.SourceRevision ||
			dependency.ContextRevision != binding.Source.ContextRevision {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("synthetic dependency differs from source"))
		}
		if err := authz.AuthorizeFields(authority, dependency.Groups); err != nil {
			return err
		}
	}
	return nil
}

func (in Input) validate() error {
	if in.SchemaVersion != 1 || in.NetworkID == uuid.Nil || !in.Protocol.Valid() ||
		len(in.Targets) > processing.MaxArguments {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid synthetic source identity"))
	}
	validForm := false
	switch in.Protocol {
	case protocols.POCSAG:
		validForm = in.Form == "text" || in.Form == "numeric" || in.Form == "tone-only"
	case protocols.FLEX:
		validForm = in.Form == "text" || in.Form == "numeric" || in.Form == "tone-only" ||
			in.Form == "binary" || in.Form == "flex-short-signal"
	case protocols.ZVEI:
		validForm = in.Form == "zvei-signal"
	case protocols.EAS:
		validForm = in.Form == "eas-header"
	}
	if !validForm {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("unsupported Protocol form"))
	}
	if in.Protocol == protocols.ZVEI || in.Protocol == protocols.EAS {
		if len(in.Targets) == 0 {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("protocol form requires a target"))
		}
	}
	if in.Protocol == protocols.EAS && len(in.Targets) > 31 {
		return platform.NewFailure(platform.CodeResourceLimit, errors.New("EAS area count exceeds 31"))
	}
	if in.Protocol == protocols.FLEX && in.Form == "binary" {
		if in.Subtype != "transparent" && in.Subtype != "secure" {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid FLEX binary subtype"))
		}
	} else if in.Subtype != "" {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("subtype is not declared for this form"))
	}
	if in.Form == "tone-only" {
		if len(in.Payload) != 0 {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("tone-only form does not accept a Payload"))
		}
	} else if len(in.Payload) == 0 {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("synthetic Payload is required"))
	}
	for _, target := range in.Targets {
		if err := protocols.ValidateTarget(in.Protocol, target); err != nil {
			return err
		}
	}
	return validatePayload(in.Form, in.Payload)
}

func validatePayload(form string, raw json.RawMessage) error {
	switch form {
	case "tone-only":
		return nil
	case "text":
		var payload struct {
			Value string `json:"value"`
		}
		if err := processing.Decode(raw, &payload); err != nil {
			return err
		}
		return nil
	case "numeric":
		var payload struct {
			Value string `json:"value"`
		}
		if err := processing.Decode(raw, &payload); err != nil {
			return err
		}
		for _, glyph := range payload.Value {
			if !strings.ContainsRune("0123456789.U -)(", glyph) {
				return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid numeric Payload glyph"))
			}
		}
		return nil
	case "binary":
		var payload struct {
			Encoding string `json:"encoding"`
			Data     string `json:"data"`
		}
		if err := processing.Decode(raw, &payload); err != nil {
			return err
		}
		if payload.Encoding != "base64" {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("binary Payload requires Base64 encoding"))
		}
		decoded, err := base64.StdEncoding.DecodeString(payload.Data)
		if err != nil || base64.StdEncoding.EncodeToString(decoded) != payload.Data {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("binary Payload is not canonical Base64"))
		}
		return nil
	case "flex-short-signal":
		var payload struct {
			SourceCode *int `json:"sourceCode"`
		}
		if err := processing.Decode(raw, &payload); err != nil {
			return err
		}
		if payload.SourceCode == nil || *payload.SourceCode < 0 || *payload.SourceCode > 7 {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid FLEX short-signal Source code"))
		}
		return nil
	case "zvei-signal":
		var payload struct {
			DoubleTone json.RawMessage `json:"double_tone"`
		}
		if err := processing.Decode(raw, &payload); err != nil {
			return err
		}
		if len(payload.DoubleTone) == 0 {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("ZVEI double_tone is required"))
		}
		if bytes.Equal(bytes.TrimSpace(payload.DoubleTone), []byte("null")) {
			return nil
		}
		var name string
		if err := processing.Decode(payload.DoubleTone, &name); err != nil {
			return err
		}
		if name == "" || !utf8.ValidString(name) {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("ZVEI double-tone name must be nonempty"))
		}
		return nil
	case "eas-header":
		return validateEASPayload(raw)
	default:
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("unsupported synthetic Payload form"))
	}
}

func validateEASPayload(raw json.RawMessage) error {
	var payload struct {
		Originator string `json:"originator"`
		EventCode  string `json:"eventCode"`
		PurgeTime  string `json:"purgeTime"`
		IssueTime  string `json:"issueTime"`
	}
	if err := processing.Decode(raw, &payload); err != nil {
		return err
	}
	if !slices.Contains(easOriginators, payload.Originator) ||
		!slices.Contains(easEventCodes, payload.EventCode) || !validPurgeTime(payload.PurgeTime) ||
		!validIssueTime(payload.IssueTime) {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid EAS header Payload"))
	}
	return nil
}

var easOriginators = []string{"EAS", "CIV", "WXR", "PEP"}
var easEventCodes = strings.Fields("EAN NIC NPT RMT RWT ADR AVW AVA BZW BLU CAE CDW CEM CFW CFA DSW EQW EVI EWW FRW FFW FFA FFS FLW FLA FLS HMW HWW HWA HUW HUA HLS LEW LAE MEP NMN TOE NUW DMO RHW SVR SVA SVS SPW SMW SPS SSA SSW TOR TOA TRW TRA TSW TSA VOW WSW WSA")

func validPurgeTime(value string) bool {
	if len(value) != 4 {
		return false
	}
	minutes, err := strconv.Atoi(value[2:])
	hours, hourErr := strconv.Atoi(value[:2])
	if err != nil || hourErr != nil || hours > 99 {
		return false
	}
	if hours == 0 {
		return minutes == 15 || minutes == 30 || minutes == 45
	}
	if minutes != 0 && minutes != 30 {
		return false
	}
	return hours >= 1
}

func validIssueTime(value string) bool {
	if len(value) != 7 {
		return false
	}
	day, dayErr := strconv.Atoi(value[:3])
	hour, hourErr := strconv.Atoi(value[3:5])
	minute, minuteErr := strconv.Atoi(value[5:])
	return dayErr == nil && hourErr == nil && minuteErr == nil &&
		day >= 1 && day <= 366 && hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59
}

func observation(value *string, now time.Time) (time.Time, error) {
	var instant time.Time
	if value == nil {
		instant = now
	} else {
		parsed, err := time.Parse(time.RFC3339Nano, *value)
		if err != nil {
			return time.Time{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("observation time must be RFC3339 with offset"))
		}
		instant = parsed
	}
	if instant.IsZero() || instant.Year() < 0 || instant.Year() > 9999 {
		return time.Time{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid synthetic observation time"))
	}
	return instant.Truncate(time.Second), nil
}

func project(input Input, network Network, observed time.Time, match assignments.MatchResult,
	unitAttributes declarations.UnitAttributes,
) (processing.SourceProjection, error) {
	transmission := uuid.New()
	projection := processing.SourceProjection{TransmissionID: transmission, Synthetic: true, Protocol: input.Protocol,
		Form: input.Form, Subtype: input.Subtype, ObservedAt: observed, SourceRevision: 1,
		ContextRevision: match.Revision, NetworkID: network.ID}
	dependency := func(group string, position *uint32) []processing.Dependency {
		var target *uint32
		if position != nil {
			value := *position
			target = &value
		}
		return []processing.Dependency{{TransmissionID: transmission, NetworkID: network.ID,
			SourceRevision: 1, ContextRevision: match.Revision, TargetPosition: target, Groups: []string{group}}}
	}
	if err := addPayloadFields(&projection, input, dependency); err != nil {
		return processing.SourceProjection{}, err
	}
	contextual, err := projection.WithContextFields(network.Name)
	if err != nil {
		return processing.SourceProjection{}, err
	}
	projection = contextual
	timeValue, err := processing.Literal(processing.TimestampValue, observed.Format(time.RFC3339Nano))
	if err != nil {
		return processing.SourceProjection{}, err
	}
	projection.Fields = append(projection.Fields, processing.CapturedField{
		Reference: processing.FieldRef{Namespace: "source", Path: []string{"observedAt"}},
		Schema:    processing.FieldSchema{Kind: processing.TimestampValue},
		Field: processing.FieldValue{State: processing.FieldPresent, Value: &timeValue,
			Dependencies: dependency("gateway", nil)}})
	projection.TargetDeclarations = []processing.FieldDeclaration{
		{Reference: processing.FieldRef{Namespace: "target", Path: []string{"address", "number"}},
			Schema: processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, Groups: []string{"address"}},
		{Reference: processing.FieldRef{Namespace: "target", Path: []string{"address", "text"}},
			Schema: processing.FieldSchema{Kind: processing.StringValue, Optional: true}, Groups: []string{"address"}},
		{Reference: processing.FieldRef{Namespace: "target", Path: []string{"function"}},
			Schema: processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, Groups: []string{"address"}},
		{Reference: processing.FieldRef{Namespace: "target", Path: []string{"unit", "id"}},
			Schema: processing.FieldSchema{Kind: processing.StringValue, Optional: true}, Groups: []string{"unit"}},
		{Reference: processing.FieldRef{Namespace: "target", Path: []string{"unit", "collections"}},
			Schema: processing.FieldSchema{Kind: processing.ListValue, Optional: true,
				Element: &processing.FieldSchema{Kind: processing.StringValue}}, Groups: []string{"unit"}},
	}
	projection.TargetDeclarations = append(projection.TargetDeclarations, unitAttributes.Declared...)
	for position, target := range input.Targets {
		index := uint32(position)
		fields := make([]processing.CapturedField, 0, len(projection.TargetDeclarations))
		appendField := func(path []string, schema processing.FieldSchema, value *processing.Value, group string) {
			state := processing.FieldAbsent
			if value != nil {
				state = processing.FieldPresent
			}
			fields = append(fields, processing.CapturedField{Reference: processing.FieldRef{Namespace: "target", Path: path},
				Schema: schema, Field: processing.FieldValue{State: state, Value: value, Dependencies: dependency(group, &index)}})
		}
		if number, ok := target.Address.Number(); ok {
			if number > math.MaxInt64 {
				return processing.SourceProjection{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("target integer exceeds shared typed integer"))
			}
			value, _ := processing.Literal(processing.IntegerValue, strconv.FormatUint(number, 10))
			appendField([]string{"address", "number"}, processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, &value, "address")
			appendField([]string{"address", "text"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, nil, "address")
		} else {
			text, _ := target.Address.Text()
			value, _ := processing.Literal(processing.StringValue, text)
			appendField([]string{"address", "number"}, processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, nil, "address")
			appendField([]string{"address", "text"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, &value, "address")
		}
		var functionValue *processing.Value
		if target.Function != nil {
			value, _ := processing.Literal(processing.IntegerValue, strconv.FormatUint(uint64(*target.Function), 10))
			functionValue = &value
		}
		appendField([]string{"function"}, processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, functionValue, "address")
		resolution := match.Resolutions[position]
		if resolution.TargetPosition != position {
			return processing.SourceProjection{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("matcher changed original target positions"))
		}
		if resolution.UnitID != nil {
			unit, ok := match.UnitContext[*resolution.UnitID]
			if !ok || unit.ID != *resolution.UnitID || unit.Deleted || unit.Revision <= 0 {
				return processing.SourceProjection{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("matcher returned missing current Unit context"))
			}
			value, _ := processing.Literal(processing.StringValue, unit.ID.String())
			appendField([]string{"unit", "id"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, &value, "unit")
			collections := make([]processing.Value, len(unit.CollectionIDs))
			for i, id := range unit.CollectionIDs {
				collections[i], _ = processing.Literal(processing.StringValue, id.String())
			}
			list, err := processing.Literal(processing.ListValue, collections)
			if err != nil {
				return processing.SourceProjection{}, err
			}
			appendField([]string{"unit", "collections"}, processing.FieldSchema{Kind: processing.ListValue, Optional: true,
				Element: &processing.FieldSchema{Kind: processing.StringValue}}, &list, "unit")
		} else {
			appendField([]string{"unit", "id"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, nil, "unit")
			appendField([]string{"unit", "collections"}, processing.FieldSchema{Kind: processing.ListValue, Optional: true,
				Element: &processing.FieldSchema{Kind: processing.StringValue}}, nil, "unit")
		}
		attributeFields, err := unitAttributes.TargetFields(resolution.UnitID, dependency("unit", &index))
		if err != nil {
			return processing.SourceProjection{}, err
		}
		fields = append(fields, attributeFields...)
		projection.Targets = append(projection.Targets, processing.TargetContext{Position: index, Fields: fields})
	}
	declared := declarations.Builtin()
	for _, ref := range declarations.BuiltinSourceRefs() {
		exists := slices.ContainsFunc(projection.Fields, func(field processing.CapturedField) bool {
			return field.Reference.Namespace == ref.Namespace && slices.Equal(field.Reference.Path, ref.Path)
		})
		if exists {
			continue
		}
		schema, err := declared.FieldSchema(ref)
		if err != nil || !schema.Optional {
			return processing.SourceProjection{}, unavailable(errors.New("synthetic projection omitted required declaration"))
		}
		projection.Fields = append(projection.Fields, processing.CapturedField{Reference: ref, Schema: schema,
			Field: processing.FieldValue{State: processing.FieldAbsent, Dependencies: dependency("payload", nil)}})
	}
	return projection, nil
}

func addPayloadFields(projection *processing.SourceProjection, input Input,
	dependency func(string, *uint32) []processing.Dependency) error {
	schema := processing.FieldSchema{Kind: processing.StringValue, Optional: true}
	addString := func(path []string, value string, present bool) error {
		field := processing.FieldValue{State: processing.FieldAbsent}
		if present {
			typed, err := processing.Literal(processing.StringValue, value)
			if err != nil {
				return err
			}
			field.State, field.Value = processing.FieldPresent, &typed
		}
		field.Dependencies = dependency("payload", nil)
		projection.Fields = append(projection.Fields, processing.CapturedField{
			Reference: processing.FieldRef{Namespace: "source", Path: path}, Schema: schema, Field: field})
		return nil
	}
	switch input.Form {
	case "tone-only":
	case "text", "numeric":
		var payload struct {
			Value string `json:"value"`
		}
		if err := processing.Decode(input.Payload, &payload); err != nil {
			return err
		}
		name := "text"
		if input.Form == "numeric" {
			name = "numeric"
		}
		if err := addString([]string{"payload", name, "value"}, payload.Value, true); err != nil {
			return err
		}
	case "binary":
		var payload struct {
			Encoding string `json:"encoding"`
			Data     string `json:"data"`
		}
		if err := processing.Decode(input.Payload, &payload); err != nil {
			return err
		}
		value, err := base64.StdEncoding.DecodeString(payload.Data)
		if err != nil {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid binary Payload"))
		}
		typed, err := processing.Literal(processing.BytesValue, value)
		if err != nil {
			return err
		}
		projection.Fields = append(projection.Fields, processing.CapturedField{
			Reference: processing.FieldRef{Namespace: "source", Path: []string{"payload", "binary"}},
			Schema:    processing.FieldSchema{Kind: processing.BytesValue, Optional: true},
			Field: processing.FieldValue{State: processing.FieldPresent, Value: &typed,
				Dependencies: dependency("payload", nil)}})
	case "flex-short-signal":
		var payload struct {
			SourceCode int `json:"sourceCode"`
		}
		if err := processing.Decode(input.Payload, &payload); err != nil {
			return err
		}
		value, err := processing.Literal(processing.IntegerValue, strconv.Itoa(payload.SourceCode))
		if err != nil {
			return err
		}
		projection.Fields = append(projection.Fields, processing.CapturedField{
			Reference: processing.FieldRef{Namespace: "source", Path: []string{"payload", "flex", "shortSignal", "sourceCode"}},
			Schema:    processing.FieldSchema{Kind: processing.IntegerValue, Optional: true},
			Field: processing.FieldValue{State: processing.FieldPresent, Value: &value,
				Dependencies: dependency("payload", nil)}})
	case "zvei-signal":
		var payload struct {
			DoubleTone json.RawMessage `json:"double_tone"`
		}
		if err := processing.Decode(input.Payload, &payload); err != nil {
			return err
		}
		value := processing.Value{Kind: processing.NullValue, Value: json.RawMessage("null")}
		if !bytes.Equal(bytes.TrimSpace(payload.DoubleTone), []byte("null")) {
			var name string
			if err := processing.Decode(payload.DoubleTone, &name); err != nil {
				return err
			}
			value, _ = processing.Literal(processing.StringValue, name)
		}
		projection.Fields = append(projection.Fields, processing.CapturedField{
			Reference: processing.FieldRef{Namespace: "source", Path: []string{"payload", "zvei", "double_tone"}},
			Schema:    processing.FieldSchema{Kind: processing.StringValue, Optional: true, Nullable: true},
			Field: processing.FieldValue{State: processing.FieldPresent, Value: &value,
				Dependencies: dependency("payload", nil)}})
	case "eas-header":
		var payload struct {
			Originator string `json:"originator"`
			EventCode  string `json:"eventCode"`
			PurgeTime  string `json:"purgeTime"`
			IssueTime  string `json:"issueTime"`
		}
		if err := processing.Decode(input.Payload, &payload); err != nil {
			return err
		}
		for _, field := range []struct{ name, value string }{
			{"originator", payload.Originator}, {"eventCode", payload.EventCode},
			{"purgeTime", payload.PurgeTime}, {"issueTime", payload.IssueTime},
		} {
			if err := addString([]string{"payload", "eas", field.name}, field.value, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func evaluateScope(workflow processing.WorkflowConfiguration, network Network, targets []protocols.Target,
	match assignments.MatchResult) (processing.EvaluationScope, []uuid.UUID, error) {
	scope := workflow.Scope
	if !slices.Contains(scope.Agencies, network.AgencyID) {
		return processing.EvaluationScope{}, nil, authz.ErrForbidden
	}
	everything := processing.EvaluationScope{SourceEligible: true, TargetPositions: allPositions(len(targets))}
	if scope.Narrowing == nil {
		return everything, nil, nil
	}
	switch scope.Narrowing.Kind {
	case "network":
		if !slices.Contains(scope.Narrowing.References, network.ID) {
			return processing.EvaluationScope{}, nil, authz.ErrForbidden
		}
		return everything, nil, nil
	case "unit":
		positions := make([]uint32, 0)
		units := make([]uuid.UUID, 0)
		seen := make(map[uuid.UUID]bool)
		for _, resolution := range match.Resolutions {
			if resolution.TargetPosition < 0 || resolution.TargetPosition >= len(targets) ||
				resolution.UnitID == nil || !slices.Contains(scope.Narrowing.References, *resolution.UnitID) {
				continue
			}
			position := uint32(resolution.TargetPosition)
			positions = append(positions, position)
			if !seen[*resolution.UnitID] {
				seen[*resolution.UnitID] = true
				units = append(units, *resolution.UnitID)
			}
		}
		return processing.EvaluationScope{SourceEligible: len(positions) > 0, TargetPositions: positions}, units, nil
	default:
		return processing.EvaluationScope{}, nil, authz.ErrInvalidInput
	}
}

// assignedUnits returns the Units the resolutions assign.
func assignedUnits(resolutions []assignments.Resolution) []uuid.UUID {
	var ids []uuid.UUID
	for _, resolution := range resolutions {
		if resolution.UnitID != nil {
			ids = append(ids, *resolution.UnitID)
		}
	}
	return ids
}

func allPositions(count int) []uint32 {
	positions := make([]uint32, count)
	for i := range positions {
		positions[i] = uint32(i)
	}
	return positions
}

func cloneUnits(values map[uuid.UUID]units.Context) map[uuid.UUID]units.Context {
	copy := make(map[uuid.UUID]units.Context, len(values))
	for id, value := range values {
		value.CollectionIDs = slices.Clone(value.CollectionIDs)
		copy[id] = value
	}
	return copy
}

func sameResolutions(left, right []assignments.Resolution) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].TargetPosition != right[i].TargetPosition || left[i].Status != right[i].Status ||
			!equalUUIDPointer(left[i].UnitID, right[i].UnitID) || !equalUUIDPointer(left[i].AddressRuleID, right[i].AddressRuleID) {
			return false
		}
	}
	return true
}

// sameUnits compares the current Unit revision and projected membership against capture.
func sameUnits(left, right map[uuid.UUID]units.Context) bool {
	if len(left) != len(right) {
		return false
	}
	for id, before := range left {
		after, ok := right[id]
		if !ok || before.ID != after.ID || before.Revision != after.Revision ||
			before.Deleted != after.Deleted || !slices.Equal(before.CollectionIDs, after.CollectionIDs) {
			return false
		}
	}
	return true
}

func equalUUIDPointer(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func invalid(err error) error { return platform.NewFailure(platform.CodeInvalidInput, err) }
func unavailable(err error) error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
}

var _ management.PreviewSourceOwner = (*Owner)(nil)
var _ processing.SourceDeclarationOwner = (processing.SourceDeclarationOwner)(nil)
