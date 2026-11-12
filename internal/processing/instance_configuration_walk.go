package processing

import (
	"encoding/json"
	"errors"
	"sort"
)

// SecretProperty describes one declared Secret property of a manifest.
type SecretProperty struct {
	// Path is the dotted property path, for example "auth.token".
	Path     string
	Scope    string
	Optional bool
}

// SecretProperties lists the manifest's Secret properties by path. Secrets never sit inside lists,
// so only object properties are walked.
func (m InstalledManifest) SecretProperties() []SecretProperty {
	var found []SecretProperty
	var visit func(schema FieldSchema, policy ConfigurationPolicy, path string)
	visit = func(schema FieldSchema, policy ConfigurationPolicy, path string) {
		if policy.Secret != nil && *policy.Secret && path != "" {
			found = append(found, SecretProperty{Path: path, Scope: policy.Scope, Optional: schema.Optional})
		}
		if schema.Kind != ObjectValue {
			return
		}
		for name, child := range schema.Properties {
			visit(child, policy.Properties[name], joinPath(path, name))
		}
	}
	visit(m.ConfigurationSchema, m.ConfigurationPolicy, "")
	sort.Slice(found, func(i, j int) bool { return found[i].Path < found[j].Path })
	return found
}

// HasPersonalProperties reports whether any declared property is user-scoped.
func (m InstalledManifest) HasPersonalProperties() bool {
	return hasScope(m.ConfigurationPolicy, "user")
}

func hasScope(policy ConfigurationPolicy, scope string) bool {
	if policy.Scope == scope {
		return true
	}
	for _, child := range policy.Properties {
		if hasScope(child, scope) {
			return true
		}
	}
	return policy.Element != nil && hasScope(*policy.Element, scope)
}

// ConfigurationSecretReferences returns the references held at Secret paths of a configuration
// value (administrative, personal or merged), keyed by dotted path. A Secret property holding
// anything but a valid reference string is invalid input; absent and null properties are skipped.
func (m InstalledManifest) ConfigurationSecretReferences(value Value) (map[string]SecretReference, error) {
	references := map[string]SecretReference{}
	_, err := rewriteSecrets(m.ConfigurationSchema, m.ConfigurationPolicy, value, "", func(path string, leaf Value) (Value, error) {
		var encoded string
		if leaf.Kind != StringValue {
			return Value{}, invalid(errors.New("secret property is not a string"))
		}
		if err := Decode(leaf.Value, &encoded); err != nil {
			return Value{}, err
		}
		reference, err := ParseSecretReference(encoded)
		if err != nil {
			return Value{}, err
		}
		references[path] = reference
		return leaf, nil
	})
	if err != nil {
		return nil, err
	}
	return references, nil
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// rewriteSecrets walks an object value along its schema and policy and replaces the value of every
// Secret property with fn's result. Objects are rebuilt only below objects; null and absent
// properties are left alone and undeclared properties are invalid.
func rewriteSecrets(schema FieldSchema, policy ConfigurationPolicy, value Value, path string,
	fn func(path string, leaf Value) (Value, error)) (Value, error) {
	if value.Kind == NullValue {
		return value, nil
	}
	if policy.Secret != nil && *policy.Secret && path != "" {
		return fn(path, value)
	}
	if schema.Kind != ObjectValue {
		return value, nil
	}
	properties, err := decodeObject(value)
	if err != nil {
		return Value{}, err
	}
	for name, child := range properties {
		declared, known := schema.Properties[name]
		if !known {
			return Value{}, invalid(errors.New("undeclared configuration property"))
		}
		rewritten, err := rewriteSecrets(declared, policy.Properties[name], child, joinPath(path, name), fn)
		if err != nil {
			return Value{}, err
		}
		properties[name] = rewritten
	}
	return encodeObject(properties)
}

// decodeObject returns the members of an object value.
func decodeObject(value Value) (map[string]Value, error) {
	if value.Kind != ObjectValue {
		return nil, invalid(errors.New("configuration is not an object"))
	}
	var properties map[string]Value
	if err := Decode(value.Value, &properties); err != nil {
		return nil, err
	}
	if properties == nil {
		properties = map[string]Value{}
	}
	return properties, nil
}

// encodeObject builds an object value from its members.
func encodeObject(properties map[string]Value) (Value, error) {
	encoded, err := json.Marshal(properties)
	if err != nil {
		return Value{}, invalid(err)
	}
	return Value{Kind: ObjectValue, Value: encoded}, nil
}
