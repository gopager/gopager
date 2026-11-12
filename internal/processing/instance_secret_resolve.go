package processing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/google/uuid"
)

// MaxSecretFileBytes bounds a referenced Secret file.
const MaxSecretFileBytes = 64 << 10

// SecretRequest asks for the plaintext of the Secret at a dotted configuration path. Owner is the
// subscriber who owns the value, nil for administrative values; a stored row must belong to it.
type SecretRequest struct {
	Path      string
	Reference SecretReference
	Owner     *uuid.UUID
}

// ResolvedSecrets holds the plaintext of one invocation and the version each value was read at.
type ResolvedSecrets struct {
	// Values maps a dotted path to its plaintext.
	Values map[string][]byte
	// Versions maps a dotted path to secret:<row ID>, file:<16 hex digest> or env:<NAME>.
	Versions map[string]string
}

// Wipe overwrites and forgets the plaintext; callers do it as soon as the invocation ended.
func (r *ResolvedSecrets) Wipe() {
	for _, value := range r.Values {
		clear(value)
	}
	clear(r.Values)
}

// SecretResolver turns Secret references into plaintext. A nil Cipher makes every stored row
// unavailable, a nil Files every file reference, a nil LookupEnv every environment reference.
type SecretResolver struct {
	Cipher    *SecretCipher
	Files     *os.Root
	LookupEnv func(string) (string, bool)
}

// Resolve reads each request: stored rows come from rows and must belong to the instance and the
// requested owner, files are read through the Secret file root, variables through LookupEnv; files
// and variables must lie in the namespace of pluginID. Any failure is AUTHORITY_UNAVAILABLE
// without detail and leaves no plaintext behind.
func (r SecretResolver) Resolve(pluginID string, instanceID uuid.UUID, requests []SecretRequest,
	rows map[uuid.UUID]StoredSecret) (ResolvedSecrets, error) {
	resolved := ResolvedSecrets{Values: make(map[string][]byte, len(requests)), Versions: make(map[string]string, len(requests))}
	for _, request := range requests {
		value, version, err := r.resolveOne(pluginID, instanceID, request, rows)
		if err != nil {
			resolved.Wipe()
			return ResolvedSecrets{}, err
		}
		resolved.Values[request.Path], resolved.Versions[request.Path] = value, version
	}
	return resolved, nil
}

// errSecretUnavailable is the detail-free cause of every resolution failure.
var errSecretUnavailable = errors.New("secret unavailable")

func (r SecretResolver) resolveOne(pluginID string, instanceID uuid.UUID, request SecretRequest,
	rows map[uuid.UUID]StoredSecret) ([]byte, string, error) {
	var value []byte
	var version string
	var err error
	// Files and environment variables are placed by the operator for administrative
	// configuration; a personal value may only use a Secret its owner stored, so a User cannot
	// route an operator-placed Secret into their own recipient settings.
	if request.Owner != nil && request.Reference.Source != SecretStored {
		return nil, "", capabilityUnavailable(errSecretUnavailable)
	}
	// A revision saved before references were bound to a plugin must not leak another plugin's Secret.
	if !request.Reference.BelongsTo(pluginID) {
		return nil, "", capabilityUnavailable(errSecretUnavailable)
	}
	switch request.Reference.Source {
	case SecretStored:
		value, version, err = r.resolveStored(instanceID, request, rows)
	case SecretFile:
		value, version, err = r.resolveFile(request.Reference.Path)
	case SecretEnv:
		value, version, err = r.resolveEnv(request.Reference.Name)
	default:
		err = errSecretUnavailable
	}
	if err != nil {
		return nil, "", capabilityUnavailable(errSecretUnavailable)
	}
	return value, version, nil
}

func (r SecretResolver) resolveStored(instanceID uuid.UUID, request SecretRequest,
	rows map[uuid.UUID]StoredSecret) ([]byte, string, error) {
	row, found := rows[request.Reference.ID]
	if !found || r.Cipher == nil || row.ID != request.Reference.ID || row.InstanceID != instanceID ||
		(row.OwnerID == nil) != (request.Owner == nil) || (row.OwnerID != nil && *row.OwnerID != *request.Owner) {
		return nil, "", errSecretUnavailable
	}
	plain, err := r.Cipher.Open(row)
	if err != nil {
		return nil, "", err
	}
	return plain, "secret:" + row.ID.String(), nil
}

func (r SecretResolver) resolveEnv(name string) ([]byte, string, error) {
	if r.LookupEnv == nil {
		return nil, "", errSecretUnavailable
	}
	value, found := r.LookupEnv(name)
	if !found || value == "" {
		return nil, "", errSecretUnavailable
	}
	return []byte(value), "env:" + name, nil
}

// resolveFile reads a regular file below the Secret file root, drops one trailing newline and
// versions the value by the digest of the raw content.
func (r SecretResolver) resolveFile(name string) ([]byte, string, error) {
	if r.Files == nil {
		return nil, "", errSecretUnavailable
	}
	info, err := r.Files.Stat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxSecretFileBytes {
		return nil, "", errSecretUnavailable
	}
	file, err := r.Files.Open(name)
	if err != nil {
		return nil, "", errSecretUnavailable
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, MaxSecretFileBytes+1))
	if err != nil || len(content) > MaxSecretFileBytes {
		return nil, "", errSecretUnavailable
	}
	sum := sha256.Sum256(content)
	value := content
	if trimmed, hadNewline := bytes.CutSuffix(content, []byte("\n")); hadNewline {
		value = bytes.TrimSuffix(trimmed, []byte("\r"))
	}
	if len(value) == 0 {
		clear(content)
		return nil, "", errSecretUnavailable
	}
	return value, "file:" + hex.EncodeToString(sum[:])[:16], nil
}
