package authz

// FieldError is invalid input at one named request field. It matches ErrInvalidInput, so callers
// that only care about the class keep working, and lets the transport name the field.
type FieldError struct {
	// Field is the JSON name of the offending request field.
	Field string
}

// Error returns the safe English fallback of ErrInvalidInput; the field name is exposed only
// through Field.
func (e *FieldError) Error() string { return ErrInvalidInput.Error() }

// Unwrap makes the error match ErrInvalidInput.
func (e *FieldError) Unwrap() error { return ErrInvalidInput }

// InvalidField returns invalid input naming the request field.
func InvalidField(field string) error { return &FieldError{Field: field} }
