package authz

import (
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
)

// accessFixture draws random authorities and targets from small pools so that scopes, ages and
// key restrictions collide often.
type accessFixture struct {
	rng         *rand.Rand
	agencies    []uuid.UUID
	units       []uuid.UUID
	collections []uuid.UUID
	now         time.Time
}

func newAccessFixture(seed int64) *accessFixture {
	f := &accessFixture{rng: rand.New(rand.NewSource(seed)), now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	for range 3 {
		f.agencies = append(f.agencies, uuid.New())
		f.units = append(f.units, uuid.New())
		f.collections = append(f.collections, uuid.New())
	}
	return f
}

func (f *accessFixture) pick(pool []uuid.UUID) []string {
	var out []string
	for _, id := range pool {
		if f.rng.Intn(2) == 0 {
			out = append(out, id.String())
		}
	}
	if len(out) == 0 {
		out = append(out, pool[f.rng.Intn(len(pool))].String())
	}
	return out
}

func (f *accessFixture) scope(kinds ...string) Scope {
	kind := kinds[f.rng.Intn(len(kinds))]
	switch kind {
	case "agency":
		return Scope{Kind: kind, References: f.pick(f.agencies)}
	case "unit":
		return Scope{Kind: kind, References: f.pick(f.units)}
	case "unitCollection":
		return Scope{Kind: kind, References: f.pick(f.collections)}
	case "account":
		return Scope{Kind: kind, References: []string{uuid.NewString()}}
	}
	return Scope{Kind: kind}
}

func (f *accessFixture) age() *int64 {
	if f.rng.Intn(2) == 0 {
		return nil
	}
	seconds := int64([]int{0, 30, 60, 600}[f.rng.Intn(4)])
	return &seconds
}

func (f *accessFixture) authority() Authority {
	principal := uuid.New()
	authority := Authority{PrincipalID: principal.String(), Revision: 1, Active: true}
	actions := []string{"message.read", "message.detail", "without-unit.read"}
	for range 1 + f.rng.Intn(4) {
		action := actions[f.rng.Intn(len(actions))]
		kinds := []string{"all", "agency", "unit", "unitCollection"}
		if action == "without-unit.read" {
			kinds = []string{"all", "agency"}
		}
		authority.Grants = append(authority.Grants, Grant{ID: uuid.NewString(), SubjectID: principal.String(),
			SubjectKind: "principal", Actions: []string{action}, Scope: f.scope(kinds...), MaxAgeSeconds: f.age()})
	}
	if f.rng.Intn(2) == 0 {
		key := &Restrictions{MaxAgeSeconds: f.age()}
		for _, action := range actions {
			if f.rng.Intn(3) != 0 {
				key.Actions = append(key.Actions, action)
			}
		}
		if len(key.Actions) == 0 {
			key.Actions = []string{"message.read"}
		}
		for range 1 + f.rng.Intn(3) {
			key.Scopes = append(key.Scopes, f.scope("all", "agency", "unit", "unitCollection", "account"))
		}
		authority.Key = key
	}
	return authority
}

func (f *accessFixture) target() Target {
	anchor := f.now.Add(-time.Duration([]int{0, 29, 30, 31, 60, 61, 599, 600, 601}[f.rng.Intn(9)]) * time.Second)
	target := Target{AgencyID: f.agencies[f.rng.Intn(3)], AgeAnchor: anchor}
	if f.rng.Intn(3) == 0 {
		target.WithoutUnit = true
		return target
	}
	target.UnitID = f.units[f.rng.Intn(3)]
	for _, id := range f.collections {
		if f.rng.Intn(2) == 0 {
			target.UnitCollections = append(target.UnitCollections, id)
		}
	}
	return target
}

// TestMessageAccessMatchesAuthorize checks that the store-neutral eligibility equals the
// per-target decision for random grants, keys, ages and targets, including the boundary
// where the age anchor equals the cutoff.
func TestMessageAccessMatchesAuthorize(t *testing.T) {
	t.Parallel()
	f := newAccessFixture(7)
	eligible := 0
	for i := range 6000 {
		authority := f.authority()
		access, err := NewMessageAccess(authority, f.now)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		for range 4 {
			target := f.target()
			wantRead := Authorize(authority, "message.read", nil, target, f.now) == nil
			if target.WithoutUnit {
				wantRead = wantRead && Authorize(authority, "without-unit.read", nil, target, f.now) == nil
			}
			wantDetail := wantRead && Authorize(authority, "message.detail", nil, target, f.now) == nil
			if got := access.ReadEligible(target); got != wantRead {
				t.Fatalf("case %d read: got %v want %v\nauthority %+v\ntarget %+v", i, got, wantRead, authority, target)
			}
			if got := access.DetailEligible(target); got != wantDetail {
				t.Fatalf("case %d detail: got %v want %v\nauthority %+v\ntarget %+v", i, got, wantDetail, authority, target)
			}
			if wantRead {
				eligible++
			}
		}
	}
	if eligible < 500 {
		t.Fatalf("fixture too sparse: %d eligible targets", eligible)
	}
}

func TestMessageAccessAgeBoundaryIsInclusive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 8, 12, 1, 0, 0, time.UTC)
	principal := uuid.New()
	agency := uuid.New()
	seconds := int64(30)
	authority := Authority{PrincipalID: principal.String(), Revision: 1, Active: true, Grants: []Grant{{
		ID: uuid.NewString(), SubjectID: principal.String(), SubjectKind: "principal",
		Actions: []string{"message.read"}, Scope: Scope{Kind: "all"}, MaxAgeSeconds: &seconds}}}
	access, err := NewMessageAccess(authority, now)
	if err != nil {
		t.Fatal(err)
	}
	target := Target{AgencyID: agency, UnitID: uuid.New(), AgeAnchor: now.Add(-30 * time.Second)}
	if !access.ReadEligible(target) {
		t.Fatal("anchor exactly at the cutoff must be eligible")
	}
	target.AgeAnchor = target.AgeAnchor.Add(-time.Microsecond)
	if access.ReadEligible(target) {
		t.Fatal("anchor before the cutoff must not be eligible")
	}
}

func TestMessageAccessRejectsInactiveAndMalformed(t *testing.T) {
	t.Parallel()
	now := time.Now()
	if _, err := NewMessageAccess(Authority{}, now); err != ErrForbidden {
		t.Fatalf("inactive authority: %v", err)
	}
	principal := uuid.New()
	bad := Authority{PrincipalID: principal.String(), Revision: 1, Active: true, Grants: []Grant{{
		ID: uuid.NewString(), SubjectID: principal.String(), SubjectKind: "principal",
		Actions: []string{"message.read"}, Scope: Scope{Kind: "agency"}}}}
	if _, err := NewMessageAccess(bad, now); err != ErrInvalidInput {
		t.Fatalf("malformed grant: %v", err)
	}
}

func TestReadableFieldGroupsIntersectKey(t *testing.T) {
	t.Parallel()
	principal := uuid.New()
	authority := Authority{PrincipalID: principal.String(), Revision: 1, Active: true, FieldPermissions: []FieldPermission{{
		ID: uuid.NewString(), Revision: 1, SubjectID: principal.String(), SubjectKind: "principal",
		Fields: []string{"payload", "unit", "address"}}}}
	groups, err := ReadableFieldGroups(authority)
	if err != nil || len(groups) != 3 || groups[0] != "payload" || groups[1] != "address" || groups[2] != "unit" {
		t.Fatalf("groups = %v, %v", groups, err)
	}
	authority.Key = &Restrictions{Actions: []string{"message.read"}, Scopes: []Scope{{Kind: "all"}}, Fields: []string{"unit"}}
	groups, err = ReadableFieldGroups(authority)
	if err != nil || len(groups) != 1 || groups[0] != "unit" {
		t.Fatalf("key groups = %v, %v", groups, err)
	}
}

func TestSavedViewManageGlobalIsAllScopeOnly(t *testing.T) {
	t.Parallel()
	if !ValidAction("saved-view.manage-global") || !ActionAllowsScope("saved-view.manage-global", "all") ||
		ActionAllowsScope("saved-view.manage-global", "agency") {
		t.Fatal("saved-view.manage-global must accept only the all scope")
	}
}
