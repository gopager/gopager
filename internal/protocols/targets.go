// Package protocols owns finalized Protocol data validation, not radio decoding.
package protocols

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
)

// Protocol identifies a paging protocol by its stable lowercase wire name.
type Protocol string

// Supported protocols; Protocol.Valid accepts exactly these.
const (
	POCSAG Protocol = "pocsag"
	FLEX   Protocol = "flex"
	ZVEI   Protocol = "zvei"
	EAS    Protocol = "eas"
)

// ErrInvalidTarget is returned for any malformed or unsupported target,
// Address or address range.
var ErrInvalidTarget = errors.New("invalid Protocol target")

// Address preserves the Protocol's integer/string distinction without floating point.
type Address struct {
	number uint64
	text   string
	kind   byte
}

// NumericAddress constructs an integer Address; the Protocol validates its domain.
func NumericAddress(value uint64) Address { return Address{number: value, kind: 'n'} }

// TextAddress preserves a string Address, including its leading zeros.
func TextAddress(value string) Address { return Address{text: value, kind: 's'} }

// Number returns the exact integer and whether the Address is numeric.
func (a Address) Number() (uint64, bool) { return a.number, a.kind == 'n' }

// Text returns the original string and whether the Address is textual.
func (a Address) Text() (string, bool) { return a.text, a.kind == 's' }

// MarshalJSON serializes the original scalar type, rejecting an unset Address.
func (a Address) MarshalJSON() ([]byte, error) {
	switch a.kind {
	case 'n':
		return []byte(strconv.FormatUint(a.number, 10)), nil
	case 's':
		return json.Marshal(a.text)
	default:
		return nil, ErrInvalidTarget
	}
}

// UnmarshalJSON accepts only an exact unsigned JSON integer or a JSON string.
func (a *Address) UnmarshalJSON(data []byte) error {
	*a = Address{}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ErrInvalidTarget
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return ErrInvalidTarget
		}
		*a = TextAddress(value)
		return nil
	}
	for _, digit := range data {
		if digit < '0' || digit > '9' {
			return ErrInvalidTarget
		}
	}
	value, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil || (len(data) > 1 && data[0] == '0') {
		return ErrInvalidTarget
	}
	*a = NumericAddress(value)
	return nil
}

// Target is one addressed recipient of a Transmission. Function is the
// POCSAG function bit pair (0..3), required for POCSAG and forbidden for all
// other protocols; nil means absent.
type Target struct {
	Address  Address `json:"address"`
	Function *uint8  `json:"function,omitempty"`
}

// UnmarshalJSON rejects unknown attributes and explicit null instead of losing their presence.
func (t *Target) UnmarshalJSON(data []byte) error {
	*t = Target{}
	var input struct {
		Address  Address         `json:"address"`
		Function json.RawMessage `json:"function"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return ErrInvalidTarget
	}
	t.Address = input.Address
	if input.Function != nil {
		var function uint8
		if bytes.Equal(bytes.TrimSpace(input.Function), []byte("null")) || json.Unmarshal(input.Function, &function) != nil {
			return ErrInvalidTarget
		}
		t.Function = &function
	}
	return nil
}

// Valid recognizes only the initial server-owned Protocol definitions.
func (p Protocol) Valid() bool {
	return p == POCSAG || p == FLEX || p == ZVEI || p == EAS
}

// ValidateTarget rejects unsupported fields/classes without rewriting original data.
func ValidateTarget(protocol Protocol, target Target) error {
	switch protocol {
	case POCSAG:
		number, ok := target.Address.Number()
		if !ok || number > 2097151 || target.Function == nil || *target.Function > 3 {
			return ErrInvalidTarget
		}
	case FLEX:
		number, ok := target.Address.Number()
		if !ok || target.Function != nil {
			return ErrInvalidTarget
		}
		if _, err := FLEXClass(number); err != nil {
			return err
		}
	case ZVEI, EAS:
		value, ok := target.Address.Text()
		size := 5
		if protocol == EAS {
			size = 6
		}
		if !ok || target.Function != nil || len(value) != size {
			return ErrInvalidTarget
		}
		for _, digit := range value {
			if digit < '0' || digit > '9' {
				return ErrInvalidTarget
			}
		}
	default:
		return ErrInvalidTarget
	}
	return nil
}

type flexInterval struct {
	first, last uint64
	class       string
}

// Appendix A, Table A-1 of ARIB RCR STD-43A defines these message-address classes.
// Network/control, unused, illegal and future-reserved regions are not user targets.
var flexIntervals = [...]flexInterval{
	{1, 1933312, "short"},
	{2009088, 2025471, "information-service"},
	{2029568, 2029583, "temporary"},
	{2029584, 2029599, "operator-messaging"},
	{2101249, 1075843072, "long-1-2"},
	{1075843073, 2149584896, "long-1-3"},
	{2149584897, 3223326720, "long-1-4"},
	{3223326721, 3923326750, "long-2-3"},
	{4280000001, 4291000000, "long-information-service"},
}

// FLEXClass identifies a supported message address without inventing individual members.
func FLEXClass(address uint64) (string, error) {
	for _, interval := range flexIntervals {
		if interval.first <= address && address <= interval.last {
			return interval.class, nil
		}
	}
	return "", ErrInvalidTarget
}

// ValidateRange proves that every address in an inclusive selector range is supported.
func ValidateRange(protocol Protocol, first, last uint64) error {
	if first > last {
		return ErrInvalidTarget
	}
	if protocol == POCSAG && last <= 2097151 {
		return nil
	}
	if protocol != FLEX {
		return ErrInvalidTarget
	}
	next := first
	for _, interval := range flexIntervals {
		if interval.last < next {
			continue
		}
		if next < interval.first {
			return ErrInvalidTarget
		}
		if last <= interval.last {
			return nil
		}
		next = interval.last + 1
	}
	return ErrInvalidTarget
}
