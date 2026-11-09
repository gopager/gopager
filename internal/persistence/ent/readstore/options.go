package readstore

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

// allReach reports whether the message.read access covers every object: an all-scope owner
// grant and, with a restricting key, an all-scope key scope. Ages do not limit options.
func allReach(access authz.ActionAccess) bool {
	has := func(clauses []authz.AccessClause) bool {
		for _, clause := range clauses {
			if clause.Kind == "all" {
				return true
			}
		}
		return false
	}
	return has(access.Owner) && (!access.Restricted || has(access.Key))
}

// Options implements readmodel.Store. Units are those the message.read scopes reach, with their
// Organizations and Agencies; Networks are those of Agencies reached by an all or agency scope.
func (s *Store) Options(ctx context.Context, tx platform.TransactionContext, query readmodel.OptionsQuery) (readmodel.OptionsRecord, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return readmodel.OptionsRecord{}, err
	}
	p := &params{}
	elig := newEligibility(p, query.Access)
	read := query.Access.Read
	everything := allReach(read)
	unitReach := elig.action("read", read, columns{agency: "u.agency_id", unit: "u.id"}, false, false)
	var record readmodel.OptionsRecord
	if query.Units {
		rows, err := executor.QueryContext(ctx, `SELECT u.id, u.name, u.agency_id, u.organization_id FROM units u
			WHERE NOT u.deleted AND `+unitReach+` ORDER BY u.name, u.id`, p.values...)
		if err != nil {
			return readmodel.OptionsRecord{}, err
		}
		for rows.Next() {
			var unit readmodel.UnitOption
			if err := rows.Scan(&unit.ID, &unit.Name, &unit.AgencyID, &unit.OrganizationID); err != nil {
				_ = rows.Close()
				return readmodel.OptionsRecord{}, err
			}
			record.Units = append(record.Units, unit)
		}
		if err := closeRows(rows); err != nil {
			return readmodel.OptionsRecord{}, err
		}
	}
	if query.Organizations {
		p = &params{}
		elig = newEligibility(p, query.Access)
		reach := elig.action("read", read, columns{agency: "u.agency_id", unit: "u.id"}, false, false)
		named, err := s.named(ctx, executor, `SELECT o.id, o.name FROM organizations o WHERE NOT o.deleted AND (`+
			boolSQL(everything)+` OR EXISTS (SELECT 1 FROM units u WHERE u.organization_id = o.id AND NOT u.deleted AND `+reach+
			`)) ORDER BY o.name, o.id`, p.values)
		if err != nil {
			return readmodel.OptionsRecord{}, err
		}
		record.Organizations = named
	}
	if query.Agencies {
		p = &params{}
		elig = newEligibility(p, query.Access)
		reach := elig.action("read", read, columns{agency: "u.agency_id", unit: "u.id"}, false, false)
		level := elig.action("read", read, columns{agency: "a.id"}, true, false)
		named, err := s.named(ctx, executor, `SELECT a.id, a.name FROM agencies a WHERE `+boolSQL(everything)+` OR `+level+
			` OR EXISTS (SELECT 1 FROM units u WHERE u.agency_id = a.id AND NOT u.deleted AND `+reach+`) ORDER BY a.name, a.id`, p.values)
		if err != nil {
			return readmodel.OptionsRecord{}, err
		}
		record.Agencies = named
	}
	if query.Networks {
		p = &params{}
		elig = newEligibility(p, query.Access)
		level := elig.action("read", read, columns{agency: "n.agency_id"}, true, false)
		named, err := s.named(ctx, executor, `SELECT n.id, n.name FROM networks n WHERE NOT n.deleted AND (`+boolSQL(everything)+
			` OR `+level+`) ORDER BY n.name, n.id`, p.values)
		if err != nil {
			return readmodel.OptionsRecord{}, err
		}
		record.Networks = named
	}
	return record, nil
}

// boolSQL renders a Go boolean as a SQL literal.
func boolSQL(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// named runs a query of (id, name) rows.
func (s *Store) named(ctx context.Context, executor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, statement string, args []any) ([]readmodel.NamedOption, error) {
	rows, err := executor.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	var out []readmodel.NamedOption
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, readmodel.NamedOption{ID: id, Name: name})
	}
	return out, closeRows(rows)
}

// closeRows reports an iteration failure and closes the rows.
func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}
