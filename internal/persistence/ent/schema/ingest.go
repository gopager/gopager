package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
)

// Network defines the networks table: a paging Network of one Protocol; soft-deleted via the
// deleted flag.
type Network struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Network schema.
func (Network) Policy() ent.Policy { return guard.Policy("Network") }

// Annotations returns the table name and code-generation annotations of the Network schema.
func (Network) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "networks"}}
}

// Fields returns the columns of the Network schema.
func (Network) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Default(1).Positive(),
		field.String("name").NotEmpty().MaxRuneLen(256),
		field.Enum("protocol").Values("pocsag", "flex", "zvei", "eas"),
		field.UUID("agency_id", uuid.UUID{}),
		field.Bool("deleted").Default(false),
	}
}

// Edges returns the relations of the Network schema: its home Agency and the Address rules in it.
func (Network) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agency", Agency.Type).Ref("networks").Field("agency_id").Unique().Required(),
		edge.To("address_rules", AddressRule.Type),
	}
}

// Indexes returns the indexes of the Network schema.
func (Network) Indexes() []ent.Index {
	return []ent.Index{index.Fields("agency_id")}
}

// Gateway defines the gateways table: an ingest gateway with its health state and last
// contact/report.
type Gateway struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Gateway schema.
func (Gateway) Policy() ent.Policy { return guard.Policy("Gateway") }

// Annotations returns the table name and code-generation annotations of the Gateway schema.
func (Gateway) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gateways"}}
}

// Fields returns the columns of the Gateway schema.
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

// GatewayNetwork defines the gateway_networks table: the mapping from a gateway's own network
// identifier to a Network; identifiers and Networks are each unique per gateway.
type GatewayNetwork struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the GatewayNetwork schema.
func (GatewayNetwork) Policy() ent.Policy { return guard.Policy("GatewayNetwork") }

// Annotations returns the table name and code-generation annotations of the GatewayNetwork schema.
func (GatewayNetwork) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gateway_networks"}}
}

// Fields returns the columns of the GatewayNetwork schema.
func (GatewayNetwork) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("gateway_id", uuid.UUID{}),
		field.String("gateway_network_identifier").NotEmpty().MaxRuneLen(128),
		field.UUID("network_id", uuid.UUID{}),
		field.Bool("default"),
	}
}

// Indexes returns the indexes and unique constraints of the GatewayNetwork schema.
func (GatewayNetwork) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("gateway_id", "gateway_network_identifier").Unique(),
		index.Fields("gateway_id", "network_id").Unique(),
	}
}

// GatewayCredential defines the gateway_credentials table: a gateway secret stored as a unique
// hash; each gateway has at most one unrevoked current and one unrevoked pending credential.
type GatewayCredential struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the GatewayCredential schema.
func (GatewayCredential) Policy() ent.Policy { return guard.Policy("GatewayCredential") }

// Annotations returns the table name and code-generation annotations of the GatewayCredential schema.
func (GatewayCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gateway_credentials"}}
}

// Fields returns the columns of the GatewayCredential schema.
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

// Indexes returns the indexes and unique constraints of the GatewayCredential schema.
func (GatewayCredential) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("secret_hash").Unique(),
		index.Fields("gateway_id").Unique().Annotations(entsql.IndexWhere("revoked_at IS NULL AND pending = false")),
		index.Fields("gateway_id").Unique().Annotations(entsql.IndexWhere("revoked_at IS NULL AND pending = true")),
	}
}

// Transmission defines the transmissions table: one accepted gateway submission (unique per
// gateway and submission ID) with its payload, targets, grouped Message and
// processing/publication status.
type Transmission struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Transmission schema.
func (Transmission) Policy() ent.Policy { return guard.Policy("Transmission") }

// Annotations returns the table name and code-generation annotations of the Transmission schema.
func (Transmission) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "transmissions"}}
}

// Fields returns the columns of the Transmission schema.
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
		field.Time("observed_at"),
		field.Int("observation_offset_seconds"),
		field.Time("accepted_at"),
		field.Int64("source_revision").Positive(),
		field.Bool("incomplete").Default(false),
		field.JSON("quality_metrics", json.RawMessage{}).SchemaType(map[string]string{dialect.Postgres: "json"}).Optional().Sensitive(),
		field.UUID("message_id", uuid.UUID{}),
		field.Enum("processing_status").Values("none", "pending", "completed", "failed").Default("pending"),
	}
}

// Indexes returns the indexes and unique constraints of the Transmission schema.
func (Transmission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("gateway_id", "submission_id").Unique(),
		index.Fields("protocol", "observed_at", "id"),
		index.Fields("message_id", "observed_at", "id"),
	}
}

// Message defines the messages table: the logical Message that groups Transmissions with the
// same identity within the grouping window.
type Message struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Message schema.
func (Message) Policy() ent.Policy { return guard.Policy("Message") }

// Annotations returns the table name and code-generation annotations of the Message schema.
func (Message) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "messages"}}
}

// Fields returns the columns of the Message schema.
func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("revision").Default(1).Positive(),
		field.String("identity_form").NotEmpty(),
		field.String("identity_subtype").Default(""),
		field.Bytes("identity_value").Sensitive(),
		field.Time("first_observed_at"),
		field.Time("last_observed_at"),
	}
}

// Indexes returns the indexes and unique constraints of the Message schema.
func (Message) Indexes() []ent.Index {
	return []ent.Index{index.Fields("identity_form", "identity_subtype", "identity_value", "last_observed_at")}
}

// GroupingControl defines the single-row grouping_control table holding the Message grouping
// window and its revision.
type GroupingControl struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the GroupingControl schema.
func (GroupingControl) Policy() ent.Policy { return guard.Policy("GroupingControl") }

// Annotations returns the table name and code-generation annotations of the GroupingControl schema.
func (GroupingControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "grouping_control"}}
}

// Fields returns the columns of the GroupingControl schema.
func (GroupingControl) Fields() []ent.Field {
	return []ent.Field{field.Int("id"), field.Int64("revision").Default(1).Positive(), field.Int64("window_seconds").NonNegative()}
}

// TransmissionTarget defines the transmission_targets table: one target of a Transmission
// (unique per position) with its current resolution and the Assignment revision it was
// resolved at.
type TransmissionTarget struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the TransmissionTarget schema.
func (TransmissionTarget) Policy() ent.Policy { return guard.Policy("TransmissionTarget") }

// Annotations returns the table name and code-generation annotations of the TransmissionTarget schema.
func (TransmissionTarget) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "transmission_targets"}}
}

// Fields returns the columns of the TransmissionTarget schema.
func (TransmissionTarget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("transmission_id", uuid.UUID{}),
		field.Int("position").NonNegative(),
		field.JSON("original", json.RawMessage{}).SchemaType(map[string]string{dialect.Postgres: "json"}).Sensitive(),
		field.Enum("resolution_status").Values("unresolved", "unassigned", "assigned", "pending", "failed"),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("matched_rule_id", uuid.UUID{}).Optional().Nillable(),
		field.Int64("assignment_revision").Positive(),
		field.UUID("network_id", uuid.UUID{}),
		field.Enum("protocol").Values("pocsag", "flex", "zvei", "eas"),
		field.Time("observed_at"),
		field.Time("accepted_at"),
		field.Int64("address_number").Optional().Nillable(),
		field.String("address_text").Optional().Nillable(),
		field.Int16("function").Optional().Nillable(),
	}
}

// Indexes returns the indexes and unique constraints of the TransmissionTarget schema: the typed
// address columns serve selector-scoped correction, the Unit index serves Unit history.
func (TransmissionTarget) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("transmission_id", "position").Unique(),
		index.Fields("network_id", "protocol", "address_number", "observed_at").Annotations(entsql.IndexWhere("address_number IS NOT NULL")),
		index.Fields("network_id", "protocol", "address_text", "observed_at").Annotations(entsql.IndexWhere("address_text IS NOT NULL")),
		index.Fields("unit_id", "observed_at"),
	}
}

// MessageUnit defines the message_units table: the rebuildable cache of which Units a Message
// alerts, one row per Message and Unit (with the Unit's Agency) or one Unit-less row per Message
// and Network Agency for targetless Transmissions, with the earliest observation, the age anchor
// and the correction status that feeds and authorization predicates read directly.
type MessageUnit struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the MessageUnit schema.
func (MessageUnit) Policy() ent.Policy { return guard.Policy("MessageUnit") }

// Annotations returns the table name and code-generation annotations of the MessageUnit schema.
func (MessageUnit) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "message_units"}}
}

// Fields returns the columns of the MessageUnit schema.
func (MessageUnit) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("message_id", uuid.UUID{}),
		field.UUID("unit_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("agency_id", uuid.UUID{}),
		field.Time("first_observed_at"),
		field.Time("age_anchor"),
		field.Enum("correction_status").Values("current", "correcting").Default("current"),
	}
}

// Indexes returns the indexes and unique constraints of the MessageUnit schema.
func (MessageUnit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("message_id", "unit_id").Unique().Annotations(entsql.IndexWhere("unit_id IS NOT NULL")),
		index.Fields("message_id", "agency_id").Unique().Annotations(entsql.IndexWhere("unit_id IS NULL")),
		index.Fields("unit_id", "age_anchor"),
		index.Fields("agency_id", "first_observed_at"),
	}
}
