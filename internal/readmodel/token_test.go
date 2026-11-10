package readmodel

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

func TestPageTokenRoundTripKeepsMicroseconds(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{7}, 32)
	filters, _ := Filters{Text: "alarm"}.Validate()
	caller := Caller{PrincipalID: uuid.NewString()}
	cursor := Cursor{Time: time.Date(2026, 11, 8, 12, 0, 0, 123456000, time.UTC), ID: uuid.New()}
	token := EncodePageToken(key, cursor, filters, caller)
	got, err := DecodePageToken(key, token, filters, caller)
	if err != nil || !got.Time.Equal(cursor.Time) || got.ID != cursor.ID {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}

func TestPageTokenRejections(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{7}, 32)
	filters, _ := Filters{Text: "alarm"}.Validate()
	other, _ := Filters{Text: "other"}.Validate()
	caller := Caller{PrincipalID: uuid.NewString()}
	token := EncodePageToken(key, Cursor{Time: time.Now(), ID: uuid.New()}, filters, caller)
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	flipped := append([]byte(nil), raw...)
	flipped[10] ^= 1
	truncated := raw[:len(raw)-1]
	cases := map[string]struct {
		token   string
		key     []byte
		filters Filters
		caller  Caller
	}{
		"other filters": {token, key, other, caller},
		"other caller":  {token, key, filters, Caller{PrincipalID: uuid.NewString()}},
		"same principal, other credential": {token, key, filters,
			Caller{PrincipalID: caller.PrincipalID, CredentialID: uuid.New()}},
		"flipped byte": {base64.RawURLEncoding.EncodeToString(flipped), key, filters, caller},
		"truncated":    {base64.RawURLEncoding.EncodeToString(truncated), key, filters, caller},
		"other key":    {token, bytes.Repeat([]byte{8}, 32), filters, caller},
		"not base64":   {"%%%", key, filters, caller},
		"empty":        {"", key, filters, caller},
	}
	for name, c := range cases {
		if _, err := DecodePageToken(c.key, c.token, c.filters, c.caller); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestSignedPositionRoundTripTamperAndMalformed(t *testing.T) {
	t.Parallel()
	key, other := []byte("0123456789abcdef0123456789abcdef"), []byte("fedcba9876543210fedcba9876543210")
	position := Position{TxID: 812, Seq: 3401, Issued: time.Unix(1_800_000_000, 0)}
	signed := position.Sign(key)
	if !strings.HasPrefix(signed, "812-3401-1800000000.") {
		t.Fatalf("Sign = %q", signed)
	}
	parsed, err := ParseSignedPosition(key, signed)
	if err != nil || parsed.TxID != 812 || parsed.Seq != 3401 || parsed.Issued.Unix() != 1_800_000_000 {
		t.Fatalf("parsed %+v, %v", parsed, err)
	}
	_, mac, _ := strings.Cut(signed, ".")
	renewed := "812-3401-" + strconv.FormatInt(time.Now().Unix(), 10) + "." + mac
	for name, text := range map[string]string{
		"renewed issue time": renewed, "moved coordinates": "1-3401-1800000000." + mac,
		"other installation": position.Sign(other), "unsigned": "812-3401-1800000000", "empty": "",
		"bad signature encoding": "812-3401-1800000000.!!", "malformed body": Position{}.Sign(key)[1:],
	} {
		if _, err := ParseSignedPosition(key, text); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s %q: %v", name, text, err)
		}
	}
}
