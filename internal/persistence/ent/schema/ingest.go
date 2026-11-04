package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
)

type Network struct{ ent.Schema }

func (Network) Policy() ent.Policy { return guard.Policy("Network") }
func (Network) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "networks"}}
}
func (Network) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Default(1).Positive(),
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.Enum("protocol").Values("pocsag", "flex", "zvei", "eas"),
		field.Bool("deleted").Default(false),
	}
}

type Gateway struct{ ent.Schema }

func (Gateway) Policy() ent.Policy { return guard.Policy("Gateway") }
func (Gateway) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gateways"}}
}
func (Gateway) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Default(1).Positive(),
		field.Enum("state").Values("active", "degraded", "stopped", "contact-lost").Default("active"),
		field.Time("last_contact_at").Optional().Nillable(),
		field.Time("last_heartbeat_at").Optional().Nillable(),
		field.Time("last_transmission_at").Optional().Nillable(),
		field.JSON("last_report", json.RawMessage{}).Optional().Sensitive(),
	}
}

type GatewayNetwork struct{ ent.Schema }

func (GatewayNetwork) Policy() ent.Policy { return guard.Policy("GatewayNetwork") }
func (GatewayNetwork) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gateway_networks"}}
}
func (GatewayNetwork) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("gateway_id", uuid.UUID{}),
		field.String("gateway_network_identifier").NotEmpty().MaxRuneLen(128),
		field.UUID("network_id", uuid.UUID{}),
		field.Bool("default"),
	}
}
func (GatewayNetwork) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("gateway_id", "gateway_network_identifier").Unique(),
		index.Fields("gateway_id", "network_id").Unique(),
	}
}

type GatewayCredential struct{ ent.Schema }

func (GatewayCredential) Policy() ent.Policy { return guard.Policy("GatewayCredential") }
func (GatewayCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gateway_credentials"}}
}
func (GatewayCredential) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.UUID("gateway_id", uuid.UUID{}),
		field.Bytes("secret_hash").Sensitive(),
		field.Bool("pending").Default(false),
		field.Time("created_at"),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}
func (GatewayCredential) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("secret_hash").Unique(),
		index.Fields("gateway_id").Unique().Annotations(entsql.IndexWhere("revoked_at IS NULL AND pending = false")),
		index.Fields("gateway_id").Unique().Annotations(entsql.IndexWhere("revoked_at IS NULL AND pending = true")),
	}
}

type Transmission struct{ ent.Schema }

func (Transmission) Policy() ent.Policy { return guard.Policy("Transmission") }
func (Transmission) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "transmissions"}}
}
func (Transmission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.UUID("gateway_id", uuid.UUID{}),
		field.UUID("network_id", uuid.UUID{}),
		field.String("submission_id").NotEmpty(),
		field.Bytes("submission_fingerprint").Sensitive(),
		field.String("network_resolution").NotEmpty(),
		field.Enum("protocol").Values("pocsag", "flex", "zvei", "eas"),
		field.JSON("payload", json.RawMessage{}).SchemaType(map[string]string{dialect.Postgres: "json"}).Sensitive(),
		field.JSON("targets", json.RawMessage{}).SchemaType(map[string]string{dialect.Postgres: "json"}).Sensitive(),
		field.Time("observed_at"),
		field.Int("observation_offset_seconds"),
		field.Time("accepted_at"),
		field.Int64("source_revision").Positive(),
		field.Bool("incomplete").Default(false),
		field.JSON("quality_metrics", json.RawMessage{}).SchemaType(map[string]string{dialect.Postgres: "json"}).Optional().Sensitive(),
		field.UUID("message_id", uuid.UUID{}),
		field.Enum("processing_status").Values("pending", "completed", "failed").Default("pending"),
		field.Enum("publication_status").Values("pending", "ready", "failed").Default("pending"),
	}
}
func (Transmission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("gateway_id", "submission_id").Unique(),
		index.Fields("protocol", "observed_at", "id"),
		index.Fields("message_id", "observed_at", "id"),
	}
}

type Message struct{ ent.Schema }

func (Message) Policy() ent.Policy { return guard.Policy("Message") }
func (Message) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "messages"}}
}
func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Default(1).Positive(),
		field.String("identity_form").NotEmpty(),
		field.String("identity_subtype").Default(""),
		field.Bytes("identity_value").Sensitive(),
		field.Time("first_observed_at"),
		field.Time("last_observed_at"),
		field.Enum("publication_status").Values("pending", "ready", "failed").Default("pending"),
	}
}
func (Message) Indexes() []ent.Index {
	return []ent.Index{index.Fields("identity_form", "identity_subtype", "identity_value")}
}

type GroupingControl struct{ ent.Schema }

func (GroupingControl) Policy() ent.Policy { return guard.Policy("GroupingControl") }
func (GroupingControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "grouping_control"}}
}
func (GroupingControl) Fields() []ent.Field {
	return []ent.Field{field.Int("id"), field.Int64("revision").Default(1).Positive(), field.Int64("window_seconds").NonNegative()}
}

type TransmissionTarget struct{ ent.Schema }

func (TransmissionTarget) Policy() ent.Policy { return guard.Policy("TransmissionTarget") }
func (TransmissionTarget) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "transmission_targets"}}
}
func (TransmissionTarget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("transmission_id", uuid.UUID{}),
		field.Int("position").NonNegative(),
		field.JSON("original", json.RawMessage{}).SchemaType(map[string]string{dialect.Postgres: "json"}).Sensitive(),
		field.Enum("resolution_status").Values("unresolved", "unassigned", "assigned", "pending", "failed"),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("matched_rule_id", uuid.UUID{}).Optional().Nillable(),
		field.Int64("assignment_revision").Positive(),
	}
}
func (TransmissionTarget) Indexes() []ent.Index {
	return []ent.Index{index.Fields("transmission_id", "position").Unique(), index.Fields("unit_id")}
}

type MessageContribution struct{ ent.Schema }

func (MessageContribution) Policy() ent.Policy { return guard.Policy("MessageContribution") }
func (MessageContribution) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "message_contributions"}}
}
func (MessageContribution) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.UUID("message_id", uuid.UUID{}),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("contribution_time"),
		field.Time("age_anchor"),
		field.Bool("ready").Default(false),
	}
}
func (MessageContribution) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("message_id", "unit_id").Unique().Annotations(entsql.IndexWhere("unit_id IS NOT NULL")),
		index.Fields("message_id").Unique().Annotations(entsql.IndexWhere("unit_id IS NULL")),
		index.Fields("unit_id", "age_anchor"),
	}
}

type ContributionSupport struct{ ent.Schema }

func (ContributionSupport) Policy() ent.Policy { return guard.Policy("ContributionSupport") }
func (ContributionSupport) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "contribution_supports"}}
}
func (ContributionSupport) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.UUID("contribution_id", uuid.UUID{}),
		field.UUID("transmission_id", uuid.UUID{}),
		field.Int("target_position").Optional().Nillable().NonNegative(),
		field.Time("observed_at"),
		field.Time("accepted_at"),
	}
}
func (ContributionSupport) Indexes() []ent.Index {
	return []ent.Index{index.Fields("transmission_id", "target_position").Unique(), index.Fields("contribution_id")}
}
