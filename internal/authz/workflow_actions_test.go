package authz

import "testing"

// TestWorkflowActionsAcceptWorkflowScopes expects each Workflow action to be grantable on all,
// an Agency or a Unit, and not on a Unit collection or an account.
func TestWorkflowActionsAcceptWorkflowScopes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"read", "create", "update", "delete", "preview", "activate", "reprocess"} {
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
}
