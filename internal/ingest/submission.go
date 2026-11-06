package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/protocols"
)

// Intake limits. MaxSubmissionBytes bounds the encoded request body;
// MaxSubmissionIDSize bounds the Submission ID and the Gateway Network
// identifier in bytes.
const (
	MaxSubmissionBytes  = 1 << 20
	MaxSubmissionIDSize = 128
)

// Submission validation outcomes. Service.Submit maps ErrInvalidSubmission to
// CodeInvalidInput and ErrSubmissionTooLarge to CodeResourceLimit, and returns
// ErrSubmissionConflict unwrapped when a Submission ID is reused with a
// different fingerprint.
var (
	ErrInvalidSubmission  = errors.New("invalid Submission")
	ErrSubmissionConflict = errors.New("Submission ID conflicts with existing operation")
	ErrSubmissionTooLarge = errors.New("Submission exceeds configured size limit")
)

// Submission is the wire form of one Gateway-observed Transmission (schema
// version 1). It is stored as received; normalization happens only in the
// fingerprint computed by Validate. Incomplete is nil when the member was
// absent.
type Submission struct {
	SchemaVersion            int                `json:"schemaVersion"`
	SubmissionID             string             `json:"submissionId"`
	Protocol                 protocols.Protocol `json:"protocol"`
	ObservationTime          string             `json:"observationTime"`
	GatewayNetworkIdentifier string             `json:"gatewayNetworkIdentifier,omitempty"`
	Payload                  json.RawMessage    `json:"payload"`
	TransmissionTargets      []protocols.Target `json:"transmissionTargets"`
	QualityMetrics           json.RawMessage    `json:"qualityMetrics,omitempty"`
	Incomplete               *bool              `json:"incomplete,omitempty"`
}

// UnmarshalJSON enforces a closed Submission object and distinguishes missing from empty required fields.
func (s *Submission) UnmarshalJSON(data []byte) error {
	members, err := jsonObjectMembers(data)
	if err != nil {
		return ErrInvalidSubmission
	}
	required := []string{"schemaVersion", "submissionId", "protocol", "observationTime", "payload", "transmissionTargets"}
	for _, field := range required {
		value, exists := members[field]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrInvalidSubmission
		}
	}
	allowed := map[string]bool{
		"schemaVersion": true, "submissionId": true, "protocol": true, "observationTime": true,
		"gatewayNetworkIdentifier": true, "payload": true, "transmissionTargets": true,
		"qualityMetrics": true, "incomplete": true,
	}
	for field, value := range members {
		if !allowed[field] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) &&
			(field == "gatewayNetworkIdentifier" || field == "qualityMetrics" || field == "incomplete") {
			return ErrInvalidSubmission
		}
	}
	type plain Submission
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return ErrInvalidSubmission
	}
	*s = Submission(decoded)
	return nil
}

// ValidatedSubmission is a Submission that passed Validate, enriched with the
// parsed observation time (truncated to whole seconds), the payload identity,
// the retry fingerprint and, once Service.Submit resolved them, the Network
// and per-target resolutions. Resolution is one of "declared",
// "inferred-single", "inferred-address" or "default".
type ValidatedSubmission struct {
	Submission
	ObservedAt        time.Time
	ResolvedNetwork   uuid.UUID
	Resolution        string
	TargetResolutions []assignments.Resolution
	Identity          protocols.PayloadIdentity
	Fingerprint       [32]byte
	AmbiguousNetwork  bool
}

// GatewayNetwork is one Network assigned to a Gateway under the Gateway-local
// Identifier. Each protocol a Gateway serves has at most one Default Network.
type GatewayNetwork struct {
	Identifier string
	NetworkID  uuid.UUID
	Protocol   protocols.Protocol
	// AgencyID is the Network's Agency, filled by GatewayNetworksForAuthorization for grant checks.
	AgencyID uuid.UUID
	Default  bool
}

// Receipt is the acceptance result returned to the Gateway. Retrying a
// Submission with the same ID and fingerprint returns the original Receipt.
type Receipt struct {
	SubmissionID     string    `json:"submissionId"`
	TransmissionID   uuid.UUID `json:"transmissionId"`
	AcceptanceTime   time.Time `json:"acceptanceTime"`
	ProcessingStatus string    `json:"processingStatus"`
	MessageID        uuid.UUID `json:"messageId"`
}

// Validate checks the Submission envelope and its protocol-specific payload
// and targets, and computes the retry fingerprint over a canonical encoding
// (observation time normalized to whole seconds, absent incomplete as false)
// without rewriting the original fields. encodedSize is the request body size;
// above MaxSubmissionBytes it fails with ErrSubmissionTooLarge. Every other
// violation, including a zero acceptedAt, yields ErrInvalidSubmission.
func (s Submission) Validate(encodedSize int, acceptedAt time.Time) (ValidatedSubmission, error) {
	if encodedSize > MaxSubmissionBytes {
		return ValidatedSubmission{}, ErrSubmissionTooLarge
	}
	if encodedSize <= 0 || s.SchemaVersion != 1 ||
		s.SubmissionID == "" || !utf8.ValidString(s.SubmissionID) || len([]byte(s.SubmissionID)) > MaxSubmissionIDSize ||
		strings.TrimSpace(s.SubmissionID) != s.SubmissionID || !s.Protocol.Valid() ||
		s.ObservationTime == "" || len(s.Payload) == 0 || len(s.TransmissionTargets) > protocols.MaxTransmissionTargets ||
		acceptedAt.IsZero() {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	observed, err := time.Parse(time.RFC3339Nano, s.ObservationTime)
	if err != nil {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	observed = observed.Truncate(time.Second)
	if observed.Year() < 0 || observed.Year() > 9999 {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	if err := protocols.ValidateTransmissionTargets(s.Protocol, s.TransmissionTargets); err != nil {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	var payload protocols.TransmissionPayload
	if err := json.Unmarshal(s.Payload, &payload); err != nil {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	identity, err := payload.Identity(s.Protocol)
	if err != nil {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	if s.QualityMetrics != nil && (!json.Valid(s.QualityMetrics) || bytes.Equal(bytes.TrimSpace(s.QualityMetrics), []byte("null"))) {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	incomplete := false
	if s.Incomplete != nil {
		incomplete = *s.Incomplete
	}
	normalized := struct {
		SubmissionID string             `json:"submissionId"`
		Protocol     protocols.Protocol `json:"protocol"`
		Observation  string             `json:"observationTime"`
		Network      string             `json:"gatewayNetworkIdentifier,omitempty"`
		Payload      json.RawMessage    `json:"payload"`
		Targets      []protocols.Target `json:"transmissionTargets"`
		Quality      json.RawMessage    `json:"qualityMetrics,omitempty"`
		Incomplete   bool               `json:"incomplete"`
	}{s.SubmissionID, s.Protocol, observed.Format(time.RFC3339), s.GatewayNetworkIdentifier,
		bytes.Clone(s.Payload), s.TransmissionTargets, bytes.Clone(s.QualityMetrics), incomplete}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return ValidatedSubmission{}, fmt.Errorf("encode Submission fingerprint: %w", err)
	}
	result := ValidatedSubmission{Submission: s, ObservedAt: observed, Identity: identity,
		Fingerprint: sha256.Sum256(canonical)}
	if s.GatewayNetworkIdentifier != "" &&
		(!utf8.ValidString(s.GatewayNetworkIdentifier) || len([]byte(s.GatewayNetworkIdentifier)) > MaxSubmissionIDSize) {
		return ValidatedSubmission{}, ErrInvalidSubmission
	}
	return result, nil
}

// IncompleteValue reports the incomplete flag, treating an absent value as false.
func (s Submission) IncompleteValue() bool { return s.Incomplete != nil && *s.Incomplete }

// ResolveNetwork applies only the assigned, protocol-compatible Gateway Network set.
func ResolveNetwork(input ValidatedSubmission, assigned []GatewayNetwork,
	addressMatches []uuid.UUID) (uuid.UUID, string, error) {
	var candidates []GatewayNetwork
	identifiers := make(map[string]bool)
	for _, network := range assigned {
		if network.Identifier == "" || network.NetworkID == uuid.Nil || !network.Protocol.Valid() ||
			identifiers[network.Identifier] {
			return uuid.Nil, "", ErrInvalidSubmission
		}
		identifiers[network.Identifier] = true
		if network.Protocol == input.Protocol {
			candidates = append(candidates, network)
		}
	}
	if input.GatewayNetworkIdentifier != "" {
		for _, candidate := range candidates {
			if candidate.Identifier == input.GatewayNetworkIdentifier {
				return candidate.NetworkID, "declared", nil
			}
		}
		return uuid.Nil, "", ErrInvalidSubmission
	}
	if len(candidates) == 1 {
		return candidates[0].NetworkID, "inferred-single", nil
	}
	if len(candidates) == 0 {
		return uuid.Nil, "", ErrInvalidSubmission
	}
	matches := make(map[uuid.UUID]bool)
	for _, id := range addressMatches {
		for _, candidate := range candidates {
			if candidate.NetworkID == id {
				matches[id] = true
			}
		}
	}
	if len(matches) == 1 {
		for id := range matches {
			return id, "inferred-address", nil
		}
	}
	for _, candidate := range candidates {
		if candidate.Default {
			return candidate.NetworkID, "default", nil
		}
	}
	return uuid.Nil, "", ErrInvalidSubmission
}
