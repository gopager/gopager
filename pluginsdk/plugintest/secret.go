package plugintest

import (
	"github.com/google/uuid"
)

// Secret says where the value of a Secret configuration property comes from. Create one with
// SecretValue, SecretFile or SecretEnv.
type Secret struct {
	plaintext string
	file      string
	env       string
}

// SecretValue is a Secret stored encrypted in the in-app store. Every use creates a new stored
// row, so a changed value is a new Secret version, as in the Core.
func SecretValue(plaintext string) Secret { return Secret{plaintext: plaintext} }

// SecretFile references a file in the plugin's own directory below the harness's Secret file
// root (see WriteSecretFile). The path is relative to that directory: the harness prefixes the
// plugin ID, as an operator lays out the file root. The file is read at every attempt.
func SecretFile(path string) Secret { return Secret{file: path} }

// SecretEnv references an environment variable that SetEnv defined. The name is the part after
// the plugin's namespace: for plugin "my-hook", SecretEnv("KEY") references
// GOPAGER_PLUGIN_SECRET_MY_HOOK__KEY, as an operator names the variable.
func SecretEnv(name string) Secret { return Secret{env: name} }

// references seals the stored Secrets of one scope and returns the reference string of every
// path. owner is nil for administrative values and the subscriber for personal ones.
func (h *Harness) references(secrets map[string]Secret, owner *uuid.UUID) map[string]string {
	h.t.Helper()
	references := make(map[string]string, len(secrets))
	for path, secret := range secrets {
		switch {
		case secret.file != "":
			references[path] = "file:" + h.secretFilePath(secret.file)
			continue
		case secret.env != "":
			references[path] = "env:" + h.secretEnvName(secret.env)
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
