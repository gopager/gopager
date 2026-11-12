package delivery

import (
	"context"

	"github.com/google/uuid"
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
	// withoutUnit reports whether a Unit-less appearance is eligible for this view: a split
	// view has none, an unsplit view has the Message's Unit-less appearances.
	withoutUnit bool
	// bound restricts the Unit-attribute candidates to at most one Unit while a filter is
	// evaluated per binding; nil means every relevant Unit contributes.
	bound *[]readmodel.UnitRecord
	// unitSet indexes units for relevantUnit; nil until first use.
	unitSet map[uuid.UUID]struct{}
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
	if isUnitAttribute(ref) {
		return v.unitAttributeCandidates(ref.Path[1:]), field
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

// relevantUnit reports whether the Unit is one of the view's relevant Units. The set is built on
// the first call because a view is asked once per attribute value; its Units must not change after
// that, and a view is not shared between goroutines.
func (v *view) relevantUnit(id uuid.UUID) bool {
	if v.unitSet == nil {
		v.unitSet = make(map[uuid.UUID]struct{}, len(v.units))
		for _, unit := range v.units {
			v.unitSet[unit.UnitID] = struct{}{}
		}
	}
	_, ok := v.unitSet[id]
	return ok
}
