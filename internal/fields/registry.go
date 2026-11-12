package fields

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/gopager/gopager/internal/processing"
)

// Registry combines providers into one field catalog. It is immutable and safe for
// concurrent use; build a new one when a provider changes.
type Registry struct {
	providers []Provider
}

// New returns a Registry over the providers. When several declare the same reference, the
// first one wins.
func New(providers ...Provider) *Registry {
	return &Registry{providers: slices.Clone(providers)}
}

// Lookup returns the field a reference names in a scope. It fails with CodeInvalidInput when
// no provider declares the reference, when the field is outside the scope, or, with strict,
// when the field is deprecated.
func (r *Registry) Lookup(scope Scope, ref processing.FieldRef, strict bool) (Field, error) {
	if err := validRef(ref); err != nil {
		return Field{}, err
	}
	for _, provider := range r.providers {
		field, ok := provider.Field(ref)
		if !ok {
			continue
		}
		if !field.In(scope) {
			return Field{}, invalidInput("field is not available here")
		}
		if strict && field.Deprecated {
			return Field{}, invalidInput("field is deprecated")
		}
		return field, nil
	}
	return Field{}, invalidInput("unknown field")
}

// List returns the non-deprecated fields of a scope, sorted by Path.
func (r *Registry) List(scope Scope) []Field {
	return r.list(scope, false)
}

// ListAll returns every field of a scope, deprecated ones included, sorted by Path. Owners that
// capture values use it: a deprecation hides a field from new references, but a reference saved
// before must keep its value.
func (r *Registry) ListAll(scope Scope) []Field {
	return r.list(scope, true)
}

func (r *Registry) list(scope Scope, withDeprecated bool) []Field {
	seen := make(map[string]bool)
	var result []Field
	for _, provider := range r.providers {
		for _, field := range provider.Fields() {
			key := refKey(field.Ref)
			if seen[key] {
				continue
			}
			seen[key] = true
			if field.In(scope) && (withDeprecated || !field.Deprecated) {
				result = append(result, field)
			}
		}
	}
	slices.SortFunc(result, func(a, b Field) int { return cmp.Compare(a.Path(), b.Path()) })
	return result
}

// Revision returns a stable string identifying the content of all providers, for cache keys.
func (r *Registry) Revision() string {
	parts := make([]string, len(r.providers))
	for i, provider := range r.providers {
		parts[i] = strconv.FormatInt(provider.Revision(), 10)
	}
	return strings.Join(parts, ".")
}

// Declarations returns the registry's view for one scope. With strict, deprecated fields are
// rejected.
func (r *Registry) Declarations(scope Scope, strict bool) Declarations {
	return Declarations{registry: r, scope: scope, strict: strict}
}

// Declarations is the Registry restricted to one scope. It implements
// processing.Declarations and processing.FieldGroupDeclarations.
type Declarations struct {
	registry *Registry
	scope    Scope
	strict   bool
}

// Field returns the declared field of a reference, for example to read its label or icon.
func (d Declarations) Field(ref processing.FieldRef) (Field, error) {
	return d.registry.Lookup(d.scope, ref, d.strict)
}

// FieldSchema implements processing.Declarations.
func (d Declarations) FieldSchema(ref processing.FieldRef) (processing.FieldSchema, error) {
	field, err := d.Field(ref)
	if err != nil {
		return processing.FieldSchema{}, err
	}
	return field.Type, nil
}

// FieldGroups implements processing.FieldGroupDeclarations: the field's group, or an empty
// slice when reading it needs no field permission.
func (d Declarations) FieldGroups(ref processing.FieldRef) ([]string, error) {
	field, err := d.Field(ref)
	if err != nil {
		return nil, err
	}
	if field.Group == "" {
		return []string{}, nil
	}
	return []string{field.Group}, nil
}

var (
	_ processing.Declarations           = Declarations{}
	_ processing.FieldGroupDeclarations = Declarations{}
)
