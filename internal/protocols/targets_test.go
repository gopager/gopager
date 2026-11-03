package protocols

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestTargetJSONPreservesTypesAndExactValues(t *testing.T) {
	function := uint8(3)
	for _, test := range []struct {
		protocol Protocol
		target   Target
		wire     string
	}{
		{POCSAG, Target{NumericAddress(0), &function}, `{"address":0,"function":3}`},
		{FLEX, Target{Address: NumericAddress(4280000001)}, `{"address":4280000001}`},
		{FLEX, Target{Address: NumericAddress(2029568)}, `{"address":2029568}`},
		{ZVEI, Target{Address: TextAddress("00123")}, `{"address":"00123"}`},
		{EAS, Target{Address: TextAddress("001234")}, `{"address":"001234"}`},
	} {
		t.Run(string(test.protocol)+test.wire, func(t *testing.T) {
			encoded, err := json.Marshal(test.target)
			if err != nil || string(encoded) != test.wire {
				t.Fatalf("changed original target type/value: %s %v", encoded, err)
			}
			var decoded Target
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if err := ValidateTarget(test.protocol, decoded); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, wire := range []string{
		`{"address":1.0}`, `{"address":1e2}`, `{"address":-1}`, `{"address":null}`,
		`{"address":18446744073709551616}`, `{"address":{}}`, `{"address":true}`,
		`{"address":1,"function":null}`, `{"address":1,"unknown":true}`,
	} {
		var target Target
		if err := json.Unmarshal([]byte(wire), &target); err == nil {
			t.Fatalf("accepted non-integer/non-string Address: %s", wire)
		}
	}
	var unset Target
	if _, err := json.Marshal(unset); !errors.Is(err, ErrInvalidTarget) {
		t.Fatal("unset Address became an artificial zero")
	}
}

func TestTargetValidationRejectsMissingWrongAndUnsupportedAttributes(t *testing.T) {
	zero, four := uint8(0), uint8(4)
	for _, test := range []struct {
		protocol Protocol
		target   Target
	}{
		{POCSAG, Target{}},
		{POCSAG, Target{Address: NumericAddress(1)}},
		{POCSAG, Target{NumericAddress(1), &four}},
		{POCSAG, Target{NumericAddress(2097152), &zero}},
		{POCSAG, Target{TextAddress("00001"), &zero}},
		{FLEX, Target{NumericAddress(1), &zero}},
		{FLEX, Target{Address: TextAddress("A0000001.128")}},
		{ZVEI, Target{Address: NumericAddress(12345)}},
		{ZVEI, Target{Address: TextAddress("0123")}},
		{ZVEI, Target{Address: TextAddress("0123E")}},
		{EAS, Target{Address: TextAddress("12345")}},
		{EAS, Target{TextAddress("001234"), &zero}},
		{"smtp", Target{Address: TextAddress("12345")}},
	} {
		if !errors.Is(ValidateTarget(test.protocol, test.target), ErrInvalidTarget) {
			t.Fatalf("accepted unsupported target: %s %+v", test.protocol, test.target)
		}
	}
}

func TestFLEXAddressClassesAndUnsupportedGaps(t *testing.T) {
	for _, interval := range flexIntervals {
		for _, address := range []uint64{interval.first, interval.last} {
			class, err := FLEXClass(address)
			if err != nil || class != interval.class {
				t.Fatalf("class %d: %s %v", address, class, err)
			}
		}
		if err := ValidateRange(FLEX, interval.first, interval.last); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []uint64{0, 1933313, 1998848, 1998849, 2009087, 2025472, 2029567,
		2029600, 2031614, 2031615, 2101248, 3923326751, 4280000000, 4291000001, 4297068542, 4297068543} {
		if _, err := FLEXClass(address); !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("illegal/control/reserved Address accepted as ordinary target: %d", address)
		}
	}
	if err := ValidateRange(FLEX, 1075843072, 2149584897); err != nil {
		t.Fatal("contiguous supported classes must not be artificially separated")
	}
	for _, pair := range [][2]uint64{{1933312, 2009088}, {2025471, 2029568}, {3923326750, 4280000001}, {2, 1}} {
		if err := ValidateRange(FLEX, pair[0], pair[1]); !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("range admitted an unsupported interior: %v", pair)
		}
	}
}
