package readmodel

import (
	"bytes"
	"encoding/base64"
	"errors"
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

func TestPositionRoundTripAndMalformed(t *testing.T) {
	t.Parallel()
	position := Position{TxID: 812, Seq: 3401, Issued: time.Unix(1_800_000_000, 0)}
	if position.String() != "812-3401-1800000000" {
		t.Fatalf("String = %q", position.String())
	}
	parsed, err := ParsePosition(position.String())
	if err != nil || parsed.TxID != 812 || parsed.Seq != 3401 || parsed.Issued.Unix() != 1_800_000_000 {
		t.Fatalf("parsed %+v, %v", parsed, err)
	}
	for _, text := range []string{"", "1-2", "1-2-3-4", "a-2-3", "1--3", "-1-2-3", "01-2-3", "1-2-3 "} {
		if _, err := ParsePosition(text); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%q: %v", text, err)
		}
	}
}
