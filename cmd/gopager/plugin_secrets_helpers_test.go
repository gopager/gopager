package main

import (
	"os"
	"path/filepath"

	"github.com/gopager/gopager/internal/processing"
)

// newTestCipher builds a cipher over a fixed test key.
func newTestCipher() (*processing.SecretCipher, error) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return processing.NewSecretCipher(key)
}

// removePlugin deletes the plugin executable from a provisioned directory.
func removePlugin(directory string) error { return os.Remove(filepath.Join(directory, "plugin")) }
