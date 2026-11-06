package assignments

import (
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
)

// correctionFacts folds the per-target changes of a batch into one assignment.changed fact per
// Transmission, so the log grows with affected Transmissions rather than with their targets. Facts
// keep the order in which their Transmission first appears; targets keep the change order, which
// the repository returns by ascending position.
func correctionFacts(changes []CorrectionChange, occurred time.Time) []events.CorrectionFact {
	var facts []events.CorrectionFact
	index := make(map[uuid.UUID]int, len(changes))
	for _, change := range changes {
		i, seen := index[change.TransmissionID]
		if !seen {
			i = len(facts)
			index[change.TransmissionID] = i
			facts = append(facts, events.CorrectionFact{
				ID: uuid.New(), Type: "assignment.changed",
				Aggregate:  events.EntityRef{Type: "message", ID: change.MessageID},
				OccurredAt: occurred, TransmissionID: change.TransmissionID,
				MessageID: change.MessageID, NetworkID: change.NetworkID, Reason: "corrected",
			})
		}
		facts[i].Revision = max(facts[i].Revision, change.MessageRev)
		facts[i].Targets = append(facts[i].Targets, events.CorrectionTarget{
			Position: change.TargetPosition, Before: change.Before, After: change.After})
	}
	return facts
}
