package messages

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/protocols"
)

func TestDecideJoinsAndMergesExactMembershipComponents(t *testing.T) {
	identity := protocols.PayloadIdentity{Form: "text", Value: []byte("page")}
	older := uuid.New()
	newer := uuid.New()
	member := func(id uuid.UUID, second int64) Member {
		return Member{TransmissionID: uuid.New(), ObservedAt: time.Unix(second, 0).UTC(), Identity: identity}
	}
	decision, err := Decide(Policy{Revision: 4, WindowSeconds: 30},
		member(uuid.New(), 25),
		[]ExistingMessage{
			{ID: older, Members: []Member{member(uuid.New(), 0)}},
			{ID: newer, Members: []Member{member(uuid.New(), 50)}},
		})
	if err != nil || decision.Kind != "merged" || decision.MessageID != minUUID(older, newer) ||
		len(decision.AbsorbedMessage) != 1 || !decision.Joined || decision.PolicyRevision != 4 {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestDecideKeepsIdentityAndWindowBoundariesExact(t *testing.T) {
	inputID := uuid.New()
	input := Member{TransmissionID: inputID, ObservedAt: time.Unix(30, 0).UTC(),
		Identity: protocols.PayloadIdentity{Form: "text", Value: []byte("same")}}
	existing := []ExistingMessage{{
		ID: uuid.New(),
		Members: []Member{
			{TransmissionID: uuid.New(), ObservedAt: time.Unix(0, 0).UTC(), Identity: input.Identity},
			{TransmissionID: uuid.New(), ObservedAt: time.Unix(0, 0).UTC(),
				Identity: protocols.PayloadIdentity{Form: "numeric", Value: []byte("same")}},
		},
	}}
	joined, err := Decide(Policy{Revision: 1, WindowSeconds: 30}, input, existing)
	if err != nil || joined.Kind != "joined" {
		t.Fatalf("boundary decision=%+v err=%v", joined, err)
	}
	created, err := Decide(Policy{Revision: 1, WindowSeconds: 29}, input, existing)
	if err != nil || created.Kind != "created" {
		t.Fatalf("outside-window decision=%+v err=%v", created, err)
	}
	disabled, err := Decide(Policy{Revision: 2, WindowSeconds: 0}, input, existing)
	if err != nil || disabled.Kind != "created" {
		t.Fatalf("disabled decision=%+v err=%v", disabled, err)
	}
}

func TestDecideValidatesSafeIntegerGroupingWindow(t *testing.T) {
	input := Member{TransmissionID: uuid.New(), ObservedAt: time.Unix(1, 0).UTC(),
		Identity: protocols.PayloadIdentity{Form: "tone-only"}}
	if _, err := Decide(Policy{Revision: 1, WindowSeconds: MaxGroupingWindowSeconds}, input, nil); err != nil {
		t.Fatalf("rejected maximum safe window: %v", err)
	}
	if _, err := Decide(Policy{Revision: 1, WindowSeconds: MaxGroupingWindowSeconds + 1}, input, nil); err == nil {
		t.Fatal("accepted window above maximum safe integer")
	}
}

func minUUID(left, right uuid.UUID) uuid.UUID {
	if left.String() < right.String() {
		return left
	}
	return right
}
