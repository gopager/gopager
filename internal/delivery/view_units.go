package delivery

import (
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// isUnitAttribute reports whether the reference is unit.attributes.<schemaKey>.<field>[…].
func isUnitAttribute(ref processing.FieldRef) bool {
	return ref.Namespace == "unit" && len(ref.Path) >= 3 && ref.Path[0] == attributesNamespace
}

// unitAttributeCandidates resolves <schemaKey>.<field>[.<property>…] over the effective values of
// the Units that contribute: the relevant Units in projection order, or the bound one.
func (v *view) unitAttributeCandidates(path []string) []processing.Value {
	contributing := v.units
	if v.bound != nil {
		contributing = *v.bound
	}
	var values []processing.Value
	for _, unit := range contributing {
		for _, attribute := range unit.Attributes {
			if attribute.SchemaKey != path[0] || attribute.Field != path[1] {
				continue
			}
			if value, ok := descend(attribute.Value, path[2:]); ok {
				values = append(values, value)
			}
		}
	}
	return values
}

// withBound returns a copy of the view in which Unit-attribute candidates come from the given
// Unit only, or from none when unit is nil.
func (v *view) withBound(unit *readmodel.UnitRecord) *view {
	copied := *v
	bound := []readmodel.UnitRecord{}
	if unit != nil {
		bound = append(bound, *unit)
	}
	copied.bound = &bound
	return &copied
}

// readsUnitAttributes reports whether the filter has a unit.attributes leaf.
func readsUnitAttributes(filter *fields.Filter) bool {
	for _, field := range filter.Fields() {
		if isUnitAttribute(field.Ref) {
			return true
		}
	}
	return false
}

// matches evaluates the compiled filter on a split; no filter matches everything. A filter with
// Unit-attribute leaves is correlated like a Message query: it holds when it holds with one
// relevant Unit bound, or with none bound when a Unit-less appearance is eligible or the view has
// no Unit, so that one appearance satisfies the whole predicate.
func matches(filter *fields.Filter, current *view) bool {
	if filter == nil {
		return true
	}
	if !readsUnitAttributes(filter) {
		return filter.Match(current)
	}
	for i := range current.units {
		if filter.Match(current.withBound(&current.units[i])) {
			return true
		}
	}
	if len(current.units) > 0 && !current.withoutUnit {
		return false
	}
	return filter.Match(current.withBound(nil))
}
