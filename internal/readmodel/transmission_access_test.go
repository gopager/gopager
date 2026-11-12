package readmodel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// detailStub answers every Detail with one fixed record or error.
type detailStub struct {
	Store
	record DetailRecord
	err    error
}

func (s detailStub) Detail(context.Context, platform.TransactionContext, DetailQuery) (DetailRecord, error) {
	return s.record, s.err
}

func TestCheckTransmissionReadableFollowsTheDetail(t *testing.T) {
	t.Parallel()
	member, hidden := uuid.New(), uuid.New()
	record := DetailRecord{FeedRecord: sampleRecord(), Transmissions: []TransmissionRecord{{ID: member}}}
	id := uuid.NewString()
	grant := func(actions ...string) authz.Authority {
		return authz.Authority{PrincipalID: id, Revision: 1, Active: true, Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: id,
			SubjectKind: "principal", Actions: actions, Scope: authz.Scope{Kind: "all"}}}}
	}
	now := time.Now()
	for name, test := range map[string]struct {
		service      *Service
		authority    authz.Authority
		transmission uuid.UUID
		want         error
	}{
		"listed in the detail": {&Service{store: detailStub{record: record}}, grant("message.read", "message.detail"), member, nil},
		"not listed":           {&Service{store: detailStub{record: record}}, grant("message.read", "message.detail"), hidden, authz.ErrNotFound},
		"no detail grant":      {&Service{store: detailStub{record: record}}, grant("message.read"), member, authz.ErrNotFound},
		"no appearance":        {&Service{store: detailStub{err: authz.ErrNotFound}}, grant("message.read", "message.detail"), member, authz.ErrNotFound},
		"no grant at all":      {&Service{store: detailStub{record: record}}, authz.Authority{PrincipalID: id, Revision: 1, Active: true}, member, authz.ErrNotFound},
		"invalid transmission": {&Service{store: detailStub{record: record}}, grant("message.read", "message.detail"), uuid.Nil, authz.ErrInvalidInput},
	} {
		_, err := test.service.CheckTransmissionReadable(context.Background(), stubTx{}, test.authority, now, record.MessageID, test.transmission)
		if !errors.Is(err, test.want) || (test.want == nil && err != nil) {
			t.Errorf("%s: err = %v, want %v", name, err, test.want)
		}
	}
}

func TestCheckTransmissionReadableReportsTheVisibleTargets(t *testing.T) {
	t.Parallel()
	member := uuid.New()
	record := DetailRecord{FeedRecord: sampleRecord(), Transmissions: []TransmissionRecord{{ID: member,
		Targets: []TargetRecord{{Position: 0}, {Position: 2}}}}}
	id := uuid.NewString()
	authority := authz.Authority{PrincipalID: id, Revision: 1, Active: true, Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: id,
		SubjectKind: "principal", Actions: []string{"message.read", "message.detail"}, Scope: authz.Scope{Kind: "all"}}}}
	positions, err := (&Service{store: detailStub{record: record}}).CheckTransmissionReadable(context.Background(), stubTx{},
		authority, time.Now(), record.MessageID, member)
	if err != nil || len(positions) != 2 || positions[0] != 0 || positions[1] != 2 {
		t.Fatalf("positions = %v, err = %v", positions, err)
	}
}
