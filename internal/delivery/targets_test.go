package delivery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

// TestDescriptorInputsMustBeUniqueScalars proves that a descriptor declaring a composite, unnamed
// or repeated input is rejected as unavailable, since its inputs could never be validated.
func TestDescriptorInputsMustBeUniqueScalars(t *testing.T) {
	t.Parallel()
	instance := processing.InstanceConfiguration{ID: uuid.New(), Revision: 2}
	descriptor := func(inputs ...InputField) TargetDescriptor {
		return TargetDescriptor{Version: DescriptorVersion, InstanceID: instance.ID, InstanceRevision: 2,
			CapabilityKind: CapabilityDelivery, ConfigurationAuthority: "admin", OutputKinds: []string{"text"}, Inputs: inputs}
	}
	valid := descriptor(InputField{Name: "recipient", Kind: processing.StringValue},
		InputField{Name: "priority", Kind: processing.IntegerValue, Required: true, MaxBytes: 16})
	if err := valid.validate(instance); err != nil {
		t.Fatalf("scalar inputs rejected: %v", err)
	}
	for name, inputs := range map[string][]InputField{
		"list":     {{Name: "recipients", Kind: processing.ListValue}},
		"object":   {{Name: "options", Kind: processing.ObjectValue}},
		"null":     {{Name: "nothing", Kind: processing.NullValue}},
		"unnamed":  {{Name: "", Kind: processing.StringValue}},
		"repeated": {{Name: "a", Kind: processing.StringValue}, {Name: "a", Kind: processing.IntegerValue}},
		"negative": {{Name: "a", Kind: processing.StringValue, MaxBytes: -1}},
		"unknown":  {{Name: "a", Kind: "decimal"}},
	} {
		err := descriptor(inputs...).validate(instance)
		if err == nil {
			t.Fatalf("%s input accepted", name)
		}
	}
}
