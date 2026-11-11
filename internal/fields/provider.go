package fields

import "github.com/gopager/gopager/internal/processing"

// Provider contributes fields to a Registry. Implementations are immutable snapshots: the
// same Provider value always reports the same fields and revision.
type Provider interface {
	// Field returns the field a reference names and whether the provider declares it.
	Field(ref processing.FieldRef) (Field, bool)
	// Fields returns every field the provider declares. The result is read-only.
	Fields() []Field
	// Revision identifies the provider's content for cache keys; 0 for static content.
	Revision() int64
}

// staticProvider serves a fixed field list with constant-time lookup.
type staticProvider struct {
	fields   []Field
	index    map[string]Field
	revision int64
}

// NewStaticProvider returns a Provider over a fixed field list with the given revision. A
// later entry for the same reference replaces an earlier one. The caller must not modify the
// fields afterwards.
func NewStaticProvider(revision int64, fields []Field) Provider {
	index := make(map[string]Field, len(fields))
	for _, field := range fields {
		index[refKey(field.Ref)] = field
	}
	return &staticProvider{fields: fields, index: index, revision: revision}
}

func (p *staticProvider) Field(ref processing.FieldRef) (Field, bool) {
	field, ok := p.index[refKey(ref)]
	return field, ok
}

func (p *staticProvider) Fields() []Field { return p.fields }

func (p *staticProvider) Revision() int64 { return p.revision }
