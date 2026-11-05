package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/commands"
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
		field.UUID("organization_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("settings", units.Settings{}),
	}
}
func (Unit) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("collections", UnitCollection.Type).StorageKey(edge.Table("unit_collection_memberships"), edge.Columns("unit_id", "collection_id")),
		edge.To("address_rules", AddressRule.Type),
		edge.From("organization", Organization.Type).Ref("units").Field("organization_id").Unique(),
	}
}
func (Unit) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id")}
}

type Organization struct{ ent.Schema }

func (Organization) Policy() ent.Policy { return guard.Policy("Organization") }
func (Organization) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organizations"}}
}
func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.JSON("defaults", units.Defaults{}),
		field.Bool("deleted").Default(false),
	}
}
func (Organization) Edges() []ent.Edge {
	return []ent.Edge{edge.To("units", Unit.Type), edge.To("templates", UnitTemplate.Type)}
}

type UnitTemplate struct{ ent.Schema }

func (UnitTemplate) Policy() ent.Policy { return guard.Policy("UnitTemplate") }
func (UnitTemplate) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (UnitTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "unit_templates"}}
}
func (UnitTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.UUID("organization_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("settings", units.Settings{}),
		field.Bool("deleted").Default(false),
	}
}
func (UnitTemplate) Edges() []ent.Edge {
	return []ent.Edge{edge.From("organization", Organization.Type).Ref("templates").Field("organization_id").Unique()}
}
func (UnitTemplate) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id")}
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
		field.Enum("reason").Values("rule-change", "unit-deleted", "suppression-change"),
		field.Time("recorded_at"),
		field.Enum("state").Values("queued", "running", "completed", "failed", "superseded").Default("queued"),
		field.Time("high_water_accepted_at").Optional().Nillable(),
		field.UUID("high_water_transmission_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("cursor_accepted_at").Optional().Nillable(),
		field.UUID("cursor_transmission_id", uuid.UUID{}).Optional().Nillable(),
		field.Int("attempts").Default(0).NonNegative(),
		field.Time("lease_until").Optional().Nillable(),
		field.Int64("fencing_token").Default(0).NonNegative(),
		field.Int64("access_revision").Default(1).Positive(),
		field.String("failure_code").Optional().NotEmpty(),
	}
}

type TargetSuppression struct{ ent.Schema }

func (TargetSuppression) Policy() ent.Policy { return guard.Policy("TargetSuppression") }
func (TargetSuppression) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "target_suppressions"}}
}
func (TargetSuppression) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.UUID("transmission_id", uuid.UUID{}),
		field.Int("target_position").NonNegative(),
		field.Int64("revision").Default(1).Positive(),
		field.Time("created_at"),
		field.UUID("created_by", uuid.UUID{}),
		field.Time("removed_at").Optional().Nillable(),
		field.UUID("removed_by", uuid.UUID{}).Optional().Nillable(),
	}
}
func (TargetSuppression) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("transmission_id", "target_position").Unique().
			Annotations(entsql.IndexWhere("removed_at IS NULL")),
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
		field.JSON("result_items", []commands.ResultItem{}).Default([]commands.ResultItem{}),
		field.Time("recorded_at"),
	}
}
func (CommandReceipt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("actor_id", "command_id").Unique()}
}
