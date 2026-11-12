package processing

import (
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// SecretSource says where a Secret property's value lives.
type SecretSource string

// The Secret sources, also the values of ConfigurationFieldView.Source.
const (
	// SecretStored is a row of the encrypted in-app store (secret:<uuid>).
	SecretStored SecretSource = "stored"
	// SecretFile is a file below the operator's Secret file root (file:<path>).
	SecretFile SecretSource = "file"
	// SecretEnv is a plugin Secret environment variable (env:<NAME>).
	SecretEnv SecretSource = "env"
)

// SecretEnvPrefix is the mandatory prefix of referenced environment variables, so an administrator
// cannot forward Core settings such as the database URL to a plugin.
const SecretEnvPrefix = "GOPAGER_PLUGIN_SECRET_"

// maxSecretPathBytes bounds a file reference's path.
const maxSecretPathBytes = 1024

var secretEnvName = regexp.MustCompile(`^GOPAGER_PLUGIN_SECRET_[A-Z0-9_]+$`)

// SecretReference is a parsed Secret property value. Only the member of its Source is set.
type SecretReference struct {
	Source SecretSource
	ID     uuid.UUID
	Path   string
	Name   string
}

// ParseSecretReference parses secret:<uuid>, file:<relative/path> or env:<NAME>. Anything else,
// including non-canonical spellings, is invalid input.
func ParseSecretReference(encoded string) (SecretReference, error) {
	scheme, rest, found := strings.Cut(encoded, ":")
	if !found {
		return SecretReference{}, invalid(errors.New("invalid secret reference"))
	}
	switch scheme {
	case "secret":
		id, err := uuid.Parse(rest)
		if err != nil || id == uuid.Nil || id.String() != rest {
			return SecretReference{}, invalid(errors.New("invalid secret reference"))
		}
		return SecretReference{Source: SecretStored, ID: id}, nil
	case "file":
		if rest == "" || len(rest) > maxSecretPathBytes || strings.ContainsAny(rest, "\x00\\") ||
			path.Clean(rest) != rest || !filepath.IsLocal(filepath.FromSlash(rest)) {
			return SecretReference{}, invalid(errors.New("invalid secret reference"))
		}
		return SecretReference{Source: SecretFile, Path: rest}, nil
	case "env":
		if !secretEnvName.MatchString(rest) {
			return SecretReference{}, invalid(errors.New("invalid secret reference"))
		}
		return SecretReference{Source: SecretEnv, Name: rest}, nil
	}
	return SecretReference{}, invalid(errors.New("invalid secret reference"))
}
