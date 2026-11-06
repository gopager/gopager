// Package identity owns local accounts, credentials and the derivation of a
// caller's current authority from transport proof.
//
// HTTPTransport.SelectProof turns a request into a Proof (bearer access key,
// session cookie or anonymous); Resolver.Resolve validates that proof inside
// the caller's transaction and returns the Authentication with current grants.
// LoginService implements local password login, logout and session reads, and
// NewHTTPHandler exposes them over HTTP. GatewayAuthenticator authenticates
// ingest gateways separately from principals. Bootstrap initializes a fresh
// installation. The helpers in this file validate usernames and passwords and
// hash passwords (Argon2id) and bearer secrets (SHA-256).
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gopager/gopager/internal/platform"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Sentinel errors of the identity package.
var (
	// ErrInvalidInput reports malformed caller input (username, password,
	// secret, or missing dependencies); HTTP maps it to INVALID_INPUT. Like
	// ErrLoginRequired it is a *platform.Failure so its code survives a
	// transaction boundary.
	ErrInvalidInput error = platform.NewFailure(platform.CodeInvalidInput, nil)
	// ErrLoginRequired reports missing, malformed, unknown, expired, revoked or
	// otherwise unusable proof, or an inactive account. It deliberately does
	// not say which; HTTP maps it to LOGIN_REQUIRED (401).
	ErrLoginRequired error = platform.NewFailure(platform.CodeLoginRequired, nil)
)

const (
	passwordMemory      = 19 * 1024
	passwordIterations  = 2
	passwordParallelism = 1
)

// UsernameKey validates a display username and returns its locale-independent identity key.
func UsernameKey(username string) (string, error) {
	if !utf8.ValidString(username) || len(username) > 512 || utf8.RuneCountInString(username) > 128 ||
		username == "" || strings.TrimSpace(username) != username {
		return "", ErrInvalidInput
	}
	for _, r := range username {
		if unicode.IsControl(r) {
			return "", ErrInvalidInput
		}
	}
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(username))), nil
}

// ValidatePassword checks the agreed limits without changing any password byte.
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || len(password) > 1024 || utf8.RuneCountInString(password) < 12 {
		return ErrInvalidInput
	}
	return nil
}

// PasswordParams are the Argon2id cost parameters of a password hash.
type PasswordParams struct {
	// MemoryKiB is the memory cost in KiB.
	MemoryKiB uint32
	// Iterations is the time cost.
	Iterations uint32
	// Parallelism is the number of lanes.
	Parallelism uint8
}

// ProductionPasswordParams are the cost parameters used for every stored password
// hash outside tests.
var ProductionPasswordParams = PasswordParams{
	MemoryKiB: passwordMemory, Iterations: passwordIterations, Parallelism: passwordParallelism,
}

// PasswordHasher hashes passwords with its own cost parameters and verifies hashes
// encoded with either those parameters or ProductionPasswordParams. It accepts no
// other parameters, so a stored hash can never make verification allocate an
// unbounded amount of memory. The zero value uses ProductionPasswordParams.
type PasswordHasher struct {
	params PasswordParams
}

// NewPasswordHasher returns a hasher for the given parameters. Production composition
// uses the zero PasswordHasher; reduced parameters exist only so tests can avoid the
// production Argon2id cost. It returns ErrInvalidInput for zero parameters.
func NewPasswordHasher(params PasswordParams) (PasswordHasher, error) {
	if params.MemoryKiB == 0 || params.Iterations == 0 || params.Parallelism == 0 {
		return PasswordHasher{}, ErrInvalidInput
	}
	return PasswordHasher{params: params}, nil
}

// effective returns the configured parameters, defaulting to production.
func (h PasswordHasher) effective() PasswordParams {
	if h.params == (PasswordParams{}) {
		return ProductionPasswordParams
	}
	return h.params
}

// Hash returns an encoded, salted Argon2id hash for a validated password.
func (h PasswordHasher) Hash(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	params := h.effective()
	hash := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, 32)
	return fmt.Sprintf("$argon2id$v=%d$%s$%s$%s", argon2.Version, params.encoding(),
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// encoding returns the parameter segment of an encoded hash.
func (p PasswordParams) encoding() string {
	return fmt.Sprintf("m=%d,t=%d,p=%d", p.MemoryKiB, p.Iterations, p.Parallelism)
}

// Verify checks an encoded approved hash without normalizing the supplied password.
func (h PasswordHasher) Verify(encoded, password string) (bool, error) {
	if !utf8.ValidString(password) || len(password) > 1024 {
		return false, nil
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return false, fmt.Errorf("unsupported stored password encoding")
	}
	var params PasswordParams
	switch parts[3] {
	case ProductionPasswordParams.encoding():
		params = ProductionPasswordParams
	case h.effective().encoding():
		params = h.effective()
	default:
		return false, fmt.Errorf("unsupported stored password encoding")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, fmt.Errorf("invalid stored password salt")
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(want) != 32 {
		return false, fmt.Errorf("invalid stored password digest")
	}
	got := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, 32)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// HashPassword returns an encoded, salted Argon2id hash for a validated password
// using ProductionPasswordParams.
func HashPassword(password string) (string, error) {
	return PasswordHasher{}.Hash(password)
}

// VerifyPassword checks an encoded approved hash without normalizing the supplied password.
func VerifyPassword(encoded, password string) (bool, error) {
	return PasswordHasher{}.Verify(encoded, password)
}

// NewSecret creates a 256-bit URL-safe credential and its storage-only hash.
func NewSecret() (string, [32]byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", [32]byte{}, fmt.Errorf("generate credential: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(value)
	return secret, sha256.Sum256([]byte(secret)), nil
}

// SecretHash validates the exact credential encoding before deriving its lookup hash.
func SecretHash(secret string) ([32]byte, error) {
	value, err := base64.RawURLEncoding.Strict().DecodeString(secret)
	if err != nil || len(value) != 32 || len(secret) != 43 {
		return [32]byte{}, ErrLoginRequired
	}
	return sha256.Sum256([]byte(secret)), nil
}
