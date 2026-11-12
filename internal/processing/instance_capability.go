package processing

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// Plugin capabilities a manifest may declare.
const (
	// CapabilityProcessing is the pure Workflow step; it is the default when a manifest names none.
	CapabilityProcessing = "processing"
	// CapabilityDelivery is a Delivery target that sends rendered output to an external provider.
	CapabilityDelivery = "delivery"
)

// sdkMinorDelivery is the SDK minor version that introduced the Delivery capability.
const sdkMinorDelivery = 1

// Bounds of a Delivery declaration.
const (
	maxDeliveryInputs       = 64
	maxDeliveryDependencies = 16
	maxSendsPerMinute       = 100000
	maxDependencyText       = 256
)

// DeliveryInputDeclaration is one typed input a Delivery target accepts per Unit or Subscription.
// MaxBytes bounds the encoded value; zero selects the Delivery default.
type DeliveryInputDeclaration struct {
	Name     string    `json:"name"`
	Kind     ValueKind `json:"kind"`
	Required bool      `json:"required,omitempty"`
	MaxBytes int       `json:"maxBytes,omitempty"`
}

// DeliveryDependency classifies something a Delivery target needs at run time: kind is one of
// network, secret, device or other. The description must not reveal a Secret or destination.
type DeliveryDependency struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// DeliveryCapability is the manifest's declaration of a Delivery target: the output kinds it can
// consume, its typed inputs, its dependencies, an optional own sending limit and whether the
// provider deduplicates by Delivery ID.
type DeliveryCapability struct {
	OutputKinds    []string                   `json:"outputKinds"`
	Inputs         []DeliveryInputDeclaration `json:"inputs,omitempty"`
	Dependencies   []DeliveryDependency       `json:"dependencies,omitempty"`
	SendsPerMinute int                        `json:"sendsPerMinute,omitempty"`
	Idempotent     bool                       `json:"idempotent,omitempty"`
}

// PluginResources is informational resource guidance shown by inspection and never enforced.
type PluginResources struct {
	MemoryMiB int    `json:"memoryMiB,omitempty"`
	Note      string `json:"note,omitempty"`
}

// ErrCapabilityUndeclared is matched (errors.Is) by a CapabilityUndeclaredError: a plugin asked for
// Core access its capability does not grant.
var ErrCapabilityUndeclared = errors.New("plugin capability undeclared")

// CapabilityUndeclaredError reports that a manifest of the named capability declares access that
// capability does not offer, for example requiredOperations on a Delivery manifest. Its message is
// safe to show; the HTTP layer maps it to CAPABILITY_UNDECLARED.
type CapabilityUndeclaredError struct {
	// Capability is the capability whose manifest overreached.
	Capability string
}

// Error returns a safe message naming only the capability.
func (e *CapabilityUndeclaredError) Error() string {
	return "the " + e.Capability + " capability does not grant the declared access"
}

// Is reports a match with ErrCapabilityUndeclared.
func (e *CapabilityUndeclaredError) Is(target error) bool { return target == ErrCapabilityUndeclared }

// capabilityKind returns the manifest's capability, defaulting an absent one to processing.
func (m InstalledManifest) capabilityKind() (string, error) {
	switch m.Capability {
	case "", CapabilityProcessing:
		return CapabilityProcessing, nil
	case CapabilityDelivery:
		return CapabilityDelivery, nil
	}
	return "", invalid(errors.New("unknown plugin capability"))
}

// EffectiveCapability returns the effective capability of the manifest, processing when none is declared.
func (m InstalledManifest) EffectiveCapability() string {
	kind, err := m.capabilityKind()
	if err != nil {
		return ""
	}
	return kind
}

// requireProcessing refuses artifacts that are not pure Workflow steps.
func (m InstalledManifest) requireProcessing() error {
	if m.EffectiveCapability() != CapabilityProcessing {
		return capabilityUnavailable(errors.New("plugin is not a processing capability"))
	}
	return nil
}

// validateCapabilityDeclaration checks the members that depend on the capability: the processing
// rules are unchanged, a Delivery manifest must be pure-less, preview-less and declare only its
// Delivery object.
func validateCapabilityDeclaration(m InstalledManifest, capability string) error {
	if capability == CapabilityProcessing {
		if m.Delivery != nil {
			return invalid(errors.New("processing manifest has a delivery declaration"))
		}
		if !m.Pure || len(m.RequiredCoreFields) != 0 || len(m.RequiredOperations) != 0 || len(m.HostCapabilities) != 0 {
			return capabilityUnavailable(errors.New("installed command requires unavailable capabilities"))
		}
		return nil
	}
	if len(m.RequiredCoreFields) != 0 || len(m.RequiredOperations) != 0 {
		return &CapabilityUndeclaredError{Capability: CapabilityDelivery}
	}
	if m.SDKMinor != sdkMinorDelivery || m.Pure || m.SupportsPreview || len(m.InputSchema) != 0 ||
		len(m.ResultSchema) != 0 || len(m.HostCapabilities) != 0 || m.Delivery == nil {
		return invalid(errors.New("invalid delivery manifest shape"))
	}
	if m.Resources != nil && (m.Resources.MemoryMiB < 0 || len(m.Resources.Note) > maxDependencyText ||
		!utf8.ValidString(m.Resources.Note)) {
		return invalid(errors.New("invalid plugin resources"))
	}
	return m.Delivery.validate()
}

// deliveryOutputKinds are the output kinds a layout can render for a Delivery.
var deliveryOutputKinds = []string{LayoutText, LayoutMarkdown, LayoutJSON}

// deliveryInputKinds are the scalar kinds a Delivery input may declare.
var deliveryInputKinds = []ValueKind{StringValue, BooleanValue, IntegerValue, NumberValue, TimestampValue, BytesValue}

// deliveryDependencyKinds are the dependency classifications a manifest may use.
var deliveryDependencyKinds = []string{"network", "secret", "device", "other"}

// validate checks the Delivery declaration's bounds and closed vocabularies.
func (d DeliveryCapability) validate() error {
	if len(d.OutputKinds) == 0 || len(d.OutputKinds) > len(deliveryOutputKinds) || d.SendsPerMinute < 0 ||
		d.SendsPerMinute > maxSendsPerMinute || len(d.Inputs) > maxDeliveryInputs ||
		len(d.Dependencies) > maxDeliveryDependencies {
		return invalid(errors.New("invalid delivery declaration"))
	}
	seenKinds := map[string]bool{}
	for _, kind := range d.OutputKinds {
		if !slices.Contains(deliveryOutputKinds, kind) || seenKinds[kind] {
			return invalid(errors.New("invalid delivery output kind"))
		}
		seenKinds[kind] = true
	}
	names := map[string]bool{}
	for _, input := range d.Inputs {
		if !validLocalName(input.Name) || names[input.Name] || !slices.Contains(deliveryInputKinds, input.Kind) ||
			input.MaxBytes < 0 || input.MaxBytes > MaxInputBytes {
			return invalid(errors.New("invalid delivery input declaration"))
		}
		names[input.Name] = true
	}
	for _, dependency := range d.Dependencies {
		if !slices.Contains(deliveryDependencyKinds, dependency.Kind) || dependency.Description == "" ||
			len(dependency.Description) > maxDependencyText || !utf8.ValidString(dependency.Description) {
			return invalid(errors.New("invalid delivery dependency"))
		}
	}
	return nil
}

// validateDeliveryConfiguration enforces the declaration rules that make Delivery configuration
// safe to store and merge: the root is administrative; Secrets are string properties outside lists;
// containers are never Secrets; user properties are optional and their subtree stays personal; list
// elements share their list's scope and an administrative list holds no personal property at any
// depth; a nullable administrative container holds no personal property, because an administrative
// null would hide personal values with no unambiguous merge; property names contain no '.' so dotted Secret paths are
// unambiguous.
func validateDeliveryConfiguration(schema FieldSchema, policy ConfigurationPolicy) error {
	if policy.Scope != "admin" {
		return invalid(errors.New("configuration root must be administrative"))
	}
	return validateDeliveryNode(schema, policy, false, "admin")
}

// validateDeliveryNode checks one node; inList reports an ancestor list, parentScope the scope of
// the enclosing container.
func validateDeliveryNode(schema FieldSchema, policy ConfigurationPolicy, inList bool, parentScope string) error {
	if parentScope == "user" && policy.Scope != "user" {
		return invalid(errors.New("administrative property inside a personal container"))
	}
	if policy.Secret != nil && *policy.Secret && (schema.Kind != StringValue || inList) {
		return invalid(errors.New("only string properties outside lists can be Secrets"))
	}
	switch schema.Kind {
	case ObjectValue:
		if schema.Nullable && policy.Scope == "admin" && hasDescendantScope(policy, "user") {
			return invalid(errors.New("nullable administrative container with personal properties"))
		}
		for name, child := range schema.Properties {
			if strings.Contains(name, ".") {
				return invalid(errors.New("configuration property names cannot contain a dot"))
			}
			declaration := policy.Properties[name]
			if declaration.Scope == "user" && !child.Optional {
				return invalid(errors.New("personal properties must be optional"))
			}
			if err := validateDeliveryNode(child, declaration, inList, policy.Scope); err != nil {
				return err
			}
		}
	case ListValue:
		if policy.Element.Scope != policy.Scope {
			return invalid(errors.New("list element scope differs from its list"))
		}
		if policy.Scope == "admin" && hasDescendantScope(policy, "user") {
			return invalid(errors.New("personal property below an administrative list"))
		}
		return validateDeliveryNode(*schema.Element, *policy.Element, true, policy.Scope)
	}
	return nil
}

// validateInstance checks an administrative instance record against the manifest it pins: a
// processing instance carries no target use and no Secrets (their owner is unavailable there); a
// Delivery instance requires a target use, and its Secret properties may hold only valid
// reference strings (syntax only, nothing is resolved).
func (m InstalledManifest) validateInstance(configuration InstanceConfiguration) error {
	if m.EffectiveCapability() == CapabilityProcessing {
		if configuration.TargetUse != nil {
			return invalid(errors.New("processing instance carries a target use"))
		}
		if hasSecretPolicy(m.ConfigurationPolicy) {
			return capabilityUnavailable(errors.New("instance Secret owner unavailable"))
		}
		return validateAdministrativeConfiguration(m.ConfigurationSchema, m.ConfigurationPolicy, configuration.Configuration)
	}
	if configuration.TargetUse == nil {
		return invalid(errors.New("delivery instance requires a target use"))
	}
	if err := validateAdministrativeConfiguration(m.ConfigurationSchema, m.ConfigurationPolicy,
		configuration.Configuration); err != nil {
		return err
	}
	_, err := m.ConfigurationSecretReferences(configuration.Configuration)
	return err
}

// hasDescendantScope reports whether any node below the policy's own node has the scope.
func hasDescendantScope(policy ConfigurationPolicy, scope string) bool {
	for _, child := range policy.Properties {
		if hasScope(child, scope) {
			return true
		}
	}
	return policy.Element != nil && hasScope(*policy.Element, scope)
}
