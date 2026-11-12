package unitstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// lastAgencyOfDefinition reports whether the Agency is the only assigned Agency of an undeleted
// Workflow, a Layout or an Attribute schema. Deleting it would leave such a definition assigned
// to no Agency, which nothing could then reach or manage. Soft-deleted Workflows do not count;
// their assignment rows cascade with the Agency. The caller holds the Agency's row lock; an
// assignment insert takes a key-share lock on the Agency through its foreign key and therefore
// waits, so no new sole assignment can appear before the deletion commits. The assignment rows of
// every definition that includes the Agency are locked first: a concurrent deletion of another
// Agency of the same definition then waits and re-checks the committed state, instead of both
// deletions seeing the other's row and orphaning the definition.
func (s *Store) lastAgencyOfDefinition(ctx context.Context, tx platform.TransactionContext, agencyID uuid.UUID) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	for _, query := range lockDefinitionAssignmentsSQL {
		if _, err := executor.ExecContext(ctx, query, agencyID); err != nil {
			return false, err
		}
	}
	var last bool
	err = executor.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM processing_workflow_agencies a JOIN processing_workflows w ON w.id = a.workflow_id
			WHERE a.agency_id = $1 AND NOT w.deleted
			AND NOT EXISTS (SELECT 1 FROM processing_workflow_agencies o WHERE o.workflow_id = a.workflow_id AND o.agency_id <> $1))
		OR EXISTS (SELECT 1 FROM layout_definition_agencies a WHERE a.agency_id = $1
			AND NOT EXISTS (SELECT 1 FROM layout_definition_agencies o WHERE o.layout_id = a.layout_id AND o.agency_id <> $1))
		OR EXISTS (SELECT 1 FROM attribute_schema_agencies a WHERE a.agency_id = $1
			AND NOT EXISTS (SELECT 1 FROM attribute_schema_agencies o WHERE o.schema_key = a.schema_key AND o.agency_id <> $1))`,
		agencyID).Scan(&last)
	return last, err
}

// lockDefinitionAssignmentsSQL locks, one statement per assignment table, every assignment row of
// the definitions that include the Agency. The fixed ordering keeps concurrent deletions from
// deadlocking. Updates that rewrite a definition's assignments delete and insert rows, so they
// conflict with it as well.
var lockDefinitionAssignmentsSQL = []string{
	`SELECT 1 FROM processing_workflow_agencies o WHERE o.workflow_id IN
		(SELECT workflow_id FROM processing_workflow_agencies WHERE agency_id = $1)
		ORDER BY o.workflow_id, o.agency_id FOR UPDATE`,
	`SELECT 1 FROM layout_definition_agencies o WHERE o.layout_id IN
		(SELECT layout_id FROM layout_definition_agencies WHERE agency_id = $1)
		ORDER BY o.layout_id, o.agency_id FOR UPDATE`,
	`SELECT 1 FROM attribute_schema_agencies o WHERE o.schema_key IN
		(SELECT schema_key FROM attribute_schema_agencies WHERE agency_id = $1)
		ORDER BY o.schema_key, o.agency_id FOR UPDATE`,
}
