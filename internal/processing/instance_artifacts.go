package processing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

// LocalInstanceInspector validates provisioned modules/configuration outside primary transactions.
// The caller owns the root and host lifetime; this inspector closes each inspected compilation.
type LocalInstanceInspector struct {
	host *PluginHost
	root *os.Root
}

// NewLocalInstanceInspector binds the existing bounded runtime and Operator artifact directory.
func NewLocalInstanceInspector(host *PluginHost, root *os.Root) (*LocalInstanceInspector, error) {
	if host == nil || root == nil {
		return nil, invalid(errors.New("missing instance inspection dependency"))
	}
	return &LocalInstanceInspector{host: host, root: root}, nil
}

// InspectInstance verifies the exact artifact pin and configuration without running guest code.
func (i *LocalInstanceInspector) InspectInstance(ctx context.Context, configuration InstanceConfiguration) (failure error) {
	if err := configuration.Validate(); err != nil {
		return err
	}
	main, err := i.host.LoadInstalled(ctx, i.root, configuration.Main.ManifestPath, configuration.Main.Pin)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, main.Close(context.WithoutCancel(ctx))) }()
	manifest, err := main.Manifest()
	if err != nil {
		return err
	}
	return manifest.validateInstance(configuration)
}

// ProjectInstanceConfiguration returns declared configuration with Secret and user-scoped values redacted.
func (i *LocalInstanceInspector) ProjectInstanceConfiguration(ctx context.Context,
	configuration InstanceConfiguration) (view InstanceConfigurationView, failure error) {
	if i == nil || ctx == nil {
		return InstanceConfigurationView{}, invalid(errors.New("missing instance view context"))
	}
	if err := configuration.Validate(); err != nil {
		return InstanceConfigurationView{}, err
	}
	main, err := i.host.LoadInstalled(ctx, i.root, configuration.Main.ManifestPath, configuration.Main.Pin)
	if err != nil {
		return InstanceConfigurationView{}, err
	}
	defer func() {
		failure = errors.Join(failure, main.Close(context.WithoutCancel(ctx)))
		if failure != nil {
			view = InstanceConfigurationView{}
		}
	}()
	manifest, err := main.Manifest()
	if err != nil {
		return InstanceConfigurationView{}, err
	}
	native, err := configuration.Configuration.Native(manifest.ConfigurationSchema)
	if err != nil {
		return InstanceConfigurationView{}, err
	}
	fields, ok := native.(map[string]any)
	if !ok {
		return InstanceConfigurationView{}, invalid(errors.New("invalid administrative configuration object"))
	}
	view.Fields = make(map[string]ConfigurationFieldView, len(manifest.ConfigurationSchema.Properties))
	for name, schema := range manifest.ConfigurationSchema.Properties {
		value, exists := fields[name]
		projected, err := projectConfigurationField(schema, manifest.ConfigurationPolicy.Properties[name], value, exists)
		if err != nil {
			return InstanceConfigurationView{}, err
		}
		view.Fields[name] = projected
	}
	return view, nil
}

func projectConfigurationField(schema FieldSchema, policy ConfigurationPolicy, value any,
	configured bool) (ConfigurationFieldView, error) {
	if policy.Secret == nil {
		return ConfigurationFieldView{}, capabilityUnavailable(errors.New("configuration view policy unavailable"))
	}
	view := ConfigurationFieldView{Scope: policy.Scope, Secret: *policy.Secret, Configured: configured}
	if !configured {
		return view, nil
	}
	if *policy.Secret || policy.Scope == "user" {
		view.Redacted = true
		if *policy.Secret {
			view.Source = secretSourceOf(value)
		}
		return view, nil
	}
	switch schema.Kind {
	case ObjectValue:
		fields, ok := value.(map[string]any)
		if !ok {
			return ConfigurationFieldView{}, invalid(errors.New("invalid administrative configuration object"))
		}
		view.Properties = make(map[string]ConfigurationFieldView, len(schema.Properties))
		for name, childSchema := range schema.Properties {
			child, exists := fields[name]
			projected, err := projectConfigurationField(childSchema, policy.Properties[name], child, exists)
			if err != nil {
				return ConfigurationFieldView{}, err
			}
			view.Properties[name] = projected
		}
	case ListValue:
		if configurationPolicyRestricted(*policy.Element) {
			view.Redacted = true
			return view, nil
		}
		values, ok := value.([]any)
		if !ok {
			return ConfigurationFieldView{}, invalid(errors.New("invalid administrative configuration list"))
		}
		view.Items = make([]ConfigurationFieldView, len(values))
		for index, item := range values {
			projected, err := projectConfigurationField(*schema.Element, *policy.Element, item, true)
			if err != nil {
				return ConfigurationFieldView{}, err
			}
			view.Items[index] = projected
		}
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ConfigurationFieldView{}, invalid(err)
		}
		view.Value = encoded
	}
	return view, nil
}

func configurationPolicyRestricted(policy ConfigurationPolicy) bool {
	if policy.Secret == nil || *policy.Secret || policy.Scope == "user" {
		return true
	}
	if policy.Element != nil && configurationPolicyRestricted(*policy.Element) {
		return true
	}
	for _, child := range policy.Properties {
		if configurationPolicyRestricted(child) {
			return true
		}
	}
	return false
}

// validateAdministrativeConfiguration rejects supplied personal properties instead of promoting them.
// Missing required personal settings remain unavailable until their distinct owner supplies them.
func validateAdministrativeConfiguration(schema FieldSchema, policy ConfigurationPolicy, value Value) error {
	native, err := value.Native(schema)
	if err != nil {
		return err
	}
	return validateAdministrativeValue(schema, policy, native)
}

// validateAdministrativeValue walks an already type-checked configuration without reinterpreting values.
func validateAdministrativeValue(schema FieldSchema, policy ConfigurationPolicy, native any) error {
	if policy.Scope != "admin" {
		return capabilityUnavailable(errors.New("personal configuration owner unavailable"))
	}
	if native == nil && schema.Nullable {
		return nil
	}
	switch schema.Kind {
	case ObjectValue:
		fields, ok := native.(map[string]any)
		if !ok {
			return invalid(errors.New("invalid administrative configuration object"))
		}
		for name, field := range fields {
			if err := validateAdministrativeValue(schema.Properties[name], policy.Properties[name], field); err != nil {
				return err
			}
		}
	case ListValue:
		elements, ok := native.([]any)
		if !ok {
			return invalid(errors.New("invalid administrative configuration list"))
		}
		for _, element := range elements {
			if err := validateAdministrativeValue(*schema.Element, *policy.Element, element); err != nil {
				return err
			}
		}
	}
	return nil
}
