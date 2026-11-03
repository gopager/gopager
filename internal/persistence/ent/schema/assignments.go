package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
)

type Unit struct{ ent.Schema }

func (Unit) Policy() ent.Policy { return guard.Policy("Unit") }
func (Unit) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Unit) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "units"}}
}
func (Unit) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.Bool("deleted").Default(false),
	}
}
func (Unit) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("collections", UnitCollection.Type).StorageKey(edge.Table("unit_collection_memberships"), edge.Columns("unit_id", "collection_id")),
		edge.To("address_rules", AddressRule.Type),
	}
}

type UnitCollection struct{ ent.Schema }

func (UnitCollection) Policy() ent.Policy { return guard.Policy("UnitCollection") }
func (UnitCollection) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (UnitCollection) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "unit_collections"}}
}
func (UnitCollection) Fields() []ent.Field {
	return []ent.Field{field.String("name").NotEmpty().MaxRuneLen(256)}
}
func (UnitCollection) Edges() []ent.Edge {
	return []ent.Edge{edge.From("units", Unit.Type).Ref("collections")}
}

type AddressRule struct{ ent.Schema }

func (AddressRule) Policy() ent.Policy { return guard.Policy("AddressRule") }
func (AddressRule) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (AddressRule) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "address_rules"}}
}
func (AddressRule) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("network_id", uuid.UUID{}),
		field.UUID("unit_id", uuid.UUID{}),
		field.Enum("protocol").Values("pocsag", "flex", "zvei", "eas"),
		field.JSON("selector", assignments.Selector{}),
		field.Time("valid_from").Optional().Nillable(),
		field.Time("valid_until").Optional().Nillable(),
		field.Bool("enabled").Default(true),
	}
}
func (AddressRule) Edges() []ent.Edge {
	return []ent.Edge{edge.From("unit", Unit.Type).Ref("address_rules").Field("unit_id").Unique().Required()}
}
func (AddressRule) Indexes() []ent.Index {
	return []ent.Index{index.Fields("network_id", "protocol", "enabled"), index.Fields("unit_id")}
}

type AssignmentControl struct{ ent.Schema }

func (AssignmentControl) Policy() ent.Policy { return guard.Policy("AssignmentControl") }
func (AssignmentControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "assignment_control"}}
}
func (AssignmentControl) Fields() []ent.Field {
	return []ent.Field{field.Int("id"), field.Int64("revision").Default(1).Positive()}
}

type PendingAssignmentScope struct{ ent.Schema }

func (PendingAssignmentScope) Policy() ent.Policy { return guard.Policy("PendingAssignmentScope") }
func (PendingAssignmentScope) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "assignment_pending_scopes"}}
}
func (PendingAssignmentScope) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Int64("assignment_revision").Positive(),
		field.JSON("network_ids", []uuid.UUID{}),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("reason").Values("rule-change", "unit-deleted"),
		field.Time("recorded_at"),
	}
}

type CommandReceipt struct{ ent.Schema }

func (CommandReceipt) Policy() ent.Policy { return guard.Policy("CommandReceipt") }
func (CommandReceipt) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "command_receipts"}}
}
func (CommandReceipt) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("actor_id", uuid.UUID{}),
		field.UUID("command_id", uuid.UUID{}),
		field.String("operation").NotEmpty(),
		field.Bytes("fingerprint").Sensitive(),
		field.UUID("result_id", uuid.UUID{}),
		field.Int64("result_revision").Positive(),
		field.Time("recorded_at"),
	}
}
func (CommandReceipt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("actor_id", "command_id").Unique()}
}
