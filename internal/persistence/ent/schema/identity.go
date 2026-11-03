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

type IdentityBase struct{ mixin.Schema }

func (IdentityBase) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Int64("revision").Default(1).Positive(),
	}
}

type Account struct{ ent.Schema }

func (Account) Policy() ent.Policy { return guard.Policy("Account") }
func (Account) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Account) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_accounts"}}
}
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

type UserGroup struct{ ent.Schema }

func (UserGroup) Policy() ent.Policy { return guard.Policy("UserGroup") }
func (UserGroup) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (UserGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_user_groups"}}
}
func (UserGroup) Fields() []ent.Field {
	return []ent.Field{field.String("name").NotEmpty().MaxRuneLen(256)}
}

type Membership struct{ ent.Schema }

func (Membership) Policy() ent.Policy { return guard.Policy("Membership") }
func (Membership) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Membership) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_memberships"}}
}
func (Membership) Fields() []ent.Field {
	return []ent.Field{field.UUID("account_id", uuid.UUID{}), field.UUID("group_id", uuid.UUID{})}
}
func (Membership) Indexes() []ent.Index {
	return []ent.Index{index.Fields("account_id", "group_id").Unique()}
}

type Grant struct{ ent.Schema }

func (Grant) Policy() ent.Policy { return guard.Policy("Grant") }
func (Grant) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Grant) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_grants"}}
}
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
func (Grant) Indexes() []ent.Index {
	return []ent.Index{index.Fields("subject_id", "subject_kind")}
}

// FieldPermission stores globally enabled field groups independently of action grants.
type FieldPermission struct{ ent.Schema }

func (FieldPermission) Policy() ent.Policy { return guard.Policy("FieldPermission") }
func (FieldPermission) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (FieldPermission) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_field_permissions"}}
}
func (FieldPermission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("subject_id", uuid.UUID{}),
		field.Enum("subject_kind").Values("principal", "userGroup"),
		field.JSON("field_names", []string{}),
		field.Bool("delegable").Default(false),
	}
}
func (FieldPermission) Indexes() []ent.Index {
	return []ent.Index{index.Fields("subject_id", "subject_kind")}
}

type Credential struct{ ent.Schema }

func (Credential) Policy() ent.Policy { return guard.Policy("Credential") }
func (Credential) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (Credential) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_credentials"}}
}
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
func (Credential) Indexes() []ent.Index {
	return []ent.Index{index.Fields("account_id", "kind")}
}

type IdentityControl struct{ ent.Schema }

func (IdentityControl) Policy() ent.Policy { return guard.Policy("IdentityControl") }
func (IdentityControl) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_control"}}
}
func (IdentityControl) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int64("access_revision").Default(1).Positive(),
		field.Bool("bootstrapped").Default(false),
	}
}

type AuditRecord struct{ ent.Schema }

func (AuditRecord) Policy() ent.Policy { return guard.Policy("AuditRecord") }
func (AuditRecord) Mixin() []ent.Mixin { return []ent.Mixin{IdentityBase{}} }
func (AuditRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_audit"}}
}
func (AuditRecord) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("actor_id", uuid.UUID{}),
		field.UUID("target_id", uuid.UUID{}),
		field.String("action").NotEmpty(),
		field.String("outcome").NotEmpty(),
		field.Time("recorded_at"),
	}
}

type DomainEvent struct{ ent.Schema }

func (DomainEvent) Policy() ent.Policy { return guard.Policy("DomainEvent") }
func (DomainEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "domain_events"}}
}
func (DomainEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.JSON("envelope", json.RawMessage{}),
	}

}

type LoginAttempt struct{ ent.Schema }

func (LoginAttempt) Policy() ent.Policy { return guard.Policy("LoginAttempt") }
func (LoginAttempt) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "identity_login_attempts"}}
}
func (LoginAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Bytes("account_key").Sensitive(),
		field.Time("attempted_at"),
	}
}
func (LoginAttempt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("attempted_at"), index.Fields("account_key", "attempted_at")}
}
