package ingeststore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// selectWorkflowsSQL selects, with a row lock, the Workflows that process a Transmission on the
// Network $1 whose matched Units are $2: enabled, undeleted, active, assigned to the Network's
// Agency and, when narrowed, referring to the Network or to one of the Units. The second column
// is the active version, the fourth the narrowing kind (” without narrowing).
const selectWorkflowsSQL = `SELECT w.id,w.active_version_id::text,w.revision,COALESCE(w.narrowing_kind,'')
	FROM processing_workflows w
	WHERE w.enabled=true AND w.deleted=false AND w.active_version_id IS NOT NULL AND
		EXISTS (
			SELECT 1 FROM processing_workflow_agencies a
			JOIN networks n ON n.agency_id=a.agency_id
			WHERE a.workflow_id=w.id AND n.id=$1
		) AND (
		w.narrowing_kind IS NULL OR
		(w.narrowing_kind='network' AND EXISTS (
			SELECT 1 FROM processing_workflow_scope_references r
			WHERE r.workflow_id=w.id AND r.network_id=$1
		)) OR
		(w.narrowing_kind='unit' AND EXISTS (
			SELECT 1 FROM processing_workflow_scope_references r
			WHERE r.workflow_id=w.id AND r.unit_id=ANY($2::uuid[])
		))
	)
	ORDER BY w.id FOR SHARE OF w`

// selectWorkflows runs selectWorkflowsSQL for a Transmission's Network and matched Units.
func selectWorkflows(ctx context.Context, executor postgres.Executor, network uuid.UUID,
	unitIDs []uuid.UUID) ([]selectedWorkflow, error) {
	rows, err := executor.QueryContext(ctx, selectWorkflowsSQL, network, unitIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workflows []selectedWorkflow
	for rows.Next() {
		var workflow selectedWorkflow
		var version string
		if err := rows.Scan(&workflow.id, &version, &workflow.revision, &workflow.narrowing); err != nil {
			return nil, err
		}
		workflow.version, err = uuid.Parse(version)
		if err != nil || workflow.version == uuid.Nil || workflow.revision <= 0 {
			return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid active Workflow reference"))
		}
		workflows = append(workflows, workflow)
	}
	return workflows, rows.Err()
}
