package readmodel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

// Cursor is the keyset position after the last Message of a page, in feed order.
type Cursor struct {
	Time time.Time
	ID   uuid.UUID
}

// Caller identifies who a page token was issued to: the Principal and, for an access key, the
// key's credential, so a token cannot be replayed by another caller.
type Caller struct {
	PrincipalID  string
	CredentialID uuid.UUID
}

// NewCaller derives the token identity from an authentication.
func NewCaller(authentication identity.Authentication) Caller {
	caller := Caller{PrincipalID: authentication.Authority.PrincipalID}
	if authentication.Kind == identity.AccessKeyProof {
		caller.CredentialID = authentication.CredentialID
	}
	return caller
}

// hash binds the identity into a token.
func (c Caller) hash() [32]byte {
	return sha256.Sum256([]byte(c.PrincipalID + "\x00" + c.CredentialID.String()))
}

// tokenVersion is the current page token layout.
const tokenVersion = 1

// tokenPayloadSize is version, time in microseconds, ID, filter hash and caller hash.
const tokenPayloadSize = 1 + 8 + 16 + 32 + 32

// EncodePageToken returns base64url(payload ‖ HMAC-SHA256(key, payload)) for the cursor,
// bound to the filters and the caller. The Message time is kept in microseconds, the
// precision of the stored time.
func EncodePageToken(key []byte, cursor Cursor, filters Filters, caller Caller) string {
	payload := make([]byte, 0, tokenPayloadSize+sha256.Size)
	payload = append(payload, tokenVersion)
	payload = binary.BigEndian.AppendUint64(payload, uint64(cursor.Time.UnixMicro()))
	payload = append(payload, cursor.ID[:]...)
	filterHash := filters.Hash()
	callerHash := caller.hash()
	payload = append(payload, filterHash[:]...)
	payload = append(payload, callerHash[:]...)
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(payload))
}

// DecodePageToken verifies a token and returns its cursor. A malformed or tampered token, one
// issued for other filters or to another caller, is authz.ErrInvalidInput.
func DecodePageToken(key []byte, token string, filters Filters, caller Caller) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenPayloadSize+sha256.Size {
		return Cursor{}, authz.ErrInvalidInput
	}
	payload, signature := raw[:tokenPayloadSize], raw[tokenPayloadSize:]
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) || payload[0] != tokenVersion {
		return Cursor{}, authz.ErrInvalidInput
	}
	filterHash := filters.Hash()
	callerHash := caller.hash()
	if !hmac.Equal(payload[25:57], filterHash[:]) || !hmac.Equal(payload[57:89], callerHash[:]) {
		return Cursor{}, authz.ErrInvalidInput
	}
	cursor := Cursor{Time: time.UnixMicro(int64(binary.BigEndian.Uint64(payload[1:9]))).UTC()}
	copy(cursor.ID[:], payload[9:25])
	return cursor, nil
}

// Position is the cutoff of a feed page in the domain-event order: every fact of a
// transaction the page could not see lies after (TxID, Seq). Issued is the primary-clock time
// the position was handed out, which bounds how long a stream may resume from it.
type Position struct {
	TxID   int64
	Seq    int64
	Issued time.Time
}

// positionDomain separates position signatures from page tokens signed with the same secret.
const positionDomain = "live-position\x00"

// Sign renders the opaque form `<txId>-<seq>-<issuedUnixSeconds>.<mac>`. The mac is the
// base64url HMAC-SHA256 of the part before the dot under the installation's token secret, so a
// client can neither move the coordinates nor renew the issue time that bounds the replay window.
func (p Position) Sign(key []byte) string {
	body := p.body()
	return body + "." + base64.RawURLEncoding.EncodeToString(positionMAC(key, body))
}

// body renders the signed part of a position.
func (p Position) body() string {
	return strconv.FormatInt(p.TxID, 10) + "-" + strconv.FormatInt(p.Seq, 10) + "-" + strconv.FormatInt(p.Issued.Unix(), 10)
}

func positionMAC(key []byte, body string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(positionDomain))
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

// ParseSignedPosition reads a position produced by Sign with the same key. A malformed,
// unsigned or tampered position is authz.ErrInvalidInput.
func ParseSignedPosition(key []byte, text string) (Position, error) {
	body, signature, found := strings.Cut(text, ".")
	if !found {
		return Position{}, authz.ErrInvalidInput
	}
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(got, positionMAC(key, body)) {
		return Position{}, authz.ErrInvalidInput
	}
	parts := strings.Split(body, "-")
	if len(parts) != 3 {
		return Position{}, authz.ErrInvalidInput
	}
	var numbers [3]int64
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value < 0 || strconv.FormatInt(value, 10) != part {
			return Position{}, authz.ErrInvalidInput
		}
		numbers[i] = value
	}
	return Position{TxID: numbers[0], Seq: numbers[1], Issued: time.Unix(numbers[2], 0).UTC()}, nil
}
