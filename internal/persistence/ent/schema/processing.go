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

type ProcessingControl struct{ ent.Schema }

type ProcessingInstance struct{ ent.Schema }

func (ProcessingInstance) Policy() ent.Policy { return guard.Policy("ProcessingInstance") }
func (ProcessingInstance) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_instances"}}
}
func (ProcessingInstance) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Positive(),
	}
}

type ProcessingInstanceVersion struct{ ent.Schema }

func (ProcessingInstanceVersion) Policy() ent.Policy {
	return guard.Policy("ProcessingInstanceVersion")
}
func (ProcessingInstanceVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_instance_versions"}}
}
func (ProcessingInstanceVersion) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("instance_id", uuid.UUID{}).Immutable(),
		field.Int64("revision").Positive().Immutable(),
		field.JSON("configuration", processing.InstanceConfiguration{}).Immutable().Sensitive(),
	}
}
func (ProcessingInstanceVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("instance_id", "revision").Unique()}
}

func (ProcessingControl) Policy() ent.Policy { return guard.Policy("ProcessingControl") }
func (ProcessingControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_control"}}
}
func (ProcessingControl) Fields() []ent.Field {
	return []ent.Field{field.Int("id"), field.Int64("revision").Default(1).Positive()}
}

type Workflow struct{ ent.Schema }

func (Workflow) Policy() ent.Policy { return guard.Policy("Workflow") }
func (Workflow) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Workflow) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_workflows"}}
}
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
func (Workflow) Edges() []ent.Edge {
	return []ent.Edge{edge.To("versions", WorkflowVersion.Type), edge.To("scope_references", WorkflowScopeReference.Type)}
}

type WorkflowScopeReference struct{ ent.Schema }

func (WorkflowScopeReference) Policy() ent.Policy { return guard.Policy("WorkflowScopeReference") }
func (WorkflowScopeReference) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_workflow_scope_references"}}
}
func (WorkflowScopeReference) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("workflow_id", uuid.UUID{}),
		field.UUID("network_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
	}
}
func (WorkflowScopeReference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workflow", Workflow.Type).Ref("scope_references").Field("workflow_id").Unique().Required(),
		edge.To("unit", Unit.Type).Field("unit_id").Unique(),
	}
}
func (WorkflowScopeReference) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workflow_id", "network_id").Unique(), index.Fields("workflow_id", "unit_id").Unique()}
}

type WorkflowVersion struct{ ent.Schema }

func (WorkflowVersion) Policy() ent.Policy { return guard.Policy("WorkflowVersion") }
func (WorkflowVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_workflow_versions"}}
}
func (WorkflowVersion) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("workflow_id", uuid.UUID{}).Immutable(),
		field.JSON("definition", processing.Definition{}).Immutable().Sensitive(),
		field.Time("created_at").Immutable(),
		field.UUID("created_by", uuid.UUID{}).Immutable(),
	}
}
func (WorkflowVersion) Edges() []ent.Edge {
	return []ent.Edge{edge.From("workflow", Workflow.Type).Ref("versions").
		Field("workflow_id").Unique().Required().Immutable()}
}
func (WorkflowVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workflow_id", "created_at", "id"), index.Fields("workflow_id", "id").Unique()}
}

type ProcessingCandidate struct{ ent.Schema }

func (ProcessingCandidate) Policy() ent.Policy { return guard.Policy("ProcessingCandidate") }
func (ProcessingCandidate) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_candidates"}}
}
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
		field.Time("barrier_applied_at").Optional().Nillable(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}
func (ProcessingCandidate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("transmission_id", "workflow_id", "definition_version_id", "source_revision", "context_revision").Unique(),
		index.Fields("state", "due_at", "id"),
		index.Fields("transmission_id", "barrier_applied_at"),
	}
}

type ProcessingResult struct{ ent.Schema }

func (ProcessingResult) Policy() ent.Policy { return guard.Policy("ProcessingResult") }
func (ProcessingResult) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "processing_results"}}
}
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
func (ProcessingResult) Indexes() []ent.Index {
	return []ent.Index{index.Fields("candidate_id", "revision").Unique()}
}
