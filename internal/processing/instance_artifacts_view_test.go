package processing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectConfigurationFieldRedactsSecretAndUserScope(t *testing.T) {
	schema := FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{
		"public":   {Kind: StringValue},
		"secret":   {Kind: StringValue},
		"personal": {Kind: StringValue},
		"missing":  {Kind: StringValue, Optional: true},
	}}
	publicSecret := false
	secret := true
	policy := ConfigurationPolicy{
		Scope: "admin", Secret: &publicSecret,
		Properties: map[string]ConfigurationPolicy{
			"public":   {Scope: "admin", Secret: &publicSecret},
			"secret":   {Scope: "admin", Secret: &secret},
			"personal": {Scope: "user", Secret: &publicSecret},
			"missing":  {Scope: "admin", Secret: &publicSecret},
		},
	}
	fields := map[string]any{
		"public": "visible", "secret": "hidden-secret", "personal": "hidden-personal",
	}
	view, err := projectConfigurationField(schema, policy, fields, true)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "hidden-secret") || strings.Contains(string(encoded), "hidden-personal") {
		t.Fatalf("restricted configuration leaked: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"visible"`) || !view.Properties["secret"].Redacted ||
		!view.Properties["personal"].Redacted || view.Properties["missing"].Configured {
		t.Fatalf("unexpected configuration projection: %s", encoded)
	}
}

func TestProjectConfigurationListRedactsSensitiveElementCounts(t *testing.T) {
	secret := true
	schema := FieldSchema{Kind: ListValue, Element: &FieldSchema{Kind: StringValue}}
	policy := ConfigurationPolicy{Scope: "admin", Secret: new(bool),
		Element: &ConfigurationPolicy{Scope: "admin", Secret: &secret}}
	view, err := projectConfigurationField(schema, policy, []any{"one", "two"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Redacted || len(view.Items) != 0 {
		t.Fatalf("sensitive list shape was disclosed: %+v", view)
	}
}
