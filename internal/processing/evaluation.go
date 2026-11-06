package processing

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// EvaluationBinding supplies the source revisions and exclusive original-target position bound.
// Workers use the actual original count; a publication fact still requires its source-owner check.
type EvaluationBinding struct {
	TransmissionID  uuid.UUID
	SourceRevision  int64
	ContextRevision int64
	TargetLimit     uint32
}

// ValidateEvaluation checks one whole private/public result without inventing a Message identity.
// It validates all fields/dependencies and encoded size, including explicit not-selected outcomes.
func ValidateEvaluation(result Evaluation, outputSchema map[string]FieldSchema, binding EvaluationBinding) error {
	if binding.TransmissionID == uuid.Nil || binding.SourceRevision <= 0 || binding.ContextRevision <= 0 ||
		binding.TargetLimit > MaxArguments || outputSchema == nil {
		return invalid(errors.New("invalid evaluation binding"))
	}
	if !result.Selected {
		if result.Fields != nil || len(result.TargetResults) != 0 {
			return invalid(errors.New("not-selected evaluation contains a result namespace"))
		}
		return nil
	}
	if (result.Fields == nil) == (len(result.TargetResults) == 0) {
		return invalid(errors.New("evaluation requires one result container"))
	}
	fieldSets := []map[string]FieldValue{result.Fields}
	for i, target := range result.TargetResults {
		if target.TargetPosition >= binding.TargetLimit || target.Fields == nil ||
			(i > 0 && target.TargetPosition <= result.TargetResults[i-1].TargetPosition) {
			return invalid(errors.New("invalid evaluation target context"))
		}
		fieldSets = append(fieldSets, target.Fields)
	}
	for _, fields := range fieldSets {
		if fields == nil {
			continue
		}
		if len(fields) > MaxCollection {
			return resource(errors.New("evaluation field collection boundary"))
		}
		if len(fields) != len(outputSchema) {
			return invalid(errors.New("evaluation fields differ from definition"))
		}
		for name, field := range fields {
			schema, declared := outputSchema[name]
			if name == "" || !utf8.ValidString(name) || strings.ContainsRune(name, '\x00') ||
				!declared {
				return invalid(errors.New("invalid evaluation field state"))
			}
			if err := schema.Validate(); err != nil {
				return err
			}
			if err := ValidateFieldValue(schema, field, false); err != nil {
				return err
			}
			for _, dependency := range field.Dependencies {
				if dependency.TransmissionID != binding.TransmissionID ||
					dependency.SourceRevision != binding.SourceRevision ||
					dependency.ContextRevision != binding.ContextRevision ||
					(dependency.TargetPosition != nil && *dependency.TargetPosition >= binding.TargetLimit) {
					return invalid(errors.New("evaluation dependency differs from source"))
				}
			}
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return invalid(err)
	}
	if len(encoded) > MaxOutputBytes {
		return resource(errors.New("whole evaluation output boundary"))
	}
	decodedBytes, err := decodedBytesInJSON(encoded)
	if err != nil {
		return err
	}
	if decodedBytes > MaxOutputBytes {
		return resource(errors.New("whole evaluation decoded bytes boundary"))
	}
	return nil
}
