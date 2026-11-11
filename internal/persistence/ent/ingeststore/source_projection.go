package ingeststore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

func projectProcessingSource(ctx context.Context, transmission uuid.UUID, network uuid.UUID, networkName string,
	protocol protocols.Protocol, payload protocols.TransmissionPayload, observedAt time.Time,
	sourceRevision int64, matched assignments.MatchResult, targets []protocols.Target,
) (processing.SourceProjection, error) {
	if ctx == nil || transmission == uuid.Nil || network == uuid.Nil || sourceRevision <= 0 ||
		len(targets) != len(matched.Resolutions) {
		return processing.SourceProjection{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid captured Transmission source"))
	}
	if _, err := payload.Identity(protocol); err != nil {
		return processing.SourceProjection{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	if err := protocols.ValidateTransmissionTargets(protocol, targets); err != nil {
		return processing.SourceProjection{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	projection := processing.SourceProjection{
		TransmissionID: transmission, Protocol: protocol, Form: payload.Form, Subtype: payload.Subtype,
		ObservedAt: observedAt, SourceRevision: sourceRevision, ContextRevision: matched.Revision,
		NetworkID: network,
	}
	dependency := func(group string, position *uint32) []processing.Dependency {
		var targetPosition *uint32
		if position != nil {
			value := *position
			targetPosition = &value
		}
		return []processing.Dependency{{TransmissionID: transmission, NetworkID: network,
			SourceRevision: sourceRevision, ContextRevision: matched.Revision,
			TargetPosition: targetPosition, Groups: []string{group}}}
	}
	sourceValues, err := sourcePayloadValues(payload)
	if err != nil {
		return processing.SourceProjection{}, err
	}
	declared := declarations.Builtin()
	for _, reference := range declarations.BuiltinSourceRefs() {
		switch strings.Join(reference.Path, ".") {
		case "protocol", "form", "subtype", "observedAt", "network.id", "network.name":
			continue
		}
		schema, err := declared.FieldSchema(reference)
		if err != nil {
			return processing.SourceProjection{}, err
		}
		field := processing.FieldValue{State: processing.FieldAbsent,
			Dependencies: dependency("payload", nil)}
		if value, ok := sourceValues[fieldReferenceKey(reference)]; ok {
			field.State, field.Value = processing.FieldPresent, &value
		}
		projection.Fields = append(projection.Fields, processing.CapturedField{
			Reference: reference, Schema: schema, Field: field,
		})
	}
	projection, err = projection.WithContextFields(networkName)
	if err != nil {
		return processing.SourceProjection{}, err
	}
	observed, err := processing.Literal(processing.TimestampValue, observedAt.Format(time.RFC3339Nano))
	if err != nil {
		return processing.SourceProjection{}, err
	}
	projection.Fields = append(projection.Fields, processing.CapturedField{
		Reference: processing.FieldRef{Namespace: "source", Path: []string{"observedAt"}},
		Schema:    processing.FieldSchema{Kind: processing.TimestampValue},
		Field: processing.FieldValue{State: processing.FieldPresent, Value: &observed,
			Dependencies: dependency("gateway", nil)},
	})
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
	for position, target := range targets {
		index := uint32(position)
		resolution := matched.Resolutions[position]
		if resolution.TargetPosition != position {
			return processing.SourceProjection{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
				errors.New("current matcher changed original target positions"))
		}
		fields, err := projectTargetFields(transmission, network, sourceRevision, matched.Revision,
			index, target, resolution, matched.UnitContext)
		if err != nil {
			return processing.SourceProjection{}, err
		}
		projection.Targets = append(projection.Targets, processing.TargetContext{Position: index, Fields: fields})
	}
	return projection, nil
}

func sourcePayloadValues(payload protocols.TransmissionPayload) (map[string]processing.Value, error) {
	values := make(map[string]processing.Value)
	add := func(path []string, kind processing.ValueKind, value any) error {
		typed, err := processing.Literal(kind, value)
		if err != nil {
			return err
		}
		values[fieldReferenceKey(processing.FieldRef{Namespace: "source", Path: path})] = typed
		return nil
	}
	switch payload.Form {
	case "text":
		if err := add([]string{"payload", "text", "value"}, processing.StringValue, payload.Value); err != nil {
			return nil, err
		}
	case "numeric":
		if err := add([]string{"payload", "numeric", "value"}, processing.StringValue, payload.Value); err != nil {
			return nil, err
		}
	case "binary":
		if err := add([]string{"payload", "binary"}, processing.BytesValue, payload.Binary); err != nil {
			return nil, err
		}
	case "flex-short-signal":
		if err := add([]string{"payload", "flex", "shortSignal", "sourceCode"},
			processing.IntegerValue, strconv.Itoa(int(*payload.SourceCode))); err != nil {
			return nil, err
		}
	case "zvei-signal":
		var value processing.Value
		if json.Valid(payload.DoubleTone) && bytes.Equal(bytes.TrimSpace(payload.DoubleTone), []byte("null")) {
			value = processing.Value{Kind: processing.NullValue, Value: json.RawMessage("null")}
		} else {
			var tone string
			if err := json.Unmarshal(payload.DoubleTone, &tone); err != nil {
				return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid ZVEI source field"))
			}
			var err error
			value, err = processing.Literal(processing.StringValue, tone)
			if err != nil {
				return nil, err
			}
		}
		values[fieldReferenceKey(processing.FieldRef{Namespace: "source", Path: []string{"payload", "zvei", "double_tone"}})] = value
	case "eas-header":
		for _, field := range []struct {
			name  string
			value string
		}{{"originator", payload.Originator}, {"eventCode", payload.EventCode},
			{"purgeTime", payload.PurgeTime}, {"issueTime", payload.IssueTime}} {
			if err := add([]string{"payload", "eas", field.name}, processing.StringValue, field.value); err != nil {
				return nil, err
			}
		}
	}
	return values, nil
}

func fieldReferenceKey(reference processing.FieldRef) string {
	return reference.Namespace + "\x00" + strings.Join(reference.Path, "\x00")
}

func projectTargetFields(transmission, network uuid.UUID, sourceRevision, contextRevision int64, position uint32,
	target protocols.Target, resolution assignments.Resolution, units map[uuid.UUID]units.Context,
) ([]processing.CapturedField, error) {
	dependency := func(group string) []processing.Dependency {
		index := position
		return []processing.Dependency{{TransmissionID: transmission, NetworkID: network,
			SourceRevision: sourceRevision, ContextRevision: contextRevision, TargetPosition: &index,
			Groups: []string{group}}}
	}
	fields := make([]processing.CapturedField, 0, 5)
	appendField := func(path []string, schema processing.FieldSchema, value *processing.Value, group string) {
		state := processing.FieldAbsent
		if value != nil {
			state = processing.FieldPresent
		}
		fields = append(fields, processing.CapturedField{Reference: processing.FieldRef{Namespace: "target", Path: path},
			Schema: schema, Field: processing.FieldValue{State: state, Value: value, Dependencies: dependency(group)}})
	}
	if number, ok := target.Address.Number(); ok {
		if number > uint64(1<<63-1) {
			return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("target integer exceeds shared typed integer"))
		}
		value, err := processing.Literal(processing.IntegerValue, strconv.FormatUint(number, 10))
		if err != nil {
			return nil, err
		}
		appendField([]string{"address", "number"}, processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, &value, "address")
		appendField([]string{"address", "text"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, nil, "address")
	} else {
		text, ok := target.Address.Text()
		if !ok {
			return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid target Address"))
		}
		value, err := processing.Literal(processing.StringValue, text)
		if err != nil {
			return nil, err
		}
		appendField([]string{"address", "number"}, processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, nil, "address")
		appendField([]string{"address", "text"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, &value, "address")
	}
	var function *processing.Value
	if target.Function != nil {
		value, err := processing.Literal(processing.IntegerValue, strconv.Itoa(int(*target.Function)))
		if err != nil {
			return nil, err
		}
		function = &value
	}
	appendField([]string{"function"}, processing.FieldSchema{Kind: processing.IntegerValue, Optional: true}, function, "address")
	if resolution.UnitID == nil {
		appendField([]string{"unit", "id"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, nil, "unit")
		appendField([]string{"unit", "collections"}, processing.FieldSchema{Kind: processing.ListValue, Optional: true,
			Element: &processing.FieldSchema{Kind: processing.StringValue}}, nil, "unit")
		return fields, nil
	}
	unit, ok := units[*resolution.UnitID]
	if !ok || unit.ID != *resolution.UnitID || unit.Deleted || unit.Revision <= 0 {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("current matcher omitted Unit context"))
	}
	unitID, err := processing.Literal(processing.StringValue, unit.ID.String())
	if err != nil {
		return nil, err
	}
	appendField([]string{"unit", "id"}, processing.FieldSchema{Kind: processing.StringValue, Optional: true}, &unitID, "unit")
	collections := make([]processing.Value, len(unit.CollectionIDs))
	for i, id := range unit.CollectionIDs {
		collections[i], err = processing.Literal(processing.StringValue, id.String())
		if err != nil {
			return nil, err
		}
	}
	list, err := processing.Literal(processing.ListValue, collections)
	if err != nil {
		return nil, err
	}
	appendField([]string{"unit", "collections"}, processing.FieldSchema{Kind: processing.ListValue, Optional: true,
		Element: &processing.FieldSchema{Kind: processing.StringValue}}, &list, "unit")
	return fields, nil
}
