package assignments

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/protocols"
)

// numericRule builds an explicit valid fixture, not a default production rule.
func numericRule(network uuid.UUID, first, last uint64, functions []uint16) Rule {
	return Rule{ID: uuid.New(), Revision: 1, NetworkID: network, UnitID: uuid.New(),
		Protocol: protocols.POCSAG, Selector: Selector{From: &first, To: &last, Mode: "function-bit", Functions: functions}, Enabled: true}
}

func TestRuleSpecificityUsesSetInclusionNotAddressPriority(t *testing.T) {
	network := uuid.New()
	broad := numericRule(network, 100, 200, nil)
	exact := numericRule(network, 150, 150, []uint16{1})
	if err := ValidateChange(t.Context(), exact, []Rule{broad}); err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	targets := []protocols.Target{
		{Address: protocols.NumericAddress(150), Function: &function},
		{Address: protocols.NumericAddress(150), Function: &function},
		{Address: protocols.NumericAddress(201), Function: &function},
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for _, rules := range [][]Rule{{broad, exact}, {exact, broad}} {
		resolutions, err := Resolve(t.Context(), network, protocols.POCSAG, now, targets, rules)
		if err != nil || len(resolutions) != 3 {
			t.Fatalf("resolution: %v", err)
		}
		for i := 0; i < 2; i++ {
			if resolutions[i].TargetPosition != i || resolutions[i].Status != "assigned" ||
				resolutions[i].UnitID == nil || *resolutions[i].UnitID != exact.UnitID || *resolutions[i].AddressRuleID != exact.ID {
				t.Fatal("proper subset or distinct original position lost")
			}
		}
		if resolutions[2].Status != "unassigned" || resolutions[2].UnitID != nil || resolutions[2].AddressRuleID != nil {
			t.Fatal("no rule became a default Unit or ambiguous state")
		}
	}
	narrowAddress := numericRule(network, 150, 150, nil)
	narrowFunction := numericRule(network, 100, 200, []uint16{1})
	if !errors.Is(ValidateChange(t.Context(), narrowAddress, []Rule{narrowFunction}), ErrRuleConflict) {
		t.Fatal("Address precision silently defeated Function precision")
	}
	if _, err := Resolve(t.Context(), network, protocols.POCSAG, now, targets[:1], []Rule{narrowAddress, narrowFunction}); !errors.Is(err, ErrRuleConflict) {
		t.Fatal("an inconsistent current rule set became a successful assignment")
	}
}

func TestRuleTemporalAndModeConflicts(t *testing.T) {
	network := uuid.New()
	base := numericRule(network, 100, 200, nil)
	from := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	base.ValidFrom, base.ValidUntil = &from, &until
	equal := base
	equal.ID = uuid.New()
	if !errors.Is(ValidateChange(t.Context(), equal, []Rule{base}), ErrRuleConflict) {
		t.Fatal("equally specific rule accepted, including with same Unit")
	}
	adjacent := equal
	adjacent.ValidFrom, adjacent.ValidUntil = &until, nil
	if err := ValidateChange(t.Context(), adjacent, []Rule{base}); err != nil {
		t.Fatal("adjacent half-open intervals overlap")
	}
	narrow := numericRule(network, 150, 150, []uint16{2})
	narrow.Selector.Mode = "subaddress"
	if !errors.Is(ValidateChange(t.Context(), narrow, []Rule{base}), ErrRuleConflict) {
		t.Fatal("overlapping POCSAG modes accepted")
	}
	base.Selector.Functions = []uint16{1}
	if !errors.Is(ValidateChange(t.Context(), narrow, []Rule{base}), ErrRuleConflict) {
		t.Fatal("disjoint Function values bypassed Address-mode consistency")
	}
	disabled := equal
	disabled.Enabled = false
	if err := ValidateChange(t.Context(), disabled, []Rule{base}); err != nil {
		t.Fatal("disabled rule treated as simultaneously active")
	}
	function := uint8(1)
	targets := []protocols.Target{{Address: protocols.NumericAddress(150), Function: &function}}
	for _, test := range []struct {
		at   time.Time
		want string
	}{{from.Add(-time.Second), "unassigned"}, {from, "assigned"}, {until.Add(-time.Second), "assigned"}, {until, "unassigned"}} {
		result, err := Resolve(t.Context(), network, protocols.POCSAG, test.at, targets, []Rule{base})
		if err != nil || result[0].Status != test.want {
			t.Fatalf("half-open boundary %v: %v %v", test.at, result, err)
		}
	}
}

func TestStringRulesAreExactAndEmptyTargetsStayEmpty(t *testing.T) {
	for _, protocol := range []protocols.Protocol{protocols.ZVEI, protocols.EAS} {
		value, other := "00123", "00124"
		if protocol == protocols.EAS {
			value, other = "001234", "001200"
		}
		rule := Rule{ID: uuid.New(), Revision: 1, NetworkID: uuid.New(), UnitID: uuid.New(), Protocol: protocol,
			Selector: Selector{Value: &value}, Enabled: true}
		now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
		targets := []protocols.Target{{Address: protocols.TextAddress(value)}, {Address: protocols.TextAddress(other)}}
		snapshot := append([]protocols.Target(nil), targets...)
		result, err := Resolve(t.Context(), rule.NetworkID, protocol, now, targets, []Rule{rule})
		if err != nil || result[0].Status != "assigned" || result[1].Status != "unassigned" || !reflect.DeepEqual(snapshot, targets) {
			t.Fatal("string Addresses expanded, coerced or mutated", err)
		}
		empty, err := Resolve(t.Context(), rule.NetworkID, protocol, now, nil, []Rule{rule})
		if err != nil || empty == nil || len(empty) != 0 {
			t.Fatal("empty target list did not retain an explicit empty result")
		}
	}
}

func TestRuleInvalidityAndCancellationAreNotUnassigned(t *testing.T) {
	rule := numericRule(uuid.New(), 100, 200, nil)
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	invalid := rule
	invalid.Selector.Functions = []uint16{}
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidRule) {
		t.Fatal("empty Function set became wildcard")
	}

	invalid = rule
	future := now.Add(time.Nanosecond)
	invalid.ValidFrom = &future
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidRule) {
		t.Fatal("unsupported rule precision silently truncated")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Resolve(ctx, rule.NetworkID, rule.Protocol, now, nil, []Rule{rule}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled evaluation became successful/empty")
	}
	if _, err := Resolve(ctx, rule.NetworkID, rule.Protocol, now, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled empty evaluation became successful")
	}
	if _, err := Resolve(t.Context(), rule.NetworkID, rule.Protocol, future, nil, []Rule{rule}); !errors.Is(err, ErrInvalidRule) {
		t.Fatal("unnormalized Observation time accepted")
	}

}

func TestSelectorJSONIsTypedAndDoesNotConvertFunctionListsToBase64(t *testing.T) {
	first, last := uint64(1), uint64(2)
	selector := Selector{From: &first, To: &last, Mode: "function-bit", Functions: []uint16{1, 3}}
	data, err := json.Marshal(selector)
	if err != nil || string(data) != `{"from":1,"to":2,"mode":"function-bit","functions":[1,3]}` {
		t.Fatalf("incorrect selector wire shape: %s %v", data, err)
	}
	var decoded Selector
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Validate(protocols.POCSAG) != nil {
		t.Fatalf("selector JSON did not round-trip: %v", err)
	}
	for _, wire := range []string{
		`{"from":1,"to":2,"mode":"function-bit","functions":"AQM="}`,
		`{"from":1,"to":2,"mode":"function-bit","functions":null}`,
		`{"from":1,"to":2,"mode":null}`,
		`{"from":1,"to":2,"unknown":true}`,
	} {
		if err := json.Unmarshal([]byte(wire), &decoded); err == nil {
			t.Fatalf("invalid selector shape accepted: %s", wire)
		}
	}
	if err := json.Unmarshal([]byte(`{"from":1,"to":2,"mode":""}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(decoded.Validate(protocols.FLEX), ErrInvalidRule) {
		t.Fatal("explicit unsupported FLEX mode disappeared during parsing")
	}
	for _, functions := range []string{`[]`, `[1,1]`, `[4]`} {
		wire := `{"from":1,"to":2,"mode":"function-bit","functions":` + functions + `}`
		if err := json.Unmarshal([]byte(wire), &decoded); err != nil {
			t.Fatal(err)
		}
		if !errors.Is(decoded.Validate(protocols.POCSAG), ErrInvalidRule) {
			t.Fatalf("invalid Function set accepted: %s", functions)
		}
	}
	if err := json.Unmarshal([]byte(`{"value":"00123","mode":""}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(decoded.Validate(protocols.ZVEI), ErrInvalidRule) {
		t.Fatal("explicit unsupported ZVEI mode disappeared during parsing")
	}
}
