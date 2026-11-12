package processing

import (
	"context"
	"encoding/json"
	"errors"
)

// secretSourceOf names where a configured Secret value comes from, or "" when the value is not a
// valid reference. It never exposes the reference itself.
func secretSourceOf(value any) string {
	encoded, ok := value.(string)
	if !ok {
		return ""
	}
	reference, err := ParseSecretReference(encoded)
	if err != nil {
		return ""
	}
	return string(reference.Source)
}

// ProjectPersonalConfiguration returns the user-scoped properties of a personal configuration for
// its owner: Secrets show only that they are configured, redacted, with their source; other
// personal values are shown, since they are the owner's own. Administrative properties never
// appear. Containers appear only when they hold personal properties.
func (m InstalledManifest) ProjectPersonalConfiguration(personal Value) (InstanceConfigurationView, error) {
	members, err := decodeObject(personal)
	if err != nil {
		return InstanceConfigurationView{}, err
	}
	view := InstanceConfigurationView{Fields: map[string]ConfigurationFieldView{}}
	for name, schema := range m.ConfigurationSchema.Properties {
		policy := m.ConfigurationPolicy.Properties[name]
		if !hasScope(policy, "user") {
			continue
		}
		field, err := projectPersonalField(schema, policy, lookup(members, name))
		if err != nil {
			return InstanceConfigurationView{}, err
		}
		view.Fields[name] = field
	}
	return view, nil
}

// projectPersonalField projects one personal property or a container holding some.
func projectPersonalField(schema FieldSchema, policy ConfigurationPolicy, value *Value) (ConfigurationFieldView, error) {
	if policy.Secret == nil {
		return ConfigurationFieldView{}, capabilityUnavailable(errors.New("configuration view policy unavailable"))
	}
	view := ConfigurationFieldView{Scope: policy.Scope, Secret: *policy.Secret, Configured: value != nil}
	if value == nil {
		return view, nil
	}
	if policy.Scope != "user" {
		members, err := decodeObject(*value)
		if err != nil {
			return ConfigurationFieldView{}, err
		}
		view.Properties = map[string]ConfigurationFieldView{}
		for name, child := range schema.Properties {
			if !hasScope(policy.Properties[name], "user") {
				continue
			}
			projected, err := projectPersonalField(child, policy.Properties[name], lookup(members, name))
			if err != nil {
				return ConfigurationFieldView{}, err
			}
			view.Properties[name] = projected
		}
		return view, nil
	}
	if *policy.Secret {
		native, err := value.Native(schema)
		if err != nil {
			return ConfigurationFieldView{}, err
		}
		view.Redacted, view.Source = true, secretSourceOf(native)
		return view, nil
	}
	native, err := value.Native(schema)
	if err != nil {
		return ConfigurationFieldView{}, err
	}
	if hasSecretPolicy(policy) {
		view.Redacted = true
		return view, nil
	}
	encoded, err := json.Marshal(native)
	if err != nil {
		return ConfigurationFieldView{}, invalid(err)
	}
	view.Value = encoded
	return view, nil
}

// ProjectPersonalConfiguration loads the instance's pinned manifest without starting the plugin
// and projects the owner's personal configuration for display.
func (i *LocalInstanceInspector) ProjectPersonalConfiguration(ctx context.Context, configuration InstanceConfiguration,
	personal PersonalConfiguration) (InstanceConfigurationView, error) {
	if i == nil || ctx == nil {
		return InstanceConfigurationView{}, invalid(errors.New("missing instance view context"))
	}
	if personal.InstanceID != configuration.ID {
		return InstanceConfigurationView{}, invalid(errors.New("personal configuration belongs to another instance"))
	}
	manifest, _, err := i.host.loadVerified(ctx, i.root, configuration.Main.ManifestPath, configuration.Main.Pin, configuration.ID)
	if err != nil {
		return InstanceConfigurationView{}, err
	}
	if manifest.EffectiveCapability() != CapabilityDelivery {
		return InstanceConfigurationView{}, capabilityUnavailable(errors.New("instance has no personal configuration"))
	}
	return manifest.ProjectPersonalConfiguration(personal.Configuration)
}
