package attributestore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// entryHolders are the tables whose rows store Unit-attribute entries, with the JSON column of
// the setting document that holds them.
var entryHolders = []struct{ table, column string }{
	{"units", "settings"},
	{"unit_templates", "settings"},
	{"organizations", "defaults"},
}

// entriesInUse reports whether an undeleted Unit, template or Organization stores an entry of
// the schema.
func entriesInUse(ctx context.Context, executor postgres.Executor, key string) (bool, error) {
	for _, holder := range entryHolders {
		var found bool
		if err := executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM `+holder.table+
			` WHERE NOT deleted AND jsonb_exists(`+holder.column+`->'attributes', $1))`, key).Scan(&found); err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// stripDeletedEntries removes the schema's entries from deleted Units, templates and
// Organizations, so that a later schema with the same key never meets them.
func stripDeletedEntries(ctx context.Context, executor postgres.Executor, key string) error {
	for _, holder := range entryHolders {
		if _, err := executor.ExecContext(ctx, `UPDATE `+holder.table+` SET `+holder.column+` = `+holder.column+
			` #- ARRAY['attributes', $1::text] WHERE deleted AND jsonb_exists(`+holder.column+`->'attributes', $1)`, key); err != nil {
			return err
		}
	}
	return nil
}

// removedAgencies returns the Agencies assigned to the stored schema that the update drops.
func removedAgencies(ctx context.Context, executor postgres.Executor, key string, keep []uuid.UUID) ([]string, error) {
	kept := make(map[uuid.UUID]bool, len(keep))
	for _, agency := range keep {
		kept[agency] = true
	}
	rows, err := executor.QueryContext(ctx, `SELECT agency_id FROM attribute_schema_agencies WHERE schema_key = $1`, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var removed []string
	for rows.Next() {
		var agency uuid.UUID
		if err := rows.Scan(&agency); err != nil {
			return nil, err
		}
		if !kept[agency] {
			removed = append(removed, agency.String())
		}
	}
	return removed, rows.Err()
}

// requireNoEntriesOf fails with authz.ErrRevisionConflict while an undeleted Unit of one of the
// Agencies stores an entry of the schema.
func requireNoEntriesOf(ctx context.Context, executor postgres.Executor, key string, agencies []string) error {
	if len(agencies) == 0 {
		return nil
	}
	var found bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM units WHERE NOT deleted
		AND agency_id = ANY($2::uuid[]) AND jsonb_exists(settings->'attributes', $1))`, key, agencies).Scan(&found); err != nil {
		return err
	}
	if found {
		return authz.ErrRevisionConflict
	}
	return nil
}
