package processing

import (
	"context"
	"errors"
	"slices"

	"github.com/gopager/gopager/internal/platform"
)

type syntheticDeclarationOwner struct{}

type syntheticDeclarations struct {
	fields map[string]FieldSchema
}

// syntheticFieldGroups declares the field group of every catalog field. They mirror the
// groups the projection owners attach to the captured values, so a definition is classified
// at save time exactly as its evaluation is later classified.
func syntheticFieldGroups(ref FieldRef) (string, bool) {
	switch ref.Namespace {
	case "source":
		switch ref.Path[0] {
		case "protocol", "form", "subtype", "payload":
			return "payload", true
		case "observedAt", "network":
			return "gateway", true
		}
	case "target":
		switch ref.Path[0] {
		case "address", "function":
			return "address", true
		case "unit":
			return "unit", true
		}
	}
	return "", false
}

// NewSyntheticDeclarationOwner returns the closed, server-owned catalog of field declarations for
// synthetic preview sources (one fixed schema per Protocol form); it is not configurable.
func NewSyntheticDeclarationOwner() SourceDeclarationOwner {
	return syntheticDeclarationOwner{}
}

func (syntheticDeclarationOwner) ProcessingDeclarations(ctx context.Context) (Declarations, error) {
	if ctx == nil {
		return nil, invalid(errors.New("missing synthetic declaration context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stringField := FieldSchema{Kind: StringValue, Optional: true}
	fields := map[string]FieldSchema{
		(sourceFieldKey("protocol")):                                     {Kind: StringValue},
		(sourceFieldKey("form")):                                         {Kind: StringValue},
		(sourceFieldKey("subtype")):                                      {Kind: StringValue, Optional: true},
		(sourceFieldKey("observedAt")):                                   {Kind: TimestampValue},
		(sourceFieldKey("network", "id")):                                {Kind: StringValue},
		(sourceFieldKey("network", "name")):                              {Kind: StringValue},
		(sourceFieldKey("payload", "text", "value")):                     stringField,
		(sourceFieldKey("payload", "numeric", "value")):                  stringField,
		(sourceFieldKey("payload", "binary")):                            {Kind: BytesValue, Optional: true},
		(sourceFieldKey("payload", "flex", "shortSignal", "sourceCode")): {Kind: IntegerValue, Optional: true},
		(sourceFieldKey("payload", "zvei", "double_tone")):               {Kind: StringValue, Optional: true, Nullable: true},
		(sourceFieldKey("payload", "eas", "originator")):                 stringField,
		(sourceFieldKey("payload", "eas", "eventCode")):                  stringField,
		(sourceFieldKey("payload", "eas", "purgeTime")):                  stringField,
		(sourceFieldKey("payload", "eas", "issueTime")):                  stringField,
		(targetFieldKey("address", "number")):                            {Kind: IntegerValue, Optional: true},
		(targetFieldKey("address", "text")):                              stringField,
		(targetFieldKey("function")):                                     {Kind: IntegerValue, Optional: true},
		(targetFieldKey("unit", "id")):                                   stringField,
		(targetFieldKey("unit", "collections")): {Kind: ListValue, Optional: true,
			Element: &FieldSchema{Kind: StringValue}},
	}
	return syntheticDeclarations{fields: fields}, nil
}

func (d syntheticDeclarations) FieldSchema(ref FieldRef) (FieldSchema, error) {
	schema, ok := d.fields[ref.key()]
	if !ok {
		return FieldSchema{}, platform.NewFailure(platform.CodeInvalidInput,
			errors.New("field is outside the trusted synthetic declaration catalog"))
	}
	schema.Properties = cloneSchema(schema).Properties
	if schema.Element != nil {
		element := *schema.Element
		schema.Element = &element
	}
	return schema, nil
}

// FieldGroups returns the field group declared for a catalog field. It implements
// FieldGroupDeclarations so save-time validation can judge a definition by the author's
// readable groups.
func (d syntheticDeclarations) FieldGroups(ref FieldRef) ([]string, error) {
	if _, ok := d.fields[ref.key()]; !ok {
		return nil, platform.NewFailure(platform.CodeInvalidInput,
			errors.New("field is outside the trusted synthetic declaration catalog"))
	}
	group, ok := syntheticFieldGroups(ref)
	if !ok {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("synthetic field declares no field group"))
	}
	return []string{group}, nil
}

func sourceFieldKey(path ...string) string {
	return (FieldRef{Namespace: "source", Path: path}).key()
}

func targetFieldKey(path ...string) string {
	return (FieldRef{Namespace: "target", Path: path}).key()
}

// SyntheticSourceDeclarationRefs returns the server-owned source fields available to definitions.
func SyntheticSourceDeclarationRefs() []FieldRef {
	refs := []FieldRef{
		{Namespace: "source", Path: []string{"protocol"}},
		{Namespace: "source", Path: []string{"form"}},
		{Namespace: "source", Path: []string{"subtype"}},
		{Namespace: "source", Path: []string{"observedAt"}},
		{Namespace: "source", Path: []string{"network", "id"}},
		{Namespace: "source", Path: []string{"network", "name"}},
		{Namespace: "source", Path: []string{"payload", "text", "value"}},
		{Namespace: "source", Path: []string{"payload", "numeric", "value"}},
		{Namespace: "source", Path: []string{"payload", "binary"}},
		{Namespace: "source", Path: []string{"payload", "flex", "shortSignal", "sourceCode"}},
		{Namespace: "source", Path: []string{"payload", "zvei", "double_tone"}},
		{Namespace: "source", Path: []string{"payload", "eas", "originator"}},
		{Namespace: "source", Path: []string{"payload", "eas", "eventCode"}},
		{Namespace: "source", Path: []string{"payload", "eas", "purgeTime"}},
		{Namespace: "source", Path: []string{"payload", "eas", "issueTime"}},
	}
	slices.SortFunc(refs, func(a, b FieldRef) int { return stringsCompare(a.key(), b.key()) })
	return refs
}

func stringsCompare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
