// Package schema declares the Ent schemas from which the generated persistence client in the
// parent ent package is built (go generate in ../generate.go). Each schema type maps one
// PostgreSQL table, named by its entsql annotation, across the identity, Unit/Assignment,
// ingest and Processing domains. Every entity installs guard.Policy, so it can only be read
// or written through a client bound with guard.Bind inside an active borrowed transaction.
// Database DDL is not derived from these schemas at runtime; it is applied by the embedded
// SQL migrations, which must be kept in step with them.
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

// Unit defines the units table: an alerting Unit with name, settings and optional
// Organization; soft-deleted via the deleted flag.
type Agency struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Agency schema.
func (Agency) Policy() ent.Policy { return guard.Policy("Agency") }

// Mixin returns the mixins shared into the Agency schema.
func (Agency) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Agency schema.
func (Agency) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "agencies"}}
}

// Fields returns the columns of the Agency schema.
func (Agency) Fields() []ent.Field {
	return []ent.Field{field.String("name").NotEmpty().MaxRuneLen(256)}
}

// Edges returns the relations of the Agency schema: the Units and Radio networks it owns.
func (Agency) Edges() []ent.Edge {
	return []ent.Edge{edge.To("units", Unit.Type), edge.To("networks", Network.Type)}
}

// Unit defines the units table: a logical recipient that Addresses resolve to, owned by one Agency.
type Unit struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Unit schema.
func (Unit) Policy() ent.Policy { return guard.Policy("Unit") }

// Mixin returns the mixins shared into the Unit schema.
func (Unit) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Unit schema.
func (Unit) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "units"}}
}

// Fields returns the columns of the Unit schema.
func (Unit) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.UUID("agency_id", uuid.UUID{}),
		field.Bool("deleted").Default(false),
		field.UUID("organization_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("settings", units.Settings{}),
	}
}

// Edges returns the relations of the Unit schema.
func (Unit) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("collections", UnitCollection.Type).StorageKey(edge.Table("unit_collection_memberships"), edge.Columns("unit_id", "collection_id")),
		edge.To("address_rules", AddressRule.Type),
		edge.From("organization", Organization.Type).Ref("units").Field("organization_id").Unique(),
		edge.From("agency", Agency.Type).Ref("units").Field("agency_id").Unique().Required(),
	}
}

// Indexes returns the indexes and unique constraints of the Unit schema.
func (Unit) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id"), index.Fields("agency_id")}
}

// Organization defines the organizations table: a grouping of Units and Unit templates that
// supplies their defaults; soft-deleted via the deleted flag.
type Organization struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Organization schema.
func (Organization) Policy() ent.Policy { return guard.Policy("Organization") }

// Mixin returns the mixins shared into the Organization schema.
func (Organization) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Organization schema.
func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organizations"}}
}

// Fields returns the columns of the Organization schema.
func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.JSON("defaults", units.Defaults{}),
		field.Bool("deleted").Default(false),
	}
}

// Edges returns the relations of the Organization schema.
func (Organization) Edges() []ent.Edge {
	return []ent.Edge{edge.To("units", Unit.Type), edge.To("templates", UnitTemplate.Type)}
}

// UnitTemplate defines the unit_templates table: reusable Unit settings, optionally owned by
// an Organization; soft-deleted via the deleted flag.
type UnitTemplate struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the UnitTemplate schema.
func (UnitTemplate) Policy() ent.Policy { return guard.Policy("UnitTemplate") }

// Mixin returns the mixins shared into the UnitTemplate schema.
func (UnitTemplate) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the UnitTemplate schema.
func (UnitTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "unit_templates"}}
}

// Fields returns the columns of the UnitTemplate schema.
func (UnitTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.UUID("organization_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("settings", units.Settings{}),
		field.Bool("deleted").Default(false),
	}
}

// Edges returns the relations of the UnitTemplate schema.
func (UnitTemplate) Edges() []ent.Edge {
	return []ent.Edge{edge.From("organization", Organization.Type).Ref("templates").Field("organization_id").Unique()}
}

// Indexes returns the indexes and unique constraints of the UnitTemplate schema.
func (UnitTemplate) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id")}
}

// UnitCollection defines the unit_collections table: a named set of Units (membership in
// unit_collection_memberships) used as an authorization scope.
type UnitCollection struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the UnitCollection schema.
func (UnitCollection) Policy() ent.Policy { return guard.Policy("UnitCollection") }

// Mixin returns the mixins shared into the UnitCollection schema.
func (UnitCollection) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the UnitCollection schema.
func (UnitCollection) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "unit_collections"}}
}

// Fields returns the columns of the UnitCollection schema.
func (UnitCollection) Fields() []ent.Field {
	return []ent.Field{field.String("name").NotEmpty().MaxRuneLen(256)}
}

// Edges returns the relations of the UnitCollection schema.
func (UnitCollection) Edges() []ent.Edge {
	return []ent.Edge{edge.From("units", Unit.Type).Ref("collections")}
}

// AddressRule defines the address_rules table: a rule assigning matching targets of one
// Network and Protocol to a Unit, optionally limited to a validity window.
type AddressRule struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the AddressRule schema.
func (AddressRule) Policy() ent.Policy { return guard.Policy("AddressRule") }

// Mixin returns the mixins shared into the AddressRule schema.
func (AddressRule) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the AddressRule schema.
func (AddressRule) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "address_rules"}}
}

// Fields returns the columns of the AddressRule schema.
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

// Edges returns the relations of the AddressRule schema.
func (AddressRule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("unit", Unit.Type).Ref("address_rules").Field("unit_id").Unique().Required(),
		edge.From("network", Network.Type).Ref("address_rules").Field("network_id").Unique().Required(),
	}
}

// Indexes returns the indexes and unique constraints of the AddressRule schema.
func (AddressRule) Indexes() []ent.Index {
	return []ent.Index{index.Fields("network_id", "protocol", "enabled"), index.Fields("unit_id")}
}

// AssignmentControl defines the single-row assignment_control table holding the current
// Assignment revision, locked to serialize Assignment changes.
type AssignmentControl struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the AssignmentControl schema.
func (AssignmentControl) Policy() ent.Policy { return guard.Policy("AssignmentControl") }

// Annotations returns the table name and code-generation annotations of the AssignmentControl schema.
func (AssignmentControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "assignment_control"}}
}

// Fields returns the columns of the AssignmentControl schema.
func (AssignmentControl) Fields() []ent.Field {
	return []ent.Field{field.Int("id"), field.Int64("revision").Default(1).Positive()}
}

// PendingAssignmentScope defines the assignment_pending_scopes table: a durable, leased
// correction run that re-resolves stored targets for given Networks/Unit after an Assignment
// change, scanning up to a high-water mark with a fenced cursor.
type PendingAssignmentScope struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the PendingAssignmentScope schema.
func (PendingAssignmentScope) Policy() ent.Policy { return guard.Policy("PendingAssignmentScope") }

// Annotations returns the table name and code-generation annotations of the PendingAssignmentScope schema.
func (PendingAssignmentScope) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "assignment_pending_scopes"}}
}

// Fields returns the columns of the PendingAssignmentScope schema.
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

// TargetSuppression defines the target_suppressions table: an operator suppression of one
// Transmission target; at most one unlifted suppression exists per target.
type TargetSuppression struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the TargetSuppression schema.
func (TargetSuppression) Policy() ent.Policy { return guard.Policy("TargetSuppression") }

// Annotations returns the table name and code-generation annotations of the TargetSuppression schema.
func (TargetSuppression) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "target_suppressions"}}
}

// Fields returns the columns of the TargetSuppression schema.
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

// Indexes returns the indexes and unique constraints of the TargetSuppression schema.
func (TargetSuppression) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("transmission_id", "target_position").Unique().
			Annotations(entsql.IndexWhere("removed_at IS NULL")),
	}
}

// CommandReceipt defines the command_receipts table: the idempotency receipt of an
// administrative command, unique per actor and command ID.
type CommandReceipt struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the CommandReceipt schema.
func (CommandReceipt) Policy() ent.Policy { return guard.Policy("CommandReceipt") }

// Annotations returns the table name and code-generation annotations of the CommandReceipt schema.
func (CommandReceipt) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "command_receipts"}}
}

// Fields returns the columns of the CommandReceipt schema.
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

// Indexes returns the indexes and unique constraints of the CommandReceipt schema.
func (CommandReceipt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("actor_id", "command_id").Unique()}
}
