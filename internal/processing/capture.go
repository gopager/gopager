package processing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// CapturedField is an owner-declared, classified value, never a reflected arbitrary property.
type CapturedField struct {
	Reference FieldRef    `json:"reference"`
	Schema    FieldSchema `json:"schema"`
	Field     FieldValue  `json:"field"`
}

// FieldDeclaration identifies a closed field in a trusted owner's projection schema.
type FieldDeclaration struct {
	Reference FieldRef    `json:"reference"`
	Schema    FieldSchema `json:"schema"`
	Groups    []string    `json:"groups"`
}

// TargetContext preserves one original position and its current owner-declared projection.
type TargetContext struct {
	Position uint32          `json:"position"`
	Fields   []CapturedField `json:"fields"`
}

// SourceProjection is supplied by the Protocol/original/context owner after validation.
// It is not a public synthetic-input DTO; untrusted callers cannot supply its declarations.
type SourceProjection struct {
	TransmissionID     uuid.UUID          `json:"transmissionId"`
	Synthetic          bool               `json:"synthetic,omitempty"`
	Protocol           protocols.Protocol `json:"protocol"`
	Form               string             `json:"form"`
	Subtype            string             `json:"subtype,omitempty"`
	ObservedAt         time.Time          `json:"observedAt"`
	SourceRevision     int64              `json:"sourceRevision,string"`
	ContextRevision    int64              `json:"contextRevision,string"`
	NetworkID          uuid.UUID          `json:"networkId"`
	Fields             []CapturedField    `json:"fields"`
	TargetDeclarations []FieldDeclaration `json:"targetDeclarations"`
	Targets            []TargetContext    `json:"targets"`
}

// WithContextFields appends classified owner-validated Protocol/form and resolved Network context.
// networkName comes from the current Network owner; callers apply visibility masking before capture.
func (p SourceProjection) WithContextFields(networkName string) (SourceProjection, error) {
	if p.TransmissionID == uuid.Nil || p.NetworkID == uuid.Nil || p.SourceRevision <= 0 ||
		p.ContextRevision <= 0 || !p.Protocol.Valid() || p.Form == "" {
		return SourceProjection{}, invalid(errors.New("invalid source context identity"))
	}
	contextFields := []struct {
		path     []string
		value    string
		group    string
		optional bool
	}{
		{[]string{"protocol"}, string(p.Protocol), "payload", false},
		{[]string{"form"}, p.Form, "payload", false},
		{[]string{"subtype"}, p.Subtype, "payload", true},
		{[]string{"network", "id"}, p.NetworkID.String(), "gateway", false},
		{[]string{"network", "name"}, networkName, "gateway", false},
	}
	p.Fields = slices.Clone(p.Fields)
	for _, contextField := range contextFields {
		if !utf8.ValidString(contextField.value) {
			return SourceProjection{}, invalid(errors.New("source context is not UTF-8"))
		}
		reference := FieldRef{Namespace: "source", Path: contextField.path}
		for _, existing := range p.Fields {
			if existing.Reference.key() == reference.key() {
				return SourceProjection{}, invalid(errors.New("source context field already declared"))
			}
		}
		field := FieldValue{State: FieldAbsent, Dependencies: []Dependency{{
			TransmissionID: p.TransmissionID, NetworkID: p.NetworkID,
			SourceRevision: p.SourceRevision, ContextRevision: p.ContextRevision,
			Groups: []string{contextField.group},
		}}}
		if !contextField.optional || contextField.value != "" {
			value, err := Literal(StringValue, contextField.value)
			if err != nil {
				return SourceProjection{}, err
			}
			field.State, field.Value = FieldPresent, &value
		}
		p.Fields = append(p.Fields, CapturedField{Reference: reference,
			Schema: FieldSchema{Kind: StringValue, Optional: contextField.optional}, Field: field})
	}
	return p, nil
}

// CapturedSource owns a validated detached copy for evaluation outside primary transactions.
type CapturedSource struct {
	projection    SourceProjection
	source        map[string]CapturedField
	targets       []map[string]CapturedField
	targetSchemas map[string]FieldSchema
	targetGroups  map[string][]string
	encodedSize   int
}

// SourceIdentity identifies an owner-captured source without exposing its field values.
type SourceIdentity struct {
	TransmissionID      uuid.UUID
	NetworkID           uuid.UUID
	SourceRevision      int64
	ContextRevision     int64
	OriginalTargetCount uint32
	Synthetic           bool
}

// Identity returns immutable reference metadata for trusted execution/publication participants.
func (s *CapturedSource) Identity() (SourceIdentity, error) {
	if s == nil {
		return SourceIdentity{}, invalid(errors.New("missing captured source"))
	}
	return SourceIdentity{TransmissionID: s.projection.TransmissionID, NetworkID: s.projection.NetworkID,
		SourceRevision: s.projection.SourceRevision, ContextRevision: s.projection.ContextRevision,
		OriginalTargetCount: uint32(len(s.targets)), Synthetic: s.projection.Synthetic}, nil
}

// CaptureSource validates a trusted owner projection and copies all mutable data.
// Protocol/form interpretation remains a required producer responsibility, not inferred JSON.
func CaptureSource(ctx context.Context, projection SourceProjection) (*CapturedSource, error) {
	if ctx == nil {
		return nil, invalid(errors.New("missing source capture context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if projection.TransmissionID == uuid.Nil || projection.NetworkID == uuid.Nil ||
		projection.SourceRevision <= 0 || projection.ContextRevision <= 0 ||
		!projection.Protocol.Valid() || projection.Form == "" || projection.ObservedAt.IsZero() {
		return nil, invalid(errors.New("invalid source projection identity"))
	}
	if len(projection.Targets) > MaxArguments {
		return nil, resource(errors.New("source target boundary"))
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return nil, invalid(err)
	}
	var detached SourceProjection
	if err := Decode(encoded, &detached); err != nil {
		return nil, err
	}
	captured := &CapturedSource{projection: detached, source: make(map[string]CapturedField),
		targetSchemas: make(map[string]FieldSchema), targetGroups: make(map[string][]string), encodedSize: len(encoded)}
	for _, declaration := range detached.TargetDeclarations {
		if err := declaration.Reference.validate(); err != nil {
			return nil, err
		}
		if declaration.Reference.Namespace != "target" {
			return nil, invalid(errors.New("invalid target declaration namespace"))
		}
		if err := declaration.Schema.Validate(); err != nil {
			return nil, err
		}
		if err := (Dependency{TransmissionID: detached.TransmissionID, NetworkID: detached.NetworkID,
			SourceRevision: detached.SourceRevision, ContextRevision: detached.ContextRevision,
			Groups: declaration.Groups}).validate(); err != nil {
			return nil, err
		}
		key := declaration.Reference.key()
		if _, exists := captured.targetSchemas[key]; exists {
			return nil, invalid(errors.New("duplicate target declaration"))
		}
		captured.targetSchemas[key] = declaration.Schema
		captured.targetGroups[key] = declaration.Groups
	}
	if err := captured.addFields(ctx, captured.source, detached.Fields, "source", nil); err != nil {
		return nil, err
	}
	for i, target := range detached.Targets {
		if target.Position != uint32(i) {
			return nil, invalid(errors.New("source target positions are not original order"))
		}
		fields := make(map[string]CapturedField)
		if err := captured.addFields(ctx, fields, target.Fields, "target", &target.Position); err != nil {
			return nil, err
		}
		for key, field := range fields {
			if previous, ok := captured.targetSchemas[key]; ok {
				left, err := json.Marshal(previous)
				if err != nil {
					return nil, invalid(err)
				}
				right, err := json.Marshal(field.Schema)
				if err != nil {
					return nil, invalid(err)
				}
				if string(left) != string(right) {
					return nil, invalid(errors.New("target declarations differ by context"))
				}
			} else {
				return nil, invalid(errors.New("target field has no owner declaration"))
			}
		}
		captured.targets = append(captured.targets, fields)
	}
	for _, fields := range captured.targets {
		if len(fields) != len(captured.targetSchemas) {
			return nil, invalid(errors.New("target context omitted a field state"))
		}
	}
	return captured, nil
}

// FieldSchema returns detached owner declarations; it never exposes source data.
func (s *CapturedSource) FieldSchema(ref FieldRef) (FieldSchema, error) {
	if s == nil {
		return FieldSchema{}, invalid(errors.New("missing captured source"))
	}
	if field, ok := s.source[ref.key()]; ok {
		return cloneSchema(field.Schema), nil
	}
	if schema, ok := s.targetSchemas[ref.key()]; ok {
		return cloneSchema(schema), nil
	}
	return FieldSchema{}, invalid(errors.New("undeclared captured field"))
}

type capturedView struct {
	source *CapturedSource
	target *uint32
}

func cloneField(field FieldValue) (FieldValue, error) {
	var err error
	field.Dependencies, err = UnionDependencies(field.Dependencies)
	if err != nil {
		return FieldValue{}, err
	}
	if field.Value != nil {
		value := *field.Value
		value.Value = slices.Clone(value.Value)
		field.Value = &value
	}
	return field, nil
}

func (v capturedView) LookupField(ctx context.Context, ref FieldRef) (FieldValue, error) {
	if err := ctx.Err(); err != nil {
		return FieldValue{}, err
	}
	if field, ok := v.source.source[ref.key()]; ok {
		return cloneField(field.Field)
	}
	if _, ok := v.source.targetSchemas[ref.key()]; !ok {
		return FieldValue{}, invalid(errors.New("undeclared captured field"))
	}
	if v.target == nil {
		return FieldValue{State: FieldAbsent, Dependencies: []Dependency{{
			TransmissionID: v.source.projection.TransmissionID, NetworkID: v.source.projection.NetworkID,
			SourceRevision: v.source.projection.SourceRevision, ContextRevision: v.source.projection.ContextRevision,
			Groups: v.source.targetGroups[ref.key()],
		}}}, nil
	}
	return cloneField(v.source.targets[*v.target][ref.key()].Field)
}

func (s *CapturedSource) addFields(ctx context.Context, into map[string]CapturedField, fields []CapturedField, namespace string, position *uint32) error {
	for _, field := range fields {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := field.Reference.validate(); err != nil {
			return err
		}
		if field.Reference.Namespace != namespace {
			return invalid(errors.New("source owner supplied a foreign namespace"))
		}
		key := field.Reference.key()
		if _, exists := into[key]; exists {
			return invalid(errors.New("duplicate projected field"))
		}
		if err := field.Schema.Validate(); err != nil {
			return err
		}
		if err := validateFieldValue(field.Schema, field.Field, true); err != nil {
			return err
		}
		for _, dependency := range field.Field.Dependencies {
			if dependency.TransmissionID != s.projection.TransmissionID ||
				dependency.NetworkID != s.projection.NetworkID ||
				dependency.SourceRevision != s.projection.SourceRevision ||
				dependency.ContextRevision != s.projection.ContextRevision ||
				(position == nil && dependency.TargetPosition != nil) ||
				(position != nil && (dependency.TargetPosition == nil || *dependency.TargetPosition != *position)) {
				return invalid(errors.New("projected dependency does not identify its source/context"))
			}
		}
		into[key] = field
	}
	return nil
}

func validateFieldValue(schema FieldSchema, field FieldValue, requireDependencies bool) error {
	if !slices.Contains([]FieldState{FieldPresent, FieldAbsent, FieldHidden}, field.State) ||
		(field.State == FieldPresent && field.Value == nil) || (field.State != FieldPresent && field.Value != nil) ||
		(field.State == FieldAbsent && !schema.Optional) || (requireDependencies && len(field.Dependencies) == 0) {
		return invalid(errors.New("invalid classified field state"))
	}

	if field.State == FieldPresent {
		if _, err := field.Value.Native(schema); err != nil {
			return err
		}
	}
	_, err := UnionDependencies(field.Dependencies)
	return err
}

// ValidateFieldValue validates a classified field against its immutable owner's declaration.
func ValidateFieldValue(schema FieldSchema, field FieldValue, requireDependencies bool) error {
	return validateFieldValue(schema, field, requireDependencies)
}

// SourceOwner captures originals/current contexts under its own current authorization.
// The caller commits/closes that transaction before running any evaluator or guest.
type SourceOwner interface {
	Capture(context.Context, platform.TransactionContext, uuid.UUID) (*CapturedSource, error)
}

// MissingSourceOwner is a placeholder SourceOwner for deployments without a real Transmission
// source owner; every real-source operation fails as authority-unavailable.
type MissingSourceOwner struct{}

// Capture reports unavailable authority rather than substituting synthetic/raw data.
func (MissingSourceOwner) Capture(context.Context, platform.TransactionContext, uuid.UUID) (*CapturedSource, error) {
	return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("original source owner unavailable"))
}
