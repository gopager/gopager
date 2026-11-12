package processing

import (
	"errors"

	"github.com/google/uuid"
)

// UnitParameters are the per-Unit input values of one Delivery instance, keyed by the name of a
// declared Delivery input.
type UnitParameters struct {
	UnitID uuid.UUID        `json:"unitId"`
	Values map[string]Value `json:"values"`
}

// ValidateDeliveryInput checks a value against its input declaration: the declared kind and the
// encoded size bound (the layout output bound when the declaration sets none).
func ValidateDeliveryInput(declaration DeliveryInputDeclaration, value Value) error {
	if _, err := value.Native(FieldSchema{Kind: declaration.Kind}); err != nil {
		return err
	}
	limit := declaration.MaxBytes
	if limit <= 0 || limit > MaxLayoutOutputBytes {
		limit = MaxLayoutOutputBytes
	}
	if len(value.Value) > limit {
		return resource(errors.New("input value too large"))
	}
	return nil
}

// ValidateUnitParameters checks a Unit's values against the manifest's Delivery inputs: every key
// is a declared input and every value is valid for it. A manifest without a Delivery declaration
// accepts nothing.
func (m InstalledManifest) ValidateUnitParameters(values map[string]Value) error {
	if m.EffectiveCapability() != CapabilityDelivery || m.Delivery == nil {
		return invalid(errors.New("plugin is not a delivery capability"))
	}
	if len(values) > maxDeliveryInputs {
		return invalid(errors.New("too many unit parameters"))
	}
	for name, value := range values {
		found := false
		for _, declaration := range m.Delivery.Inputs {
			if declaration.Name != name {
				continue
			}
			found = true
			if err := ValidateDeliveryInput(declaration, value); err != nil {
				return invalid(err)
			}
		}
		if !found {
			return invalid(errors.New("undeclared unit parameter"))
		}
	}
	return nil
}
