package assignmentstore

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// openScopeSQL is inScopeSQL with its positional parameters replaced by the columns of the
// alias s (assignment_pending_scopes), so one statement can test a target against every open
// scope. The longer placeholders come first because the replacer matches in argument order.
var openScopeSQL = strings.NewReplacer("$10", "s.target_position", "$11", "s.assignment_revision",
	"$1", "s.network_id", "$2", "s.protocol", "$3", "s.address_from", "$4", "s.address_to",
	"$5", "s.address_text", "$6", "s.valid_from", "$7", "s.valid_until", "$8", "s.unit_id",
	"$9", "s.transmission_id").Replace(inScopeSQL)

// refreshMessageUnits rebuilds a Message's cache rows from its current evidence: one row per Unit
// of an unsuppressed assigned target, carrying the Unit's Agency, and one Unit-less row per
// Network Agency of its targetless Transmissions. The rebuilt rows are current unless a target
// of the Message still lies in a queued or running scope at an older revision, in which case
// they stay correcting until the scope that owns that target has run.
func refreshMessageUnits(ctx context.Context, executor postgres.Executor, message uuid.UUID) error {
	for _, statement := range []string{
		`DELETE FROM message_units WHERE message_id=$1`,
		`INSERT INTO message_units (message_id,unit_id,agency_id,first_observed_at,age_anchor)
		SELECT t.message_id,target.unit_id,u.agency_id,MIN(t.observed_at),MIN(LEAST(t.observed_at,t.accepted_at))
		FROM transmissions t JOIN transmission_targets target ON target.transmission_id=t.id
		JOIN units u ON u.id=target.unit_id
		WHERE t.message_id=$1 AND target.resolution_status='assigned'
		AND NOT EXISTS (SELECT 1 FROM target_suppressions suppression
			WHERE suppression.transmission_id=target.transmission_id
			AND suppression.target_position=target.position AND suppression.removed_at IS NULL)
		GROUP BY t.message_id,target.unit_id,u.agency_id`,
		`INSERT INTO message_units (message_id,unit_id,agency_id,first_observed_at,age_anchor)
		SELECT t.message_id,NULL,n.agency_id,MIN(t.observed_at),MIN(LEAST(t.observed_at,t.accepted_at))
		FROM transmissions t JOIN networks n ON n.id=t.network_id
		WHERE t.message_id=$1 AND NOT EXISTS (SELECT 1 FROM transmission_targets target WHERE target.transmission_id=t.id)
		GROUP BY t.message_id,n.agency_id`,
		`UPDATE message_units SET correction_status='correcting' WHERE message_id=$1 AND EXISTS (
			SELECT 1 FROM assignment_pending_scopes s
			JOIN transmissions t ON t.message_id=$1
			JOIN transmission_targets target ON target.transmission_id=t.id
			WHERE s.state IN ('queued','running') AND ` + openScopeSQL + `)`,
	} {
		if _, err := executor.ExecContext(ctx, statement, message); err != nil {
			return err
		}
	}
	return nil
}
