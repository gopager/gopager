package processing

// FieldPresentation is implemented by declarations that know how a field is displayed. A Layout
// compiled against such declarations gives a field or chip block without a label or icon the
// declared ones, so the display metadata of a field is maintained in one place.
type FieldPresentation interface {
	// FieldPresentation returns the label and icon of the field, and false when the field has no
	// presentation. Either value may be empty.
	FieldPresentation(FieldRef) (label, icon string, ok bool)
}
