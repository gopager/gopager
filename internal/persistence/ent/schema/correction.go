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

type CorrectionDirtyTransmission struct{ ent.Schema }

func (CorrectionDirtyTransmission) Policy() ent.Policy {
	return guard.Policy("CorrectionDirtyTransmission")
}
func (CorrectionDirtyTransmission) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "correction_dirty_transmissions"}}
}
func (CorrectionDirtyTransmission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("scope_id", uuid.UUID{}),
		field.UUID("transmission_id", uuid.UUID{}),
		field.Time("accepted_at"),
		field.Time("recorded_at"),
	}
}
func (CorrectionDirtyTransmission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_id", "accepted_at", "transmission_id").Unique(),
		index.Fields("transmission_id"),
	}
}
