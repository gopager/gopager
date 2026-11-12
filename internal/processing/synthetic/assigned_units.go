package synthetic

import (
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
)

// sameAssignedUnits reports whether every resolution assigns its target position to the Unit the
// target is persisted with, or leaves it unassigned where it is persisted unassigned.
func sameAssignedUnits(resolutions []assignments.Resolution, persisted []*uuid.UUID) bool {
	if len(resolutions) != len(persisted) {
		return false
	}
	for _, resolution := range resolutions {
		if resolution.TargetPosition < 0 || resolution.TargetPosition >= len(persisted) {
			return false
		}
		want := persisted[resolution.TargetPosition]
		if (want == nil) != (resolution.UnitID == nil) || want != nil && *want != *resolution.UnitID {
			return false
		}
	}
	return true
}
