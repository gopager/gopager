package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/processing"
)

// ProcessingControl defines the single-row processing_control table holding the processing
// configuration revision, locked to serialize configuration changes.
type ProcessingControl struct{ ent.Schema }

// ProcessingInstance defines the processing_instances table: a configured plugin instance and
// its current revision.
type ProcessingInstance struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the ProcessingInstance schema.
func (ProcessingInstance) Policy() ent.Policy { return guard.Policy("ProcessingInstance") }

// Annotations returns the table name and code-generation annotations of the ProcessingInstance schema.
func (ProcessingInstance) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_instances"}}
}

// Fields returns the columns of the ProcessingInstance schema.
func (ProcessingInstance) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Positive(),
	}
}

// ProcessingInstanceVersion defines the processing_instance_versions table: an immutable,
// secret-bearing configuration snapshot of a plugin instance, unique per instance and
// revision.
type ProcessingInstanceVersion struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the ProcessingInstanceVersion schema.
func (ProcessingInstanceVersion) Policy() ent.Policy {
	return guard.Policy("ProcessingInstanceVersion")
}

// Annotations returns the table name and code-generation annotations of the ProcessingInstanceVersion schema.
func (ProcessingInstanceVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_instance_versions"}}
}

// Fields returns the columns of the ProcessingInstanceVersion schema.
func (ProcessingInstanceVersion) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("instance_id", uuid.UUID{}).Immutable(),
		field.Int64("revision").Positive().Immutable(),
		field.JSON("configuration", processing.InstanceConfiguration{}).Immutable().Sensitive(),
	}
}

// Indexes returns the indexes and unique constraints of the ProcessingInstanceVersion schema.
func (ProcessingInstanceVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("instance_id", "revision").Unique()}
}

// Policy returns the privacy policy that guards queries and mutations of the ProcessingControl schema.
func (ProcessingControl) Policy() ent.Policy { return guard.Policy("ProcessingControl") }

// Annotations returns the table name and code-generation annotations of the ProcessingControl schema.
func (ProcessingControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_control"}}
}

// Fields returns the columns of the ProcessingControl schema.
func (ProcessingControl) Fields() []ent.Field {
	return []ent.Field{field.Int("id"), field.Int64("revision").Default(1).Positive()}
}

// Workflow defines the processing_workflows table: a Workflow with its scope kind, enablement
// and active version; soft-deleted via the deleted flag.
type Workflow struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Workflow schema.
func (Workflow) Policy() ent.Policy { return guard.Policy("Workflow") }

// Mixin returns the mixins shared into the Workflow schema.
func (Workflow) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Workflow schema.
func (Workflow) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_workflows"}}
}

// Fields returns the columns of the Workflow schema.
func (Workflow) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Enum("scope_kind").Values("all", "network", "unit"),
		field.Bool("enabled").Default(false),
		field.Bool("deleted").Default(false),
		field.UUID("active_version_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

// Edges returns the relations of the Workflow schema.
func (Workflow) Edges() []ent.Edge {
	return []ent.Edge{edge.To("versions", WorkflowVersion.Type), edge.To("scope_references", WorkflowScopeReference.Type)}
}

// WorkflowScopeReference defines the processing_workflow_scope_references table: a Network or
// Unit a Workflow's scope refers to, unique per Workflow.
type WorkflowScopeReference struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the WorkflowScopeReference schema.
func (WorkflowScopeReference) Policy() ent.Policy { return guard.Policy("WorkflowScopeReference") }

// Annotations returns the table name and code-generation annotations of the WorkflowScopeReference schema.
func (WorkflowScopeReference) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_workflow_scope_references"}}
}

// Fields returns the columns of the WorkflowScopeReference schema.
func (WorkflowScopeReference) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("workflow_id", uuid.UUID{}),
		field.UUID("network_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
	}
}

// Edges returns the relations of the WorkflowScopeReference schema.
func (WorkflowScopeReference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workflow", Workflow.Type).Ref("scope_references").Field("workflow_id").Unique().Required(),
		edge.To("unit", Unit.Type).Field("unit_id").Unique(),
		edge.To("network", Network.Type).Field("network_id").Unique(),
	}
}

// Indexes returns the indexes and unique constraints of the WorkflowScopeReference schema.
func (WorkflowScopeReference) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workflow_id", "network_id").Unique(), index.Fields("workflow_id", "unit_id").Unique()}
}

// WorkflowVersion defines the processing_workflow_versions table: an immutable saved Workflow
// definition.
type WorkflowVersion struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the WorkflowVersion schema.
func (WorkflowVersion) Policy() ent.Policy { return guard.Policy("WorkflowVersion") }

// Annotations returns the table name and code-generation annotations of the WorkflowVersion schema.
func (WorkflowVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_workflow_versions"}}
}

// Fields returns the columns of the WorkflowVersion schema.
func (WorkflowVersion) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("workflow_id", uuid.UUID{}).Immutable(),
		field.JSON("definition", processing.Definition{}).Immutable().Sensitive(),
		field.Time("created_at").Immutable(),
		field.UUID("created_by", uuid.UUID{}).Immutable(),
	}
}

// Edges returns the relations of the WorkflowVersion schema.
func (WorkflowVersion) Edges() []ent.Edge {
	return []ent.Edge{edge.From("workflow", Workflow.Type).Ref("versions").
		Field("workflow_id").Unique().Required().Immutable()}
}

// Indexes returns the indexes and unique constraints of the WorkflowVersion schema.
func (WorkflowVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workflow_id", "created_at", "id"), index.Fields("workflow_id", "id").Unique()}
}

// ProcessingCandidate defines the processing_candidates table: leased, fenced evaluation work
// for one Workflow version against one Transmission, unique per Transmission, Workflow,
// version and source/context revision.
type ProcessingCandidate struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the ProcessingCandidate schema.
func (ProcessingCandidate) Policy() ent.Policy { return guard.Policy("ProcessingCandidate") }

// Annotations returns the table name and code-generation annotations of the ProcessingCandidate schema.
func (ProcessingCandidate) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_candidates"}}
}

// Fields returns the columns of the ProcessingCandidate schema.
func (ProcessingCandidate) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Int64("revision").Default(1).Positive(),
		field.UUID("transmission_id", uuid.UUID{}),
		field.UUID("workflow_id", uuid.UUID{}),
		field.UUID("definition_version_id", uuid.UUID{}),
		field.Int64("workflow_revision").Positive(),
		field.Int64("configuration_revision").Positive(),
		field.Int64("source_revision").Positive(),
		field.Int64("context_revision").Positive(),
		field.Enum("state").Values("pending", "running", "completed", "failed", "superseded").Default("pending"),
		field.Bool("selected").Optional().Nillable(),
		field.Int64("attempts").Default(0).NonNegative(),
		field.Int("automatic_attempts").Default(0).Range(0, processing.MaxAutomaticAttempts),
		field.Bool("manual_retry_pending").Default(false),
		field.Int64("fencing_token").Default(0).NonNegative(),
		field.UUID("worker_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("lease_until").Optional().Nillable(),
		field.Time("due_at"),
		field.String("failure_code").Optional().Nillable(),
		field.String("failure_step").Optional().Nillable(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

// Indexes returns the indexes and unique constraints of the ProcessingCandidate schema.
func (ProcessingCandidate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("transmission_id", "workflow_id", "definition_version_id", "source_revision", "context_revision").Unique(),
		index.Fields("state", "due_at", "id"),
	}
}

// ProcessingResult defines the processing_results table: the private evaluation result of a
// candidate, unique per candidate and revision, with its publication time and Message once published.
type ProcessingResult struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the ProcessingResult schema.
func (ProcessingResult) Policy() ent.Policy { return guard.Policy("ProcessingResult") }

// Annotations returns the table name and code-generation annotations of the ProcessingResult schema.
func (ProcessingResult) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_results"}}
}

// Fields returns the columns of the ProcessingResult schema.
func (ProcessingResult) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Int64("revision").Default(1).Positive(),
		field.UUID("candidate_id", uuid.UUID{}),
		field.Int64("producing_fence").Positive(),
		field.JSON("evaluation", processing.Evaluation{}).Sensitive(),
		field.Time("completed_at"),
		field.Time("published_at").Optional().Nillable(),
		field.UUID("message_id", uuid.UUID{}).Optional().Nillable(),
	}
}

// Indexes returns the indexes and unique constraints of the ProcessingResult schema.
func (ProcessingResult) Indexes() []ent.Index {
	return []ent.Index{index.Fields("candidate_id", "revision").Unique()}
}
