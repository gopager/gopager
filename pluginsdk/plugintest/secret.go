package plugintest

import (
	"github.com/google/uuid"
)

// Secret says where the value of a Secret configuration property comes from. Create one with
// SecretValue, SecretFile or SecretEnv.
type Secret struct {
	plaintext string
	reference string
}

// SecretValue is a Secret stored encrypted in the in-app store. Every use creates a new stored
// row, so a changed value is a new Secret version, as in the Core.
func SecretValue(plaintext string) Secret { return Secret{plaintext: plaintext} }

// SecretFile references a file below the harness's Secret file root (see WriteSecretFile). The
// file is read at every attempt.
func SecretFile(path string) Secret { return Secret{reference: "file:" + path} }

// SecretEnv references an environment variable that SetEnv defined. The Core only accepts names
// that start with processing.SecretEnvPrefix.
func SecretEnv(name string) Secret { return Secret{reference: "env:" + name} }

// references seals the stored Secrets of one scope and returns the reference string of every
// path. owner is nil for administrative values and the subscriber for personal ones.
func (h *Harness) references(secrets map[string]Secret, owner *uuid.UUID) map[string]string {
	h.t.Helper()
	references := make(map[string]string, len(secrets))
	for path, secret := range secrets {
		if secret.reference != "" {
			references[path] = secret.reference
			continue
		}
		id := uuid.New()
		row, err := h.cipher.Seal(id, h.instanceID, owner, []byte(secret.plaintext))
		if err != nil {
			h.t.Fatalf("plugintest: seal secret %s: %v", path, err)
		}
		h.stores.putSecret(row)
		references[path] = "secret:" + id.String()
	}
	return references
}
