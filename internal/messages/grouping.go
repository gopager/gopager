package messages

import (
	"bytes"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/protocols"
)

var ErrInvalidGrouping = errors.New("invalid Grouping input")

const MaxGroupingWindowSeconds int64 = 9007199254740991

type Policy struct {
	Revision      int64
	WindowSeconds int64
}

type Member struct {
	TransmissionID uuid.UUID
	ObservedAt     time.Time
	Identity       protocols.PayloadIdentity
}

type ExistingMessage struct {
	ID      uuid.UUID
	Members []Member
}

type Decision struct {
	Kind            string
	MessageID       uuid.UUID
	AbsorbedMessage []uuid.UUID
	Joined          bool
	PolicyRevision  int64
	MessageRevision int64
	LateJoin        bool
}

// Decide applies exact Payload identity and the active inclusive whole-second window.
func Decide(policy Policy, transmission Member, existing []ExistingMessage) (Decision, error) {
	if policy.Revision <= 0 || policy.WindowSeconds < 0 || policy.WindowSeconds > MaxGroupingWindowSeconds ||
		transmission.TransmissionID == uuid.Nil || !validObservation(transmission.ObservedAt) ||
		!validIdentity(transmission.Identity) {
		return Decision{}, ErrInvalidGrouping
	}
	if policy.WindowSeconds == 0 {
		return Decision{Kind: "created", MessageID: uuid.New(), PolicyRevision: policy.Revision}, nil
	}

	matches := make(map[uuid.UUID]bool)
	seenMessages := make(map[uuid.UUID]bool)
	seenTransmissions := make(map[uuid.UUID]bool)
	for _, message := range existing {
		if message.ID == uuid.Nil || seenMessages[message.ID] {
			return Decision{}, ErrInvalidGrouping
		}
		seenMessages[message.ID] = true
		for _, member := range message.Members {
			if member.TransmissionID == uuid.Nil || seenTransmissions[member.TransmissionID] ||
				!validObservation(member.ObservedAt) || !validIdentity(member.Identity) {
				return Decision{}, ErrInvalidGrouping
			}
			seenTransmissions[member.TransmissionID] = true
			if member.TransmissionID == transmission.TransmissionID {
				return Decision{}, ErrInvalidGrouping
			}
			if sameIdentity(member.Identity, transmission.Identity) &&
				withinWindow(member.ObservedAt, transmission.ObservedAt, policy.WindowSeconds) {
				matches[message.ID] = true
			}
		}
	}
	if len(matches) == 0 {
		return Decision{Kind: "created", MessageID: uuid.New(), PolicyRevision: policy.Revision}, nil
	}
	ids := make([]uuid.UUID, 0, len(matches))
	for id := range matches {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })
	decision := Decision{Kind: "joined", MessageID: ids[0], Joined: true, PolicyRevision: policy.Revision}
	if len(ids) > 1 {
		decision.Kind = "merged"
		decision.AbsorbedMessage = append([]uuid.UUID(nil), ids[1:]...)
	}
	return decision, nil
}

func validIdentity(identity protocols.PayloadIdentity) bool {
	return identity.Form != ""
}

func validObservation(value time.Time) bool {
	return !value.IsZero() && value.Nanosecond() == 0 && value.Year() >= 0 && value.Year() <= 9999
}

func sameIdentity(left, right protocols.PayloadIdentity) bool {
	return left.Form == right.Form && left.Subtype == right.Subtype && bytes.Equal(left.Value, right.Value)
}

func withinWindow(left, right time.Time, window int64) bool {
	leftSecond, rightSecond := left.Unix(), right.Unix()
	var difference int64
	if leftSecond >= rightSecond {
		difference = leftSecond - rightSecond
	} else {
		difference = rightSecond - leftSecond
	}
	return difference <= window
}
