package delivery

import (
	"context"
	"slices"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// view is everything one Delivery's filter and layout can read: the fact, the authorized
// Message or Gateway, the Delivery's relevant Units and the subscriber's readable groups. It
// supplies all candidate values of a field to filters and one value to layouts.
type view struct {
	event        eventView
	message      *readmodel.DeliveryProjection
	units        []readmodel.UnitRecord
	gateway      *GatewayView
	groups       map[string]bool
	declarations fields.Declarations
}

var (
	_ fields.ValueSource     = (*view)(nil)
	_ processing.FieldReader = (*view)(nil)
)

// candidates returns the values a field has in this view, in the order a layout prefers them.
// A field the subscriber cannot read now has none: fields of an explicit filter or layout are
// checked against the groups before this point, the built-in layouts rely on it.
func (v *view) candidates(ref processing.FieldRef) ([]processing.Value, fields.Field) {
	field, err := v.declarations.Field(ref)
	if err != nil || (field.Group != "" && !v.groups[field.Group]) {
		return nil, fields.Field{}
	}
	if ref.Namespace == attributesNamespace {
		return v.attributeCandidates(ref.Path), field
	}
	if len(ref.Path) != 1 {
		return nil, field
	}
	value, ok := v.static(ref.Namespace + "." + ref.Path[0])
	if !ok {
		return nil, field
	}
	return []processing.Value{value}, field
}

// Values implements fields.ValueSource.
func (v *view) Values(ref processing.FieldRef) []processing.Value {
	values, _ := v.candidates(ref)
	return values
}

// LookupField implements processing.FieldReader: the first candidate that decodes against the
// declared type. For an attribute that is the untargeted value, else the one of the lowest
// relevant target position.
func (v *view) LookupField(_ context.Context, ref processing.FieldRef) (processing.FieldValue, error) {
	values, field := v.candidates(ref)
	for _, value := range values {
		if _, err := value.Native(field.Type); err == nil {
			return processing.FieldValue{State: processing.FieldPresent, Value: &value}, nil
		}
	}
	return processing.FieldValue{State: processing.FieldAbsent}, nil
}

// unitIDs returns the IDs of the view's relevant Units.
func (v *view) unitIDs() []string {
	ids := make([]string, len(v.units))
	for i, unit := range v.units {
		ids[i] = unit.UnitID.String()
	}
	return ids
}

// relevantUnit reports whether the Unit is one of the view's relevant Units.
func (v *view) relevantUnit(id string) bool { return slices.Contains(v.unitIDs(), id) }
