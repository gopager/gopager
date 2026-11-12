package processing

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

// secretKeyBytes is the AES-256 master key size.
const secretKeyBytes = 32

// secretNonceBytes is the AES-GCM nonce size.
const secretNonceBytes = 12

// secretRedacted is what every textual form of a SecretCipher shows.
const secretRedacted = "SecretCipher(redacted)"

// ParseSecretKey decodes the operator-supplied master key: standard padded Base64 of exactly 32
// bytes, with surrounding whitespace ignored. Errors never include the input.
func ParseSecretKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != secretKeyBytes {
		return nil, invalid(errors.New("secret key must be standard Base64 of 32 bytes"))
	}
	return key, nil
}

// StoredSecret is one immutable row of the encrypted in-app Secret store. OwnerID is nil for
// administrative values. CreatedAt is assigned by the store, not by the cipher.
type StoredSecret struct {
	ID         uuid.UUID
	InstanceID uuid.UUID
	OwnerID    *uuid.UUID
	KeyID      string
	Nonce      []byte
	Ciphertext []byte
	CreatedAt  time.Time
}

// SecretStore reads stored Secret rows inside the caller's transaction.
type SecretStore interface {
	// ReadSecrets returns the rows that exist for ids; unknown IDs are absent from the result.
	ReadSecrets(ctx context.Context, tx platform.TransactionContext, ids []uuid.UUID) (map[uuid.UUID]StoredSecret, error)
}

// SecretCipher seals and opens Secret rows with AES-256-GCM. The row ID, instance ID and owner ID
// are authenticated, so a row cannot be moved to another instance, owner or identity. It never
// reveals its key: every textual form is redacted.
type SecretCipher struct {
	aead  cipher.AEAD
	keyID string
	// fingerprintKey is derived from the master key for keyed command fingerprints.
	fingerprintKey []byte
}

// NewSecretCipher builds a cipher from a 32-byte master key; the key is not retained.
func NewSecretCipher(key []byte) (*SecretCipher, error) {
	if len(key) != secretKeyBytes {
		return nil, invalid(errors.New("secret key must be 32 bytes"))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, invalid(errors.New("secret key rejected"))
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, invalid(errors.New("secret cipher unavailable"))
	}
	sum := sha256.Sum256(key)
	return &SecretCipher{aead: aead, keyID: "k1:" + hex.EncodeToString(sum[:])[:16],
		fingerprintKey: deriveFingerprintKey(key)}, nil
}

// KeyID identifies the master key without revealing it: "k1:" plus the first 16 hex digits of
// its SHA-256.
func (c SecretCipher) KeyID() string { return c.keyID }

// String redacts the cipher.
func (SecretCipher) String() string { return secretRedacted }

// GoString redacts the cipher for %#v.
func (SecretCipher) GoString() string { return secretRedacted }

// LogValue redacts the cipher in structured logs.
func (SecretCipher) LogValue() slog.Value { return slog.StringValue(secretRedacted) }

// associatedData binds a row to its identity: row, instance and owner IDs as fixed-width bytes;
// the owner is absent for administrative values.
func associatedData(id, instanceID uuid.UUID, ownerID *uuid.UUID) []byte {
	data := make([]byte, 0, 48)
	data = append(data, id[:]...)
	data = append(data, instanceID[:]...)
	if ownerID != nil {
		data = append(data, ownerID[:]...)
	}
	return data
}

// Seal encrypts plaintext into a new row with a fresh random nonce.
func (c *SecretCipher) Seal(id, instanceID uuid.UUID, ownerID *uuid.UUID, plaintext []byte) (StoredSecret, error) {
	if c == nil || id == uuid.Nil || instanceID == uuid.Nil || (ownerID != nil && *ownerID == uuid.Nil) {
		return StoredSecret{}, invalid(errors.New("invalid secret identity"))
	}
	nonce := make([]byte, secretNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return StoredSecret{}, capabilityUnavailable(errors.New("secret nonce unavailable"))
	}
	var owner *uuid.UUID
	if ownerID != nil {
		copied := *ownerID
		owner = &copied
	}
	return StoredSecret{ID: id, InstanceID: instanceID, OwnerID: owner, KeyID: c.keyID, Nonce: nonce,
		Ciphertext: c.aead.Seal(nil, nonce, plaintext, associatedData(id, instanceID, ownerID))}, nil
}

// Open decrypts a row. A different key, a changed identity or any modification fails with
// AUTHORITY_UNAVAILABLE and no detail.
func (c *SecretCipher) Open(stored StoredSecret) ([]byte, error) {
	if c == nil || stored.KeyID != c.keyID || len(stored.Nonce) != secretNonceBytes {
		return nil, capabilityUnavailable(errors.New("secret cannot be opened"))
	}
	plaintext, err := c.aead.Open(nil, stored.Nonce, stored.Ciphertext, associatedData(stored.ID, stored.InstanceID, stored.OwnerID))
	if err != nil {
		return nil, capabilityUnavailable(errors.New("secret cannot be opened"))
	}
	return plaintext, nil
}
