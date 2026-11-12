package processing

import (
	"errors"
	"sort"
	"strings"
)

// emptyObject is the object value without members.
var emptyObject = Value{Kind: ObjectValue, Value: []byte("{}")}

// SecretReferenceStrings returns the reference strings held at the Secret paths of a configuration
// value, keyed by dotted path. Absent and null properties are skipped; anything but a string at a
// Secret path is invalid input. The strings are not parsed.
func (m InstalledManifest) SecretReferenceStrings(value Value) (map[string]string, error) {
	found := map[string]string{}
	_, err := rewriteSecrets(m.ConfigurationSchema, m.ConfigurationPolicy, value, "", func(path string, leaf Value) (Value, error) {
		var encoded string
		if leaf.Kind != StringValue {
			return Value{}, invalid(errors.New("secret property is not a string"))
		}
		if err := Decode(leaf.Value, &encoded); err != nil {
			return Value{}, err
		}
		found[path] = encoded
		return leaf, nil
	})
	return found, err
}

// RejectSecretValues fails with invalid input when a configuration value carries a value at any
// Secret path other than null: Secrets travel only in a separate map, never inside the
// configuration, so a write cannot smuggle a plaintext or a reference past that map's checks.
func (m InstalledManifest) RejectSecretValues(value Value) error {
	_, err := rewriteSecrets(m.ConfigurationSchema, m.ConfigurationPolicy, value, "", func(string, Value) (Value, error) {
		return Value{}, invalid(errors.New("secret property inside configuration"))
	})
	return err
}

// WithSecretReferences returns the configuration object with each reference string set at its
// dotted Secret path; missing containers on the way are created. The value must be an object.
func (m InstalledManifest) WithSecretReferences(value Value, references map[string]string) (Value, error) {
	paths := make([]string, 0, len(references))
	for path := range references {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		leaf, err := Literal(StringValue, references[path])
		if err != nil {
			return Value{}, err
		}
		if value, err = setAtPath(value, strings.Split(path, "."), leaf); err != nil {
			return Value{}, err
		}
	}
	return value, nil
}

// setAtPath sets leaf below the object at the path, creating absent containers.
func setAtPath(object Value, path []string, leaf Value) (Value, error) {
	members, err := decodeObject(object)
	if err != nil {
		return Value{}, err
	}
	if len(path) == 1 {
		members[path[0]] = leaf
		return encodeObject(members)
	}
	child, found := members[path[0]]
	if !found {
		child = emptyObject
	}
	updated, err := setAtPath(child, path[1:], leaf)
	if err != nil {
		return Value{}, err
	}
	members[path[0]] = updated
	return encodeObject(members)
}

// EmptyConfiguration returns an object value without members.
func EmptyConfiguration() Value { return emptyObject }

// ValidatePersonalConfiguration checks a personal configuration object: it holds only declared
// user-scoped properties, each valid for its declaration. An administrative or undeclared property
// is invalid input.
func (m InstalledManifest) ValidatePersonalConfiguration(value Value) error {
	if m.EffectiveCapability() != CapabilityDelivery {
		return invalid(errors.New("plugin is not a delivery capability"))
	}
	return validatePersonalNode(m.ConfigurationSchema, m.ConfigurationPolicy, value)
}

func validatePersonalNode(schema FieldSchema, policy ConfigurationPolicy, value Value) error {
	if policy.Scope == "user" {
		_, err := value.Native(schema)
		return err
	}
	if schema.Kind != ObjectValue || !hasScope(policy, "user") {
		return invalid(errors.New("administrative property in personal configuration"))
	}
	members, err := decodeObject(value)
	if err != nil {
		return err
	}
	for name, child := range members {
		declared, known := schema.Properties[name]
		if !known {
			return invalid(errors.New("undeclared personal property"))
		}
		if err := validatePersonalNode(declared, policy.Properties[name], child); err != nil {
			return err
		}
	}
	return nil
}

// RetainableSecretReferences returns the previous references that may be carried over into the
// submitted configuration: a reference qualifies only when every parent object of its path is
// present in the submitted value, so omitting an optional branch removes its Secrets with it
// instead of recreating the branch. Top-level Secrets always qualify.
func (m InstalledManifest) RetainableSecretReferences(previous map[string]string, submitted Value) map[string]string {
	retained := make(map[string]string, len(previous))
	for path, reference := range previous {
		if parentsPresent(submitted, strings.Split(path, ".")[:strings.Count(path, ".")]) {
			retained[path] = reference
		}
	}
	return retained
}

// parentsPresent reports whether the object has a nested object at each name of the path in turn.
func parentsPresent(object Value, parents []string) bool {
	if object.Kind != ObjectValue {
		return false
	}
	if len(parents) == 0 {
		return true
	}
	members, err := decodeObject(object)
	if err != nil {
		return false
	}
	child, found := members[parents[0]]
	return found && parentsPresent(child, parents[1:])
}
