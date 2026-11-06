package schema

import (
	"encoding/json"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
)

// IdentityBase is the mixin adding a generated UUID id and a positive revision (starting at 1)
// to entity schemas.
type IdentityBase struct{ mixin.Schema }

// Fields returns the columns of the IdentityBase schema.
func (IdentityBase) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Int64("revision").Default(1).Positive(),
	}
}

// Account defines the identity_accounts table: a human, technical or guest Principal with
// optional username (unique normalized key) and password hash.
type Account struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Account schema.
func (Account) Policy() ent.Policy { return guard.Policy("Account") }

// Mixin returns the mixins shared into the Account schema.
func (Account) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Account schema.
func (Account) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_accounts"}}
}

// Fields returns the columns of the Account schema.
func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("kind").Values("human", "technical", "guest"),
		field.Enum("state").Values("active", "suspended", "deleting", "deleted").Default("active"),
		field.Text("username").Optional().Nillable(),
		field.Text("username_key").Optional().Nillable().Unique(),
		field.Text("password_hash").Optional().Nillable().Sensitive(),
		field.Text("display_name").Default(""),
		field.Time("created_at"),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

// UserGroup defines the identity_user_groups table: a named group of Accounts that grants can
// target.
type UserGroup struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the UserGroup schema.
func (UserGroup) Policy() ent.Policy { return guard.Policy("UserGroup") }

// Mixin returns the mixins shared into the UserGroup schema.
func (UserGroup) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the UserGroup schema.
func (UserGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_user_groups"}}
}

// Fields returns the columns of the UserGroup schema.
func (UserGroup) Fields() []ent.Field {
	return []ent.Field{field.String("name").NotEmpty().MaxRuneLen(256)}
}

// Membership defines the identity_memberships table: one Account's membership in a UserGroup,
// unique per pair.
type Membership struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Membership schema.
func (Membership) Policy() ent.Policy { return guard.Policy("Membership") }

// Mixin returns the mixins shared into the Membership schema.
func (Membership) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Membership schema.
func (Membership) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_memberships"}}
}

// Fields returns the columns of the Membership schema.
func (Membership) Fields() []ent.Field {
	return []ent.Field{field.UUID("account_id", uuid.UUID{}), field.UUID("group_id", uuid.UUID{})}
}

// Indexes returns the indexes and unique constraints of the Membership schema.
func (Membership) Indexes() []ent.Index {
	return []ent.Index{index.Fields("account_id", "group_id").Unique()}
}

// Grant defines the identity_grants table: actions granted to a Principal or UserGroup within
// a scope, with optional maximum data age and delegability.
type Grant struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Grant schema.
func (Grant) Policy() ent.Policy { return guard.Policy("Grant") }

// Mixin returns the mixins shared into the Grant schema.
func (Grant) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Grant schema.
func (Grant) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_grants"}}
}

// Fields returns the columns of the Grant schema.
func (Grant) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("subject_id", uuid.UUID{}),
		field.Enum("subject_kind").Values("principal", "userGroup"),
		field.JSON("actions", []string{}),
		field.JSON("scope", authz.Scope{}),
		field.Int64("max_age_seconds").Optional().Nillable().NonNegative(),
		field.Bool("delegable").Default(false),
	}
}

// Indexes returns the indexes and unique constraints of the Grant schema.
func (Grant) Indexes() []ent.Index {
	return []ent.Index{index.Fields("subject_id", "subject_kind")}
}

// FieldPermission stores globally enabled field groups independently of action grants.
type FieldPermission struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the FieldPermission schema.
func (FieldPermission) Policy() ent.Policy { return guard.Policy("FieldPermission") }

// Mixin returns the mixins shared into the FieldPermission schema.
func (FieldPermission) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the FieldPermission schema.
func (FieldPermission) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_field_permissions"}}
}

// Fields returns the columns of the FieldPermission schema.
func (FieldPermission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("subject_id", uuid.UUID{}),
		field.Enum("subject_kind").Values("principal", "userGroup"),
		field.JSON("field_names", []string{}),
		field.Bool("delegable").Default(false),
	}
}

// Indexes returns the indexes and unique constraints of the FieldPermission schema.
func (FieldPermission) Indexes() []ent.Index {
	return []ent.Index{index.Fields("subject_id", "subject_kind")}
}

// Credential defines the identity_credentials table: a session, access key or recovery secret
// of an Account, stored only as a unique hash.
type Credential struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the Credential schema.
func (Credential) Policy() ent.Policy { return guard.Policy("Credential") }

// Mixin returns the mixins shared into the Credential schema.
func (Credential) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the Credential schema.
func (Credential) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_credentials"}}
}

// Fields returns the columns of the Credential schema.
func (Credential) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("account_id", uuid.UUID{}),
		field.Enum("kind").Values("session", "accessKey", "recovery"),
		field.Bytes("secret_hash").Sensitive().Unique(),
		field.String("csrf_token").Optional().Nillable().Sensitive(),
		field.JSON("restrictions", &authz.Restrictions{}).Optional(),
		field.Time("created_at"),
		field.Time("last_used_at"),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
		field.Time("consumed_at").Optional().Nillable(),
	}
}

// Indexes returns the indexes and unique constraints of the Credential schema.
func (Credential) Indexes() []ent.Index {
	return []ent.Index{index.Fields("account_id", "kind")}
}

// IdentityControl defines the single-row identity_control table holding the access revision
// (locked to serialize access changes) and the bootstrap flag.
type IdentityControl struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the IdentityControl schema.
func (IdentityControl) Policy() ent.Policy { return guard.Policy("IdentityControl") }

// Annotations returns the table name and code-generation annotations of the IdentityControl schema.
func (IdentityControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_control"}}
}

// Fields returns the columns of the IdentityControl schema.
func (IdentityControl) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int64("access_revision").Default(1).Positive(),
		field.Bool("bootstrapped").Default(false),
	}
}

// AuditRecord defines the identity_audit table: one audited administrative action with actor,
// target and outcome.
type AuditRecord struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the AuditRecord schema.
func (AuditRecord) Policy() ent.Policy { return guard.Policy("AuditRecord") }

// Mixin returns the mixins shared into the AuditRecord schema.
func (AuditRecord) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }

// Annotations returns the table name and code-generation annotations of the AuditRecord schema.
func (AuditRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_audit"}}
}

// Fields returns the columns of the AuditRecord schema.
func (AuditRecord) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("actor_id", uuid.UUID{}),
		field.UUID("target_id", uuid.UUID{}),
		field.String("action").NotEmpty(),
		field.String("outcome").NotEmpty(),
		field.Time("recorded_at"),
	}
}

// DomainEvent defines the domain_events table: one recorded domain fact stored as its JSON
// envelope, keyed by fact ID and ordered by the database-assigned seq.
//
// The columns seq, type and occurred_at are generated by the database (identity and stored
// generated columns), so the schema marks them optional and immutable to keep Ent from ever
// writing them. The tx_id column is managed by the database alone and read only through the
// eventstore's gap-safe reader, so it is not modelled here.
type DomainEvent struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the DomainEvent schema.
func (DomainEvent) Policy() ent.Policy { return guard.Policy("DomainEvent") }

// Annotations returns the table name and code-generation annotations of the DomainEvent schema.
func (DomainEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "domain_events"}}
}

// Fields returns the columns of the DomainEvent schema.
func (DomainEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int64("seq").Optional().Immutable().Unique(),
		field.JSON("envelope", json.RawMessage{}),
		field.String("type").Optional().Immutable(),
		field.Time("occurred_at").Optional().Immutable(),
	}
}

// ConsumerOffset defines the consumer_offsets table: the last domain_events seq a named
// consumer has durably processed.
type ConsumerOffset struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the ConsumerOffset schema.
func (ConsumerOffset) Policy() ent.Policy { return guard.Policy("ConsumerOffset") }

// Annotations returns the table name and code-generation annotations of the ConsumerOffset schema.
func (ConsumerOffset) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "consumer_offsets"}}
}

// Fields returns the columns of the ConsumerOffset schema.
func (ConsumerOffset) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("consumer").Immutable(),
		field.Int64("last_seq").NonNegative(),
		field.Time("updated_at"),
	}
}

// LoginAttempt defines the identity_login_attempts table: a timestamped login attempt per
// hashed account key, used for login rate limiting.
type LoginAttempt struct{ ent.Schema }

// Policy returns the privacy policy that guards queries and mutations of the LoginAttempt schema.
func (LoginAttempt) Policy() ent.Policy { return guard.Policy("LoginAttempt") }

// Annotations returns the table name and code-generation annotations of the LoginAttempt schema.
func (LoginAttempt) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_login_attempts"}}
}

// Fields returns the columns of the LoginAttempt schema.
func (LoginAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Bytes("account_key").Sensitive(),
		field.Time("attempted_at"),
	}
}

// Indexes returns the indexes and unique constraints of the LoginAttempt schema.
func (LoginAttempt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("attempted_at"), index.Fields("account_key", "attempted_at")}
}
