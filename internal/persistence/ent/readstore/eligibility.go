package readstore

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

// params collects the positional parameters of one statement.
type params struct{ values []any }

// add registers a value and returns its placeholder.
func (p *params) add(value any) string {
	p.values = append(p.values, value)
	return "$" + strconv.Itoa(len(p.values))
}

// uuids registers a UUID list as a uuid[] parameter.
func (p *params) uuids(ids []uuid.UUID) string { return p.add(ids) + "::uuid[]" }

// columns names the SQL expressions of the row an eligibility predicate is evaluated for: its
// Agency, its Unit (NULL for a Unit-less row) and its age anchor.
type columns struct{ agency, unit, anchor string }

// eligibility renders authz.MessageAccess as SQL over a row's columns. Equal renderings are
// shared so that a predicate used in several places binds its parameters once.
type eligibility struct {
	params *params
	access authz.MessageAccess
	memo   map[string]string
}

func newEligibility(p *params, access authz.MessageAccess) *eligibility {
	return &eligibility{params: p, access: access, memo: make(map[string]string)}
}

// clause renders one alternative. Ages are skipped for rows without an age anchor.
func (e *eligibility) clause(c authz.AccessClause, cols columns, ages bool) string {
	var parts []string
	if ages && c.Cutoff != nil {
		parts = append(parts, cols.anchor+" >= "+e.params.add(*c.Cutoff))
	}
	switch c.Kind {
	case "agency":
		parts = append(parts, cols.agency+" = ANY("+e.params.uuids(c.References)+")")
	case "unit":
		parts = append(parts, cols.unit+" = ANY("+e.params.uuids(c.References)+")")
	case "unitCollection":
		parts = append(parts, "EXISTS (SELECT 1 FROM unit_collection_memberships ucm WHERE ucm.unit_id = "+cols.unit+
			" AND ucm.collection_id = ANY("+e.params.uuids(c.References)+"))")
	}
	if len(parts) == 0 {
		return "true"
	}
	return "(" + strings.Join(parts, " AND ") + ")"
}

// action renders one action's access: any owner clause AND, with a restricting key, any key
// clause AND the key's age. With agencyLevel only the all and agency clauses count, which
// serves objects without a Unit; with ages false cutoffs are ignored.
func (e *eligibility) action(name string, a authz.ActionAccess, cols columns, agencyLevel, ages bool) string {
	key := strings.Join([]string{name, cols.agency, cols.unit, cols.anchor, strconv.FormatBool(agencyLevel), strconv.FormatBool(ages)}, "|")
	if rendered, ok := e.memo[key]; ok {
		return rendered
	}
	alternatives := func(clauses []authz.AccessClause) string {
		var rendered []string
		for _, c := range clauses {
			if agencyLevel && c.Kind != "all" && c.Kind != "agency" {
				continue
			}
			rendered = append(rendered, e.clause(c, cols, ages))
		}
		if len(rendered) == 0 {
			return "false"
		}
		return "(" + strings.Join(rendered, " OR ") + ")"
	}
	result := alternatives(a.Owner)
	if a.Restricted {
		result += " AND " + alternatives(a.Key)
		if ages && a.KeyCutoff != nil {
			result += " AND " + cols.anchor + " >= " + e.params.add(*a.KeyCutoff)
		}
	}
	result = "(" + result + ")"
	e.memo[key] = result
	return result
}

// appearance renders the read eligibility of an appearance row: message.read and, for a
// Unit-less row, without-unit.read as well.
func (e *eligibility) appearance(cols columns) string {
	read := e.action("read", e.access.Read, cols, false, true)
	withoutUnit := e.action("without-unit", e.access.WithoutUnit, cols, false, true)
	return "(" + read + " AND (" + cols.unit + " IS NOT NULL OR " + withoutUnit + "))"
}

// appearanceDetail adds message.detail to the read eligibility.
func (e *eligibility) appearanceDetail(cols columns) string {
	return "(" + e.appearance(cols) + " AND " + e.action("detail", e.access.Detail, cols, false, true) + ")"
}

// context renders the eligibility of a target without an appearance, such as a failed one:
// message.read and message.detail on the Network's Agency.
func (e *eligibility) context(cols columns) string {
	return "(" + e.action("read", e.access.Read, cols, false, true) + " AND " +
		e.action("detail", e.access.Detail, cols, false, true) + ")"
}
