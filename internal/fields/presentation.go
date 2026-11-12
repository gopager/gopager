package fields

import "github.com/gopager/gopager/internal/processing"

// FieldPresentation implements processing.FieldPresentation: the label and icon of the field.
func (d Declarations) FieldPresentation(ref processing.FieldRef) (label, icon string, ok bool) {
	field, err := d.Field(ref)
	if err != nil {
		return "", "", false
	}
	return field.Label, field.Icon, true
}

var _ processing.FieldPresentation = Declarations{}
