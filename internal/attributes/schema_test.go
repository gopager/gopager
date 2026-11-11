package attributes

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func alarmSchema() Schema {
	return Schema{
		Key: "alarm", Label: "Alarm", Subject: SubjectMessage, Agencies: []uuid.UUID{uuid.New()},
		Fields: map[string]Field{
			"keyword": {Kind: processing.StringValue, Label: "Keyword", Icon: "tag"},
			"level":   {Kind: processing.IntegerValue},
			"location": {Kind: processing.ObjectValue, Properties: map[string]Field{
				"street": {Kind: processing.StringValue},
				"lat":    {Kind: processing.NumberValue, Optional: true},
			}},
			"vehicles": {Kind: processing.ListValue, Element: &Field{Kind: processing.StringValue}},
		},
	}
}

func code(err error) platform.ErrorCode {
	if err == nil {
		return ""
	}
	return platform.FailureCode(err)
}

func TestValidKeyAndName(t *testing.T) {
	for _, key := range []string{"a", "alarm", "alarm-2", strings.Repeat("a", MaxKeyLength)} {
		if !ValidKey(key) {
			t.Errorf("ValidKey(%q) = false", key)
		}
	}
	for _, key := range []string{"", "Alarm", "2alarm", "-a", "a_b", "a.b", strings.Repeat("a", MaxKeyLength+1)} {
		if ValidKey(key) {
			t.Errorf("ValidKey(%q) = true", key)
		}
	}
	for _, name := range []string{"a", "streetName", "lat_2"} {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false", name)
		}
	}
	for _, name := range []string{"", "_a", "1a", "a-b", "a.b", "ä"} {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true", name)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := alarmSchema().Validate(); err != nil {
		t.Fatalf("valid schema: %v", err)
	}
	cases := map[string]func(*Schema){
		"bad key":            func(s *Schema) { s.Key = "Alarm" },
		"bad subject":        func(s *Schema) { s.Subject = "gateway" },
		"empty label":        func(s *Schema) { s.Label = " " },
		"no fields":          func(s *Schema) { s.Fields = nil },
		"bad field name":     func(s *Schema) { s.Fields["a.b"] = Field{Kind: processing.StringValue} },
		"top-level optional": func(s *Schema) { s.Fields["x"] = Field{Kind: processing.StringValue, Optional: true} },
		"bad icon":           func(s *Schema) { s.Fields["x"] = Field{Kind: processing.StringValue, Icon: "a b"} },
		"bad kind":           func(s *Schema) { s.Fields["x"] = Field{Kind: "map"} },
		"list without element": func(s *Schema) {
			s.Fields["x"] = Field{Kind: processing.ListValue}
		},
		"bad property name": func(s *Schema) {
			s.Fields["x"] = Field{Kind: processing.ObjectValue, Properties: map[string]Field{"a-b": {Kind: processing.StringValue}}}
		},
		"deprecated element": func(s *Schema) {
			s.Fields["x"] = Field{Kind: processing.ListValue, Element: &Field{Kind: processing.StringValue, Deprecated: true}}
		},
		"no agency":        func(s *Schema) { s.Agencies = nil },
		"nil agency":       func(s *Schema) { s.Agencies = []uuid.UUID{uuid.Nil} },
		"duplicate agency": func(s *Schema) { s.Agencies = append(s.Agencies, s.Agencies[0]) },
	}
	for name, mutate := range cases {
		schema := alarmSchema()
		mutate(&schema)
		if got := code(schema.Validate()); got != platform.CodeInvalidInput {
			t.Errorf("%s: code %q, want INVALID_INPUT", name, got)
		}
	}
}

func TestResolve(t *testing.T) {
	schema := alarmSchema()
	field, typ, deprecated, ok := schema.Resolve([]string{"location", "street"})
	if !ok || deprecated || field.Kind != processing.StringValue || !typ.Optional {
		t.Fatalf("location.street: %+v %+v %v %v", field, typ, deprecated, ok)
	}
	for _, path := range [][]string{nil, {"missing"}, {"keyword", "x"}, {"location", "missing"}, {"vehicles", "x"}} {
		if _, _, _, ok := schema.Resolve(path); ok {
			t.Errorf("Resolve(%v) succeeded", path)
		}
	}
	location := schema.Fields["location"]
	location.Deprecated = true
	schema.Fields["location"] = location
	if _, _, deprecated, _ := schema.Resolve([]string{"location", "street"}); !deprecated {
		t.Error("deprecation of a parent is not inherited")
	}
	schema = alarmSchema()
	schema.Deprecated = true
	if _, _, deprecated, _ := schema.Resolve([]string{"keyword"}); !deprecated {
		t.Error("schema deprecation is not inherited")
	}
	if typ, ok := schema.FieldSchema("level"); !ok || !typ.Optional || typ.Kind != processing.IntegerValue {
		t.Errorf("FieldSchema(level) = %+v, %v", typ, ok)
	}
}

func TestCheckChange(t *testing.T) {
	allowed := map[string]func(*Schema){
		"display metadata": func(s *Schema) {
			s.Label, s.Description = "Alert", "Parsed alarm"
			f := s.Fields["keyword"]
			f.Label, f.Icon, f.Description = "Stichwort", "fire", "text"
			s.Fields["keyword"] = f
		},
		"new top-level field": func(s *Schema) { s.Fields["priority"] = Field{Kind: processing.IntegerValue} },
		"new optional property": func(s *Schema) {
			f := s.Fields["location"]
			f.Properties = map[string]Field{"street": f.Properties["street"], "lat": f.Properties["lat"],
				"city": {Kind: processing.StringValue, Optional: true}}
			s.Fields["location"] = f
		},
		"deprecate": func(s *Schema) {
			f := s.Fields["level"]
			f.Deprecated = true
			s.Fields["level"] = f
			s.Deprecated = true
		},
		"agencies": func(s *Schema) { s.Agencies = []uuid.UUID{uuid.New(), uuid.New()} },
	}
	for name, mutate := range allowed {
		next := alarmSchema()
		mutate(&next)
		if err := CheckChange(alarmSchema(), next); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	rejected := map[string]func(*Schema){
		"key":           func(s *Schema) { s.Key = "other" },
		"subject":       func(s *Schema) { s.Subject = SubjectUnit },
		"remove field":  func(s *Schema) { delete(s.Fields, "level") },
		"change kind":   func(s *Schema) { s.Fields["level"] = Field{Kind: processing.NumberValue} },
		"make nullable": func(s *Schema) { s.Fields["level"] = Field{Kind: processing.IntegerValue, Nullable: true} },
		"required property": func(s *Schema) {
			f := s.Fields["location"]
			f.Properties = map[string]Field{"street": f.Properties["street"], "lat": f.Properties["lat"],
				"city": {Kind: processing.StringValue}}
			s.Fields["location"] = f
		},
		"property optionality": func(s *Schema) {
			f := s.Fields["location"]
			f.Properties = map[string]Field{"street": {Kind: processing.StringValue, Optional: true}, "lat": f.Properties["lat"]}
			s.Fields["location"] = f
		},
		"remove property": func(s *Schema) {
			f := s.Fields["location"]
			f.Properties = map[string]Field{"street": f.Properties["street"]}
			s.Fields["location"] = f
		},
		"element kind": func(s *Schema) {
			s.Fields["vehicles"] = Field{Kind: processing.ListValue, Element: &Field{Kind: processing.IntegerValue}}
		},
	}
	for name, mutate := range rejected {
		next := alarmSchema()
		mutate(&next)
		if got := code(CheckChange(alarmSchema(), next)); got != platform.CodeInvalidInput {
			t.Errorf("%s: code %q, want INVALID_INPUT", name, got)
		}
	}
}
