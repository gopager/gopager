package unitstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// agencyAssignments maps an Agency to the Attribute schema keys assigned to it.
type agencyAssignments map[uuid.UUID]map[string]bool

// assigned returns the assignment test of the Agency for units.ResolveSettings.
func (a agencyAssignments) assigned(agency uuid.UUID) func(string) bool {
	keys := a[agency]
	return func(schemaKey string) bool { return keys[schemaKey] }
}

// loadAssignments reads the schema assignments of the Agencies of rows with one query, so that
// resolving a page of Units costs no query per Unit.
func (s *Store) loadAssignments(ctx context.Context, tx platform.TransactionContext, rows []*ent.Unit) (agencyAssignments, error) {
	result := make(agencyAssignments)
	agencies := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, seen := result[row.AgencyID]; !seen {
			result[row.AgencyID] = map[string]bool{}
			agencies = append(agencies, row.AgencyID.String())
		}
	}
	if len(agencies) == 0 {
		return result, nil
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	found, err := executor.QueryContext(ctx,
		`SELECT agency_id, schema_key FROM attribute_schema_agencies WHERE agency_id = ANY($1::uuid[])`, agencies)
	if err != nil {
		return nil, err
	}
	defer func() { _ = found.Close() }()
	for found.Next() {
		var agency uuid.UUID
		var key string
		if err := found.Scan(&agency, &key); err != nil {
			return nil, err
		}
		result[agency][key] = true
	}
	return result, found.Err()
}

// unitDTOs projects rows with their inherited attributes resolved against the schemas assigned
// to each Unit's Agency.
func (s *Store) unitDTOs(ctx context.Context, tx platform.TransactionContext, rows []*ent.Unit) ([]units.Unit, error) {
	assignments, err := s.loadAssignments(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	result := make([]units.Unit, len(rows))
	for i, row := range rows {
		result[i] = unitDTO(row, assignments.assigned(row.AgencyID))
	}
	return result, nil
}
