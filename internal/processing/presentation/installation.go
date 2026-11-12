package presentation

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// NoticeAction changes the installation settings.
const NoticeAction = "notice.manage"

// Bounds of the installation settings, in characters.
const (
	maxTitleChars  = 128
	maxBannerChars = 2048
)

// The severities of the banner.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Installation is the installation-wide presentation: the title and notice banner every client
// shows, and the Layout used when no narrower layer chooses one.
type Installation struct {
	Title string `json:"title"`
	// Banner is plain text; nil when there is no banner.
	Banner *string `json:"banner,omitempty"`
	// Severity is set exactly when Banner is.
	Severity        *string    `json:"severity,omitempty"`
	DefaultLayoutID *uuid.UUID `json:"defaultLayoutId,omitempty"`
	Revision        int64      `json:"revision,string"`
}

// InstallationInput replaces every editable field of the installation settings.
type InstallationInput struct {
	Title           string
	Banner          *string
	Severity        *string
	DefaultLayoutID *uuid.UUID
}

// SettingsStore is the persistence port of the installation settings. Every method runs in the
// caller's transaction.
type SettingsStore interface {
	// ReadInstallation returns the settings.
	ReadInstallation(ctx context.Context, tx platform.TransactionContext) (Installation, error)
	// UpdateInstallation replaces the settings at the expected revision and bumps it. A stale
	// revision is authz.ErrRevisionConflict; a default Layout that does not exist is
	// authz.InvalidField("defaultLayoutId"). The Layout stays until the transaction ends.
	UpdateInstallation(ctx context.Context, tx platform.TransactionContext, expected int64, input InstallationInput, now time.Time) error
}

// InstallationService reads and changes the installation settings. Create it with
// NewInstallationService; it is safe for concurrent use.
type InstallationService struct {
	callerScope
	store SettingsStore
}

// NewInstallationService composes the service. It fails with authz.ErrInvalidInput on a missing
// dependency.
func NewInstallationService(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store SettingsStore) (*InstallationService, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil {
		return nil, authz.ErrInvalidInput
	}
	return &InstallationService{callerScope: callerScope{transactions: transactions, resolver: resolver, clock: clock},
		store: store}, nil
}

// Read returns the settings. It needs no credential: every client, guests included, shows the
// title and banner before anyone signs in.
func (s *InstallationService) Read(ctx context.Context) (Installation, error) {
	var result Installation
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		result, err = s.store.ReadInstallation(tx.Context(), tx)
		return err
	})
	return result, err
}

// Update replaces the settings at the expected revision and returns them. It needs notice.manage
// with scope all. Invalid input is authz.InvalidField naming the field; a stale revision is
// authz.ErrRevisionConflict.
func (s *InstallationService) Update(ctx context.Context, proof identity.Proof, expected int64, input InstallationInput,
	check func(identity.Authentication) error) (Installation, error) {
	if expected <= 0 {
		return Installation{}, authz.InvalidField("expectedRevision")
	}
	if err := validateInstallation(input); err != nil {
		return Installation{}, err
	}
	var result Installation
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		if err := authz.Authorize(who.authority, NoticeAction, nil, authz.Target{}, now); err != nil {
			return err
		}
		if err := s.store.UpdateInstallation(tx.Context(), tx, expected, input, now); err != nil {
			return err
		}
		var err error
		result, err = s.store.ReadInstallation(tx.Context(), tx)
		return err
	})
	return result, err
}

// validateInstallation checks every field and names the first invalid one.
func validateInstallation(input InstallationInput) error {
	if !plainText(input.Title, maxTitleChars) || strings.TrimSpace(input.Title) == "" {
		return authz.InvalidField("title")
	}
	if input.Banner != nil && (!plainText(*input.Banner, maxBannerChars) || strings.TrimSpace(*input.Banner) == "") {
		return authz.InvalidField("banner")
	}
	if input.Severity != nil && !slices.Contains([]string{SeverityInfo, SeverityWarning, SeverityCritical}, *input.Severity) {
		return authz.InvalidField("severity")
	}
	if (input.Banner == nil) != (input.Severity == nil) {
		return authz.InvalidField("severity")
	}
	if input.DefaultLayoutID != nil && *input.DefaultLayoutID == uuid.Nil {
		return authz.InvalidField("defaultLayoutId")
	}
	return nil
}

// plainText reports valid UTF-8 without NUL of at most limit characters.
func plainText(value string, limit int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= limit
}
