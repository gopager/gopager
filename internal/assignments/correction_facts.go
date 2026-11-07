package assignments

import (
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
)

// correctionFacts folds the per-target changes and Network moves of a batch into one
// assignment.changed fact per Transmission, so the log grows with affected Transmissions rather
// than with their targets. Facts keep the order in which their Transmission first appears (changes
// before moves); targets keep the change order, which the repository returns by ascending
// position. A moved Transmission's fact names its new Network and the previous one, and has no
// targets when none changed.
func correctionFacts(changes []CorrectionChange, moves []NetworkMove, occurred time.Time) []events.CorrectionFact {
	var facts []events.CorrectionFact
	index := make(map[uuid.UUID]int, len(changes)+len(moves))
	fact := func(transmission, message, network uuid.UUID) *events.CorrectionFact {
		i, seen := index[transmission]
		if !seen {
			i = len(facts)
			index[transmission] = i
			facts = append(facts, events.CorrectionFact{
				ID: uuid.New(), Type: "assignment.changed",
				Aggregate:  events.EntityRef{Type: "message", ID: message},
				OccurredAt: occurred, TransmissionID: transmission,
				MessageID: message, NetworkID: network, Reason: "corrected",
			})
		}
		return &facts[i]
	}
	for _, change := range changes {
		current := fact(change.TransmissionID, change.MessageID, change.NetworkID)
		current.Revision = max(current.Revision, change.MessageRev)
		current.Targets = append(current.Targets, events.CorrectionTarget{
			Position: change.TargetPosition, Before: change.Before, After: change.After})
	}
	for _, move := range moves {
		current := fact(move.TransmissionID, move.MessageID, move.To)
		from := move.From
		current.NetworkID, current.PreviousNetworkID = move.To, &from
		current.Revision = max(current.Revision, move.MessageRev)
	}
	return facts
}
