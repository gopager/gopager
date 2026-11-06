package authz

import "testing"

// TestWorkflowActionsReplaceRuleActions expects the Workflow actions to be named workflow.*
// (rule.* is ambiguous next to address-rule.*), the old names to be unknown, and each new name
// to accept the scopes its predecessor accepted.
func TestWorkflowActionsReplaceRuleActions(t *testing.T) {
	for _, name := range []string{"read", "create", "update", "delete", "preview", "activate", "reprocess"} {
		if ValidAction("rule." + name) {
			t.Fatalf("rule.%s is still a valid action", name)
		}
		action := "workflow." + name
		if !ValidAction(action) {
			t.Fatalf("%s is not a valid action", action)
		}
		for kind, want := range map[string]bool{"all": true, "agency": true, "unit": true, "unitCollection": false, "account": false} {
			if got := ActionAllowsScope(action, kind); got != want {
				t.Fatalf("ActionAllowsScope(%s, %s)=%t want %t", action, kind, got, want)
			}
		}
	}
	if !ValidAction("presentation.manage") || !ValidAction("address-rule.read") {
		t.Fatal("presentation.manage or address-rule.read was lost")
	}
}
