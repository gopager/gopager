package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
)

// CorrectionDirtyTransmission defines the correction_dirty_transmissions table: Transmissions a
// correction scope must reprocess explicitly, outside its cursor scan, ordered by acceptance.
type CorrectionDirtyTransmission struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the CorrectionDirtyTransmission schema.
func (CorrectionDirtyTransmission) Policy() ent.Policy {
	return guard.Policy("CorrectionDirtyTransmission")
}

// Annotations returns the table name and code-generation annotations of the CorrectionDirtyTransmission schema.
func (CorrectionDirtyTransmission) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "correction_dirty_transmissions"}}
}

// Fields returns the columns of the CorrectionDirtyTransmission schema.
func (CorrectionDirtyTransmission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("scope_id", uuid.UUID{}),
		field.UUID("transmission_id", uuid.UUID{}),
		field.Time("accepted_at"),
		field.Time("recorded_at"),
	}
}

// Indexes returns the indexes and unique constraints of the CorrectionDirtyTransmission schema.
func (CorrectionDirtyTransmission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_id", "accepted_at", "transmission_id").Unique(),
		index.Fields("transmission_id"),
	}
}
