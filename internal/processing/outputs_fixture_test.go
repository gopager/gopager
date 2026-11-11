package processing

import "errors"

// testOutputSchemaKey is the Attribute schema key of the outputs the tests write.
const testOutputSchemaKey = "t"

// outputFixture declares the fields of the Attribute schema "t" and nothing else.
type outputFixture map[string]FieldSchema

// testOutputs declares the output fields the tests use. Their top-level optionality is forced
// by the compiler, as for real Attribute schemas.
var testOutputs Declarations = outputFixture{
	"address": {Kind: StringValue}, "constant": {Kind: StringValue}, "number": {Kind: IntegerValue},
	"level": {Kind: IntegerValue}, "text": {Kind: StringValue}, "context": {Kind: StringValue},
	"blob": {Kind: StringValue}, "picked": {Kind: StringValue}, "value": {Kind: StringValue},
	"network": {Kind: StringValue}, "bad": {Kind: IntegerValue}, "binary": {Kind: BytesValue},
	"padding": {Kind: StringValue}, "nullable": {Kind: StringValue, Nullable: true},
}

// FieldSchema implements Declarations.
func (f outputFixture) FieldSchema(ref FieldRef) (FieldSchema, error) {
	if ref.Namespace == "attributes" && len(ref.Path) == 2 && ref.Path[0] == "many" {
		return FieldSchema{Kind: StringValue}, nil
	}
	if ref.Namespace != "attributes" || len(ref.Path) != 2 || ref.Path[0] != testOutputSchemaKey {
		return FieldSchema{}, invalid(errors.New("unknown output"))
	}
	schema, ok := f[ref.Path[1]]
	if !ok {
		return FieldSchema{}, invalid(errors.New("unknown output field"))
	}
	return schema, nil
}

// setOutput assigns the field of the test schema in the definition.
func setOutput(definition *Definition, field string, assignment Assignment) {
	if definition.Outputs == nil {
		definition.Outputs = make(map[string]map[string]Assignment)
	}
	if definition.Outputs[testOutputSchemaKey] == nil {
		definition.Outputs[testOutputSchemaKey] = make(map[string]Assignment)
	}
	definition.Outputs[testOutputSchemaKey][field] = assignment
}
