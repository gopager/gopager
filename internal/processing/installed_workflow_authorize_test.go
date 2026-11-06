package processing

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// deadlineProbeOwner records whether the context handed to AuthorizeInstalled carries a deadline.
type deadlineProbeOwner struct {
	*installedOwnerFixture
	sawDeadline bool
	calls       int
}

func (o *deadlineProbeOwner) AuthorizeInstalled(ctx context.Context, id uuid.UUID, revision int64,
	call InvocationContext, input map[string]FieldValue) (InvocationEnvelope, []Dependency, bool, error) {
	o.calls++
	_, deadline := ctx.Deadline()
	o.sawDeadline = o.sawDeadline || deadline
	return o.installedOwnerFixture.AuthorizeInstalled(ctx, id, revision, call, input)
}

// TestInstalledAuthorizationIsNotChargedToTheBuiltinComputeAllowance pins that the authorization
// transaction of an installed step runs on the caller's context: it is database I/O, whose latency
// under load must not exhaust the 1 s built-in compute allowance and surface as STORAGE_UNAVAILABLE.
func TestInstalledAuthorizationIsNotChargedToTheBuiltinComputeAllowance(t *testing.T) {
	t.Parallel()
	owner := &deadlineProbeOwner{installedOwnerFixture: newInstalledOwnerFixture(t)}
	_, source := sourceFixture(t)
	definition := installedDefinition(owner.installedOwnerFixture, "ok", FieldRef{Namespace: "source", Path: []string{"text"}})
	program, err := CompileWorkflowInstalled(context.Background(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.EvaluateInstalled(context.Background(), source, owner)
	if err != nil || !result.Selected || owner.calls == 0 {
		t.Fatalf("installed run failed: %+v, %v (authorizations %d)", result, err, owner.calls)
	}
	if owner.sawDeadline {
		t.Fatal("authorization ran under the built-in compute allowance deadline")
	}
}
