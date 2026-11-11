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
// waits, so no new sole assignment can appear before the deletion commits.
func (s *Store) lastAgencyOfDefinition(ctx context.Context, tx platform.TransactionContext, agencyID uuid.UUID) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
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
