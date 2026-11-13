package delivery

import (
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// MessageFieldReader returns the field reader a Delivery layout reads one Message with, for a
// rendering without a Domain event: the projection's Message, all of its eligible Unit
// appearances and its readable groups. Event and Gateway fields are absent. A layout rendered
// with it shows the same values as a Delivery of the same Message to the same reader.
func MessageFieldReader(projection readmodel.DeliveryProjection, declarations fields.Declarations) processing.FieldReader {
	groups := make(map[string]bool, len(projection.Groups))
	for _, group := range projection.Groups {
		groups[group] = true
	}
	message := projection
	return &view{message: &message, units: projection.Units, groups: groups, declarations: declarations,
		withoutUnit: len(projection.WithoutUnit) > 0, withoutEvent: true}
}
