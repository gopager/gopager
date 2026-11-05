// Package assignments owns current temporal Address rules, the resolution of
// transmission targets to Units, and historical assignment correction.
//
// Rule, Selector, ValidateChange and Resolve are the pure rule model. Service
// (built with NewService) provides authorized rule management (CreateRule,
// UpdateRule, DeleteRule, Preview, ReadRule, ListRules), suppression and
// correction management (CreateSuppression, LiftSuppression, RetryCorrection,
// and reads), and the transaction-participating matcher used by ingest
// (MatchTargets, NetworksWithMatches). Every rule or suppression change bumps
// the global assignment revision and queues a pending correction scope;
// CorrectionRunner drains those scopes in the background, rewrites stored
// target resolutions and appends assignment.changed facts.
package assignments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/protocols"
)

// Sentinel errors returned by rule validation and resolution.
var (
	// ErrInvalidRule reports a malformed rule, selector or target input.
	ErrInvalidRule = errors.New("invalid Address rule")
	// ErrRuleConflict reports overlapping enabled rules whose winner cannot be
	// decided by strict selector containment, or overlapping POCSAG rules with
	// different modes.
	ErrRuleConflict = errors.New("Address rule conflicts with current configuration")
)

// Selector describes an Address/Function set; absent Functions covers all POCSAG bits.
type Selector struct {
	From          *uint64  `json:"from,omitempty"`
	To            *uint64  `json:"to,omitempty"`
	Value         *string  `json:"value,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	Functions     []uint16 `json:"functions,omitempty"`
	modeSpecified bool
}

// UnmarshalJSON preserves optional-field presence and rejects unknown/null selector attributes.
func (s *Selector) UnmarshalJSON(data []byte) error {
	*s = Selector{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return ErrInvalidRule
	}
	for name, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrInvalidRule
		}
		switch name {
		case "from":
			if err := json.Unmarshal(value, &s.From); err != nil {
				return ErrInvalidRule
			}
		case "to":
			if err := json.Unmarshal(value, &s.To); err != nil {
				return ErrInvalidRule
			}
		case "value":
			if err := json.Unmarshal(value, &s.Value); err != nil {
				return ErrInvalidRule
			}
		case "mode":
			if err := json.Unmarshal(value, &s.Mode); err != nil {
				return ErrInvalidRule
			}
			s.modeSpecified = true
		case "functions":
			if err := json.Unmarshal(value, &s.Functions); err != nil {
				return ErrInvalidRule
			}
		default:
			return ErrInvalidRule
		}
	}
	return nil
}

// Rule is mutable configuration, not an immutable historical rule-version entity.
type Rule struct {
	ID         uuid.UUID          `json:"id"`
	Revision   int64              `json:"revision,string"`
	NetworkID  uuid.UUID          `json:"networkId"`
	UnitID     uuid.UUID          `json:"unitId"`
	Protocol   protocols.Protocol `json:"protocol"`
	Selector   Selector           `json:"selector"`
	ValidFrom  *time.Time         `json:"validFrom"`
	ValidUntil *time.Time         `json:"validUntil"`
	Enabled    bool               `json:"enabled"`
}

// Validate checks a complete rule's shape without querying owner references.
func (r Rule) Validate() error {
	if r.ID == uuid.Nil || r.Revision <= 0 || r.NetworkID == uuid.Nil || r.UnitID == uuid.Nil || !r.Protocol.Valid() {
		return ErrInvalidRule
	}
	for _, bound := range []*time.Time{r.ValidFrom, r.ValidUntil} {
		if bound != nil && (bound.Year() < 0 || bound.Year() > 9999 || bound.Nanosecond()%1000 != 0) {
			return ErrInvalidRule
		}
	}
	if r.ValidFrom != nil && r.ValidUntil != nil && !r.ValidFrom.Before(*r.ValidUntil) {
		return ErrInvalidRule
	}
	return r.Selector.Validate(r.Protocol)
}

// Validate rejects mixed selector variants, invalid Functions and unsupported ranges.
func (s Selector) Validate(protocol protocols.Protocol) error {
	switch protocol {
	case protocols.POCSAG, protocols.FLEX:
		if s.From == nil || s.To == nil || s.Value != nil || protocols.ValidateRange(protocol, *s.From, *s.To) != nil {
			return ErrInvalidRule
		}
		if protocol == protocols.FLEX {
			if s.Mode != "" || s.modeSpecified || s.Functions != nil {
				return ErrInvalidRule
			}
			return nil
		}
		if s.Mode != "function-bit" && s.Mode != "subaddress" {
			return ErrInvalidRule
		}
		if s.Functions != nil && len(s.Functions) == 0 {
			return ErrInvalidRule
		}
		var seen uint8
		for _, function := range s.Functions {
			if function > 3 || seen&(1<<function) != 0 {
				return ErrInvalidRule
			}
			seen |= 1 << function
		}
	case protocols.ZVEI, protocols.EAS:
		if s.Value == nil || s.From != nil || s.To != nil || s.Mode != "" || s.modeSpecified || s.Functions != nil ||
			protocols.ValidateTarget(protocol, protocols.Target{Address: protocols.TextAddress(*s.Value)}) != nil {
			return ErrInvalidRule
		}
	default:
		return ErrInvalidRule
	}
	return nil
}

// functionMask returns the represented Function set after selector validation.
func (s Selector) functionMask() uint8 {
	if s.Functions == nil {
		return 15
	}
	var mask uint8
	for _, function := range s.Functions {
		mask |= 1 << function
	}
	return mask
}

// addressOverlaps compares two validated selectors of the same Protocol.
func (s Selector) addressOverlaps(other Selector) bool {
	if s.Value != nil {
		return *s.Value == *other.Value
	}
	return *s.From <= *other.To && *other.From <= *s.To
}

// contains compares set inclusion, not a heuristic Address-versus-Function score.
func (s Selector) contains(other Selector) bool {
	addressContains := false
	if s.Value != nil {
		addressContains = *s.Value == *other.Value
	} else {
		addressContains = *s.From <= *other.From && *other.To <= *s.To
	}
	return addressContains && other.functionMask()&s.functionMask() == other.functionMask()
}

// activeAt applies the half-open rule interval at original Observation time.
func (r Rule) activeAt(observed time.Time) bool {
	return r.Enabled && (r.ValidFrom == nil || !observed.Before(*r.ValidFrom)) &&
		(r.ValidUntil == nil || observed.Before(*r.ValidUntil))
}

// ValidateChange rejects ambiguity and overlapping POCSAG-mode disagreement.
func ValidateChange(ctx context.Context, candidate Rule, current []Rule) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	for _, existing := range current {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := existing.Validate(); err != nil {
			return err
		}
		if existing.ID == candidate.ID || existing.NetworkID != candidate.NetworkID ||
			existing.Protocol != candidate.Protocol || !existing.Enabled || !candidate.Enabled ||
			(existing.ValidUntil != nil && candidate.ValidFrom != nil && !candidate.ValidFrom.Before(*existing.ValidUntil)) ||
			(candidate.ValidUntil != nil && existing.ValidFrom != nil && !existing.ValidFrom.Before(*candidate.ValidUntil)) ||
			!existing.Selector.addressOverlaps(candidate.Selector) {
			continue
		}
		if existing.Protocol == protocols.POCSAG && existing.Selector.Mode != candidate.Selector.Mode {
			return ErrRuleConflict
		}
		if existing.Selector.functionMask()&candidate.Selector.functionMask() == 0 {
			continue
		}
		left, right := existing.Selector.contains(candidate.Selector), candidate.Selector.contains(existing.Selector)
		if left == right {
			return ErrRuleConflict
		}
	}
	return nil
}

// Resolution is the outcome for one target position: Status "assigned" with
// UnitID and AddressRuleID of the winning rule, or "unassigned" with both nil.
type Resolution struct {
	TargetPosition int        `json:"targetPosition"`
	Status         string     `json:"status"`
	UnitID         *uuid.UUID `json:"unitId"`
	AddressRuleID  *uuid.UUID `json:"addressRuleId"`
}

// validateTargets checks the shared normalized input even when the candidate Network set is empty.
func validateTargets(ctx context.Context, protocol protocols.Protocol, observed time.Time, targets []protocols.Target) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !protocol.Valid() || observed.Nanosecond() != 0 ||
		observed.Year() < 0 || observed.Year() > 9999 || len(targets) > 1024 {
		return ErrInvalidRule
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := protocols.ValidateTarget(protocol, target); err != nil {
			return err
		}
	}
	return nil
}

// Resolve evaluates ordered validated original targets without deduplication or mutation.
// Current rules and Unit existence must be captured by the caller's live transaction.
func Resolve(ctx context.Context, network uuid.UUID, protocol protocols.Protocol, observed time.Time, targets []protocols.Target, rules []Rule) ([]Resolution, error) {
	if err := validateTargets(ctx, protocol, observed, targets); err != nil {
		return nil, err
	}
	if network == uuid.Nil {
		return nil, ErrInvalidRule
	}
	for _, rule := range rules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := rule.Validate(); err != nil {
			return nil, err
		}
	}
	result := make([]Resolution, len(targets))
	for position, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var winner *Rule
		for i := range rules {
			rule := &rules[i]
			if rule.NetworkID != network || rule.Protocol != protocol || !rule.activeAt(observed) || !rule.Selector.matches(target) {
				continue
			}
			if winner == nil {
				winner = rule
				continue
			}
			if rule.Protocol == protocols.POCSAG && rule.Selector.Mode != winner.Selector.Mode {
				return nil, ErrRuleConflict
			}
			left, right := winner.Selector.contains(rule.Selector), rule.Selector.contains(winner.Selector)
			if left == right {
				return nil, ErrRuleConflict
			}
			if left {
				winner = rule
			}
		}
		result[position] = Resolution{TargetPosition: position, Status: "unassigned"}
		if winner != nil {
			unit, ruleID := winner.UnitID, winner.ID
			result[position].Status = "assigned"
			result[position].UnitID, result[position].AddressRuleID = &unit, &ruleID
		}
	}
	return result, nil
}

// matches checks a validated target against a validated selector of its Protocol.
func (s Selector) matches(target protocols.Target) bool {
	if s.Value != nil {
		value, ok := target.Address.Text()
		return ok && value == *s.Value
	}
	number, ok := target.Address.Number()
	return ok && *s.From <= number && number <= *s.To &&
		(target.Function == nil || s.functionMask()&(1<<*target.Function) != 0)
}
