package processing

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

func testKey(fill byte) []byte { return bytes.Repeat([]byte{fill}, 32) }

func testCipher(t *testing.T, fill byte) *SecretCipher {
	t.Helper()
	cipher, err := NewSecretCipher(testKey(fill))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func TestParseSecretKey(t *testing.T) {
	t.Parallel()
	encoded := base64.StdEncoding.EncodeToString(testKey(7))
	key, err := ParseSecretKey("  " + encoded + "\n")
	if err != nil || !bytes.Equal(key, testKey(7)) {
		t.Fatalf("valid key rejected: %v", err)
	}
	for name, input := range map[string]string{
		"empty":     "",
		"short":     base64.StdEncoding.EncodeToString(testKey(1)[:16]),
		"long":      base64.StdEncoding.EncodeToString(append(testKey(1), 1)),
		"not b64":   "%%%",
		"url-safe":  strings.NewReplacer("+", "-", "/", "_").Replace(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32))),
		"unpadded":  strings.TrimRight(encoded, "="),
		"inner gap": encoded[:10] + " " + encoded[10:],
	} {
		if _, err := ParseSecretKey(input); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	if _, err := NewSecretCipher(testKey(1)[:5]); err == nil {
		t.Fatal("short key accepted by cipher")
	}
}

func TestSecretCipherRoundTripAndBinding(t *testing.T) {
	t.Parallel()
	cipher := testCipher(t, 1)
	sum := sha256.Sum256(testKey(1))
	if want := "k1:" + hex.EncodeToString(sum[:])[:16]; cipher.KeyID() != want {
		t.Fatalf("key ID %q, want %q", cipher.KeyID(), want)
	}
	id, instance, owner := uuid.New(), uuid.New(), uuid.New()
	for _, ownerID := range []*uuid.UUID{nil, &owner} {
		stored, err := cipher.Seal(id, instance, ownerID, []byte("hunter2"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(stored.Ciphertext, []byte("hunter2")) || len(stored.Nonce) != 12 {
			t.Fatal("ciphertext leaks plaintext or nonce has wrong size")
		}
		plain, err := cipher.Open(stored)
		if err != nil || string(plain) != "hunter2" {
			t.Fatalf("round trip failed: %q %v", plain, err)
		}
		again, err := cipher.Seal(id, instance, ownerID, []byte("hunter2"))
		if err != nil || bytes.Equal(again.Nonce, stored.Nonce) {
			t.Fatalf("nonce reused: %v", err)
		}
		tamper := func(mutate func(*StoredSecret)) {
			t.Helper()
			copied := stored
			copied.Ciphertext = bytes.Clone(stored.Ciphertext)
			mutate(&copied)
			if _, err := cipher.Open(copied); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
				t.Fatalf("tampered secret opened: %v", err)
			}
		}
		tamper(func(s *StoredSecret) { s.Ciphertext[0] ^= 1 })
		tamper(func(s *StoredSecret) { s.ID = uuid.New() })
		tamper(func(s *StoredSecret) { s.InstanceID = uuid.New() })
		tamper(func(s *StoredSecret) {
			other := uuid.New()
			s.OwnerID = &other
		})
		tamper(func(s *StoredSecret) { s.KeyID = "k1:0000000000000000" })
		if ownerID == nil {
			tamper(func(s *StoredSecret) { s.OwnerID = &owner })
		}
	}
	stored, _ := cipher.Seal(id, instance, nil, []byte("x"))
	if _, err := testCipher(t, 2).Open(stored); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("wrong key opened the secret: %v", err)
	}
}

func TestSecretCipherNeverPrinted(t *testing.T) {
	t.Parallel()
	cipher := testCipher(t, 9)
	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("x", "cipher", cipher, "value", *cipher)
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("x", "cipher", cipher)
	printed := fmt.Sprintf("%v %+v %#v %s %v %+v %#v", cipher, cipher, cipher, cipher, *cipher, *cipher, *cipher) + logs.String()
	for _, secret := range []string{string(testKey(9)), base64.StdEncoding.EncodeToString(testKey(9)),
		hex.EncodeToString(testKey(9))} {
		if strings.Contains(printed, secret) {
			t.Fatalf("key material printed: %q", printed)
		}
	}
	if !strings.Contains(printed, "redacted") {
		t.Fatalf("no redaction marker: %q", printed)
	}
}

func TestParseSecretReference(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	valid := map[string]SecretReference{
		"secret:" + id.String():            {Source: SecretStored, ID: id},
		"file:tokens/api.txt":              {Source: SecretFile, Path: "tokens/api.txt"},
		"env:GOPAGER_PLUGIN_SECRET_TOKEN1": {Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_TOKEN1"},
	}
	for input, want := range valid {
		got, err := ParseSecretReference(input)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v", input, got, err)
		}
	}
	invalid := []string{"", "plain", "secret:", "secret:not-a-uuid", "secret:" + uuid.Nil.String(),
		"secret:" + strings.ToUpper(id.String()), "secret:{" + id.String() + "}",
		"file:", "file:/etc/passwd", "file:../x", "file:a/../../x", "file:a\x00b", "file:a\\b", "file:./a", "file:a//b", "file:a/",
		"env:", "env:PATH", "env:GOPAGER_DATABASE_URL", "env:GOPAGER_PLUGIN_SECRET_", "env:GOPAGER_PLUGIN_SECRET_lower",
		"http://x", "file:" + strings.Repeat("a", 5000)}
	for _, input := range invalid {
		if _, err := ParseSecretReference(input); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%q accepted: %v", input, err)
		}
	}
}

func resolverFixture(t *testing.T) (SecretResolver, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	env := map[string]string{"GOPAGER_PLUGIN_SECRET_A": "env-value", "GOPAGER_PLUGIN_SECRET_EMPTY": ""}
	return SecretResolver{Cipher: testCipher(t, 3), Files: root, LookupEnv: func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}}, dir
}

func TestSecretResolver(t *testing.T) {
	t.Parallel()
	resolver, dir := resolverFixture(t)
	instance, owner := uuid.New(), uuid.New()
	row := uuid.New()
	stored, err := resolver.Cipher.Seal(row, instance, &owner, []byte("personal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api.txt"), []byte("file-value\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := []SecretRequest{
		{Path: "personal", Reference: SecretReference{Source: SecretStored, ID: row}, Owner: &owner},
		{Path: "auth.file", Reference: SecretReference{Source: SecretFile, Path: "api.txt"}},
		{Path: "auth.env", Reference: SecretReference{Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_A"}},
	}
	rows := map[uuid.UUID]StoredSecret{row: stored}
	resolved, err := resolver.Resolve(instance, requests, rows)
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved.Values["personal"]) != "personal" || string(resolved.Values["auth.file"]) != "file-value" ||
		string(resolved.Values["auth.env"]) != "env-value" {
		t.Fatalf("wrong values: %v", resolved.Values)
	}
	sum := sha256.Sum256([]byte("file-value\r\n"))
	if resolved.Versions["personal"] != "secret:"+row.String() ||
		resolved.Versions["auth.file"] != "file:"+hex.EncodeToString(sum[:])[:16] ||
		resolved.Versions["auth.env"] != "env:GOPAGER_PLUGIN_SECRET_A" {
		t.Fatalf("wrong versions: %v", resolved.Versions)
	}
	resolved.Wipe()
	if len(resolved.Values) != 0 {
		t.Fatal("Wipe kept values")
	}
	if err := os.WriteFile(filepath.Join(dir, "api.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := resolver.Resolve(instance, requests[1:2], nil)
	if err != nil || next.Versions["auth.file"] == "file:"+hex.EncodeToString(sum[:])[:16] {
		t.Fatalf("file version did not follow the content: %v %v", next.Versions, err)
	}
}

func TestSecretResolverFailuresAreUnavailable(t *testing.T) {
	t.Parallel()
	resolver, dir := resolverFixture(t)
	instance, owner := uuid.New(), uuid.New()
	row := uuid.New()
	stored, _ := resolver.Cipher.Seal(row, instance, &owner, []byte("v"))
	if err := os.WriteFile(filepath.Join(dir, "big"), bytes.Repeat([]byte("a"), MaxSecretFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty"), []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("file-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	wrongInstance, _ := resolver.Cipher.Seal(row, uuid.New(), &owner, []byte("v"))
	cases := map[string]struct {
		request SecretRequest
		rows    map[uuid.UUID]StoredSecret
		mutate  func(*SecretResolver)
	}{
		"missing file":    {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "nope"}}},
		"oversized file":  {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "big"}}},
		"empty file":      {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "empty"}}},
		"directory":       {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "dir"}}},
		"symlink escape":  {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "escape"}}},
		"no file root":    {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "big"}}, mutate: func(r *SecretResolver) { r.Files = nil }},
		"unset env":       {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_NOPE"}}},
		"empty env":       {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_EMPTY"}}},
		"no env lookup":   {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_A"}}, mutate: func(r *SecretResolver) { r.LookupEnv = nil }},
		"missing row":     {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretStored, ID: uuid.New()}, Owner: &owner}, rows: map[uuid.UUID]StoredSecret{row: stored}},
		"no cipher":       {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretStored, ID: row}, Owner: &owner}, rows: map[uuid.UUID]StoredSecret{row: stored}, mutate: func(r *SecretResolver) { r.Cipher = nil }},
		"foreign owner":   {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretStored, ID: row}, Owner: &other}, rows: map[uuid.UUID]StoredSecret{row: stored}},
		"admin used row":  {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretStored, ID: row}}, rows: map[uuid.UUID]StoredSecret{row: stored}},
		"personal env":    {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_A"}, Owner: &owner}},
		"personal file":   {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretFile, Path: "token"}, Owner: &owner}},
		"foreign instace": {request: SecretRequest{Path: "a", Reference: SecretReference{Source: SecretStored, ID: row}, Owner: &owner}, rows: map[uuid.UUID]StoredSecret{row: wrongInstance}},
	}
	for name, tc := range cases {
		local := resolver
		if tc.mutate != nil {
			tc.mutate(&local)
		}
		_, err := local.Resolve(instance, []SecretRequest{tc.request}, tc.rows)
		if platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
			t.Errorf("%s: %v", name, err)
		}
	}
}
