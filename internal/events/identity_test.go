package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Identity facts")
}

var _ = Describe("The classified identity envelope", func() {
	var fact events.IdentityFact
	var now time.Time

	BeforeEach(func() {
		now = time.Date(2026, 10, 26, 14, 0, 0, 123456000, time.UTC)
		fact = events.IdentityFact{
			ID: uuid.New(), Type: events.PrincipalAccessChanged,
			Aggregate: events.EntityRef{Type: "account", ID: uuid.New()}, Revision: 9007199254740993,
			OccurredAt: now, ChangeKind: events.GrantsChanged,
		}
	})

	It("serializes revisions losslessly and retains only the classified change data", func() {
		envelope, err := fact.Envelope(now)
		Expect(err).NotTo(HaveOccurred())
		data, err := json.Marshal(envelope)
		Expect(err).NotTo(HaveOccurred())
		var wire map[string]json.RawMessage
		Expect(json.Unmarshal(data, &wire)).To(Succeed())
		Expect(string(wire["aggregateRevision"])).To(Equal(`"9007199254740993"`))
		Expect(string(wire["classification"])).To(Equal(`"authorization"`))
		Expect(string(wire["replayMode"])).To(Equal(`"normal"`))
		Expect(string(wire["data"])).To(Equal(`{"changeKind":"grants-changed"}`))
		Expect(wire).NotTo(HaveKey("causationId"))
	})

	It("rejects unknown, malformed and higher-precision fact input explicitly", func() {
		for _, change := range []func(*events.IdentityFact){
			func(f *events.IdentityFact) { f.ID = uuid.Nil },
			func(f *events.IdentityFact) { f.Aggregate.ID = uuid.Nil },
			func(f *events.IdentityFact) { f.Aggregate.Type = "arbitrary" },
			func(f *events.IdentityFact) { f.Revision = 0 },
			func(f *events.IdentityFact) { f.Type = "message.created" },
			func(f *events.IdentityFact) { f.ChangeKind = "private-user-profile" },
			func(f *events.IdentityFact) { f.OccurredAt = time.Time{} },
			func(f *events.IdentityFact) { f.OccurredAt = now.Add(time.Nanosecond) },
			func(f *events.IdentityFact) { f.Type = events.PrincipalDeleted },
			func(f *events.IdentityFact) { invalid := uuid.Nil; f.CausationID = &invalid },
		} {
			candidate := fact
			change(&candidate)
			_, err := candidate.Envelope(now)
			Expect(err).To(MatchError(events.ErrInvalidFact))
		}
		_, err := fact.Envelope(time.Time{})
		Expect(err).To(MatchError(events.ErrInvalidFact))
	})
})
