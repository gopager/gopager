package processing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

// PersonalConfiguration is one subscriber's user-scoped values for a Delivery instance. Secret
// properties inside it are reference strings, like in the administrative record.
type PersonalConfiguration struct {
	InstanceID    uuid.UUID
	OwnerID       uuid.UUID
	Revision      int64
	Configuration Value
	UpdatedAt     time.Time
}

// PersonalConfigurations reads personal values inside the caller's transaction.
type PersonalConfigurations interface {
	// ReadPersonalConfiguration returns the owner's values for the instance, or authz.ErrNotFound
	// when the owner has none.
	ReadPersonalConfiguration(ctx context.Context, tx platform.TransactionContext,
		instanceID, ownerID uuid.UUID) (PersonalConfiguration, error)
}

// mergeConfiguration combines the administrative and the personal object into the single object
// the plugin receives. Every property has exactly one scope, so nothing shadows anything: a
// user-scoped property comes only from personal, any other only from admin. Personal values for
// administrative or undeclared properties are ignored, as is an administrative value at a personal
// one. A nil personal value merges nothing. The result still holds Secret references.
func (m InstalledManifest) mergeConfiguration(admin Value, personal *Value) (Value, error) {
	merged, err := mergeNode(m.ConfigurationSchema, m.ConfigurationPolicy, &admin, personal)
	if err != nil || merged == nil {
		return admin, err
	}
	return *merged, nil
}

// mergeNode merges one node; a nil result means neither side has a value.
func mergeNode(schema FieldSchema, policy ConfigurationPolicy, admin, personal *Value) (*Value, error) {
	if policy.Scope == "user" {
		return personal, nil
	}
	if schema.Kind != ObjectValue || admin == nil && (personal == nil || !hasScope(policy, "user")) {
		return admin, nil
	}
	if admin != nil && admin.Kind == NullValue {
		return admin, nil
	}
	var adminMembers, personalMembers map[string]Value
	var err error
	if admin != nil {
		if adminMembers, err = decodeObject(*admin); err != nil {
			return nil, err
		}
	}
	if personal != nil && personal.Kind == ObjectValue {
		if personalMembers, err = decodeObject(*personal); err != nil {
			return nil, err
		}
	}
	members := map[string]Value{}
	for name, child := range schema.Properties {
		merged, err := mergeNode(child, policy.Properties[name], lookup(adminMembers, name), lookup(personalMembers, name))
		if err != nil {
			return nil, err
		}
		if merged != nil {
			members[name] = *merged
		}
	}
	if admin == nil && len(members) == 0 {
		return nil, nil
	}
	object, err := encodeObject(members)
	if err != nil {
		return nil, err
	}
	return &object, nil
}

// lookup returns a pointer to the member, or nil when absent.
func lookup(members map[string]Value, name string) *Value {
	value, found := members[name]
	if !found {
		return nil
	}
	return &value
}
