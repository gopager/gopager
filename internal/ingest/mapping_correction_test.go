package ingest

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/protocols"
)

func TestChangedProtocolsComparesCandidatesPerProtocol(t *testing.T) {
	t.Parallel()
	a, b, flex := uuid.New(), uuid.New(), uuid.New()
	before := []GatewayNetwork{{Identifier: "a", NetworkID: a, Protocol: protocols.POCSAG, Priority: 0},
		{Identifier: "flex", NetworkID: flex, Protocol: protocols.FLEX, Priority: 1},
		{Identifier: "b", NetworkID: b, Protocol: protocols.POCSAG, Priority: 2}}
	for name, test := range map[string]struct {
		after []GatewayNetwork
		want  []protocols.Protocol
	}{
		"unchanged": {before, []protocols.Protocol{}},
		"other protocol between them": {[]GatewayNetwork{{"a", a, protocols.POCSAG, uuid.Nil, 0},
			{"b", b, protocols.POCSAG, uuid.Nil, 1}, {"flex", flex, protocols.FLEX, uuid.Nil, 2}}, []protocols.Protocol{}},
		"swapped POCSAG order": {[]GatewayNetwork{{"a", a, protocols.POCSAG, uuid.Nil, 2},
			{"flex", flex, protocols.FLEX, uuid.Nil, 1}, {"b", b, protocols.POCSAG, uuid.Nil, 0}}, []protocols.Protocol{protocols.POCSAG}},
		"renamed identifier": {[]GatewayNetwork{{"x", a, protocols.POCSAG, uuid.Nil, 0},
			{"flex", flex, protocols.FLEX, uuid.Nil, 1}, {"b", b, protocols.POCSAG, uuid.Nil, 2}}, []protocols.Protocol{protocols.POCSAG}},
		"FLEX removed": {[]GatewayNetwork{{"a", a, protocols.POCSAG, uuid.Nil, 0},
			{"b", b, protocols.POCSAG, uuid.Nil, 1}}, []protocols.Protocol{protocols.FLEX}},
	} {
		if got := changedProtocols(before, test.after); !slices.Equal(got, test.want) {
			t.Fatalf("%s: %v, want %v", name, got, test.want)
		}
	}
}
