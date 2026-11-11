// Package agencyscope translates an authz.AgencyVisibility into a PostgreSQL condition for the
// raw-SQL stores of Agency-bound definitions.
package agencyscope

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/jackc/pgx/v5/pgconn"
)

// Condition returns a boolean SQL expression that is true when the Agency in column is visible,
// and the arguments extended by the placeholders it uses. column must be a trusted SQL
// expression of type uuid, never caller input; all values travel as parameters.
func Condition(visibility authz.AgencyVisibility, column string, args []any) (string, []any) {
	terms := make([]string, 0, len(visibility.Terms))
	for _, term := range visibility.Terms {
		var grant, key string
		grant, args = filter(term.Grant, column, args)
		if term.Key == nil {
			terms = append(terms, grant)
			continue
		}
		key, args = filter(*term.Key, column, args)
		terms = append(terms, "("+grant+" AND "+key+")")
	}
	if len(terms) == 0 {
		return "FALSE", args
	}
	return "(" + strings.Join(terms, " OR ") + ")", args
}

// filter renders one AgencyFilter.
func filter(f authz.AgencyFilter, column string, args []any) (string, []any) {
	if f.All {
		return "TRUE", args
	}
	var parts []string
	add := func(ids []uuid.UUID, format string) {
		if len(ids) == 0 {
			return
		}
		values := make([]string, len(ids))
		for i, id := range ids {
			values[i] = id.String()
		}
		args = append(args, values)
		parts = append(parts, fmt.Sprintf(format, column, len(args)))
	}
	add(f.Agencies, "%s = ANY($%d::uuid[])")
	add(f.Units, "%s IN (SELECT agency_id FROM units WHERE NOT deleted AND id = ANY($%d::uuid[]))")
	add(f.Collections, `%s IN (SELECT u.agency_id FROM units u JOIN unit_collection_memberships m ON m.unit_id = u.id
		WHERE NOT u.deleted AND m.collection_id = ANY($%d::uuid[]))`)
	if len(parts) == 0 {
		return "FALSE", args
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// InvalidAgency maps a foreign-key violation, which means a referenced Agency does not exist,
// to authz.ErrInvalidInput and passes every other error through.
func InvalidAgency(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23503" {
		return authz.ErrInvalidInput
	}
	return err
}
