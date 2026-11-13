package identity

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"golang.org/x/text/language"

	// The runtime image carries no time zone database; embedding it keeps the validation of
	// IANA names independent of the host.
	_ "time/tzdata"
)

// PreferenceAction lets an account read and change its own preferences.
const PreferenceAction = "preference.manage-own"

// Bounds of the preference values.
const (
	maxLocaleBytes   = 35
	maxTimeZoneBytes = 64
	maxTimeGap       = 1440
)

// Preferences are the display preferences of one interactive account. An account that never saved
// any uses DefaultPreferences.
type Preferences struct {
	// Locale is a canonical BCP 47 language tag.
	Locale string `json:"locale"`
	// TimeZone is an IANA time zone name.
	TimeZone string `json:"timeZone"`
	// TimeGapMinutes (0 to 1440) is the gap between Messages that starts a new time header.
	TimeGapMinutes int `json:"timeGapMinutes"`
	// Revision is 0 until the first save.
	Revision int64 `json:"revision,string"`
}

// DefaultPreferences are what an account without saved preferences uses.
var DefaultPreferences = Preferences{Locale: "en", TimeZone: "UTC"}

// PreferencesInput replaces every editable preference.
type PreferencesInput struct {
	Locale         string
	TimeZone       string
	TimeGapMinutes int
}

// PreferenceRepository is the persistence port of preferences. Every method runs in the caller's
// transaction.
type PreferenceRepository interface {
	// ReadPreferences returns the stored preferences of the account, and false when it has none.
	ReadPreferences(ctx context.Context, tx platform.TransactionContext, account uuid.UUID) (Preferences, bool, error)
	// SavePreferences stores the preferences: expected 0 inserts and expected > 0 replaces at
	// that revision. A stale revision, or an insert where a row exists, is authz.ErrRevisionConflict.
	SavePreferences(ctx context.Context, tx platform.TransactionContext, account uuid.UUID, expected int64,
		input PreferencesInput, now time.Time) error
}

// PreferenceService reads and changes the caller's own preferences. Create it with
// NewPreferenceService; it is safe for concurrent use.
type PreferenceService struct {
	transactions Transactions
	resolver     AuthorityResolver
	clock        platform.Clock
	repository   PreferenceRepository
}

// NewPreferenceService composes the service. It fails with authz.ErrInvalidInput on a missing
// dependency.
func NewPreferenceService(transactions Transactions, resolver AuthorityResolver, clock platform.Clock,
	repository PreferenceRepository) (*PreferenceService, error) {
	if transactions == nil || resolver == nil || clock == nil || repository == nil {
		return nil, authz.ErrInvalidInput
	}
	return &PreferenceService{transactions: transactions, resolver: resolver, clock: clock, repository: repository}, nil
}

// run resolves the caller and runs operation with the caller's account in the same transaction.
// Only an interactive account acting through its own Session may use preferences: an access key
// is authz.ErrForbidden, the anonymous guest ErrLoginRequired, and the account needs the
// preference.manage-own action.
func (s *PreferenceService) run(ctx context.Context, proof Proof, mutation bool, check func(Authentication) error,
	operation func(tx platform.TransactionContext, account uuid.UUID, now time.Time) error) error {
	// The wrapper below is never nil, so WithAuthority cannot see a missing check itself.
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	var kind ProofKind
	remember := func(authentication Authentication) error {
		kind = authentication.Kind
		if check != nil {
			return check(authentication)
		}
		return nil
	}
	return WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, mutation, remember,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			switch kind {
			case SessionProof:
			case AnonymousProof:
				return ErrLoginRequired
			default:
				return authz.ErrForbidden
			}
			account, err := uuid.Parse(authority.PrincipalID)
			if err != nil || account == uuid.Nil {
				return authz.ErrForbidden
			}
			if err := authz.Authorize(authority, PreferenceAction, nil, authz.Target{AccountID: account}, now); err != nil {
				return err
			}
			return operation(tx, account, now.Truncate(time.Microsecond))
		})
}

// Read returns the caller's preferences, or DefaultPreferences at revision 0.
func (s *PreferenceService) Read(ctx context.Context, proof Proof) (Preferences, error) {
	result := DefaultPreferences
	err := s.run(ctx, proof, false, nil, func(tx platform.TransactionContext, account uuid.UUID, _ time.Time) error {
		stored, found, err := s.repository.ReadPreferences(tx.Context(), tx, account)
		if found {
			result = stored
		}
		return err
	})
	return result, err
}

// Save replaces the caller's preferences at the expected revision (0 for the first save) and
// returns them. Invalid input is authz.InvalidField naming the field; a stale revision is
// authz.ErrRevisionConflict.
func (s *PreferenceService) Save(ctx context.Context, proof Proof, expected int64, input PreferencesInput,
	check func(Authentication) error) (Preferences, error) {
	if expected < 0 {
		return Preferences{}, authz.InvalidField("expectedRevision")
	}
	input, err := validatePreferences(input)
	if err != nil {
		return Preferences{}, err
	}
	var result Preferences
	err = s.run(ctx, proof, true, check, func(tx platform.TransactionContext, account uuid.UUID, now time.Time) error {
		if err := s.repository.SavePreferences(tx.Context(), tx, account, expected, input, now); err != nil {
			return err
		}
		result = Preferences{Locale: input.Locale, TimeZone: input.TimeZone, TimeGapMinutes: input.TimeGapMinutes, Revision: expected + 1}
		return nil
	})
	return result, err
}

// validatePreferences checks every value, naming the first invalid field, and returns the input
// with the locale in canonical form.
func validatePreferences(input PreferencesInput) (PreferencesInput, error) {
	if input.Locale == "" || len(input.Locale) > maxLocaleBytes {
		return input, authz.InvalidField("locale")
	}
	tag, err := language.Parse(input.Locale)
	if err != nil || tag == language.Und {
		return input, authz.InvalidField("locale")
	}
	input.Locale = tag.String()
	if len(input.Locale) > maxLocaleBytes {
		return input, authz.InvalidField("locale")
	}
	// LoadLocation accepts "" and "Local" as the process time zone, which is no stable choice.
	if input.TimeZone == "" || input.TimeZone == "Local" || len(input.TimeZone) > maxTimeZoneBytes ||
		strings.ContainsRune(input.TimeZone, 0) {
		return input, authz.InvalidField("timeZone")
	}
	if _, err := time.LoadLocation(input.TimeZone); err != nil {
		return input, authz.InvalidField("timeZone")
	}
	if input.TimeGapMinutes < 0 || input.TimeGapMinutes > maxTimeGap {
		return input, authz.InvalidField("timeGapMinutes")
	}
	return input, nil
}
