package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

// secretEnvironment returns an environment lookup with the database URL and the given variables.
func secretEnvironment(values map[string]string) func(string) string {
	return func(key string) string {
		if key == "GOPAGER_DATABASE_URL" {
			return "synthetic-dsn"
		}
		return values[key]
	}
}

func encodedKey(fill byte) string {
	key := make([]byte, 32)
	for i := range key {
		key[i] = fill
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestParseConfigPluginSecrets(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	keyFile := filepath.Join(directory, "key")
	if err := os.WriteFile(keyFile, []byte(" "+encodedKey(2)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	badFile := filepath.Join(directory, "bad")
	if err := os.WriteFile(badFile, []byte("not-a-key-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--roles=worker"}
	t.Run("unset means no key and no root", func(t *testing.T) {
		t.Parallel()
		config, err := parseConfig(args, secretEnvironment(nil))
		if err != nil || config.PluginSecrets.Cipher != nil || config.PluginSecrets.FileRoot != "" {
			t.Fatalf("defaults wrong: %+v, %v", config.PluginSecrets, err)
		}
	})
	t.Run("key from variable", func(t *testing.T) {
		t.Parallel()
		config, err := parseConfig(args, secretEnvironment(map[string]string{"GOPAGER_SECRET_KEY": encodedKey(1)}))
		if err != nil || config.PluginSecrets.Cipher == nil {
			t.Fatalf("key not parsed: %v", err)
		}
	})
	t.Run("key file wins over variable", func(t *testing.T) {
		t.Parallel()
		fromFile, err := parseConfig(args, secretEnvironment(map[string]string{
			"GOPAGER_SECRET_KEY": encodedKey(1), "GOPAGER_SECRET_KEY_FILE": keyFile}))
		if err != nil {
			t.Fatal(err)
		}
		direct, err := parseConfig(args, secretEnvironment(map[string]string{"GOPAGER_SECRET_KEY": encodedKey(2)}))
		if err != nil || fromFile.PluginSecrets.Cipher.KeyID() != direct.PluginSecrets.Cipher.KeyID() {
			t.Fatalf("key file did not win: %v", err)
		}
	})
	t.Run("invalid input names only the variable", func(t *testing.T) {
		t.Parallel()
		for name, values := range map[string]map[string]string{
			"GOPAGER_SECRET_KEY":       {"GOPAGER_SECRET_KEY": "synthetic-secret-key"},
			"GOPAGER_SECRET_KEY_FILE":  {"GOPAGER_SECRET_KEY_FILE": badFile},
			"GOPAGER_SECRET_FILE_ROOT": {"GOPAGER_SECRET_FILE_ROOT": "/dev/null"},
		} {
			_, err := parseConfig(args, secretEnvironment(values))
			if err == nil || platform.FailureCode(err) != platform.CodeInvalidInput || !strings.Contains(errors.Unwrap(err).Error(), name) ||
				strings.Contains(err.Error(), "synthetic-secret-key") || strings.Contains(err.Error(), "not-a-key-content") {
				t.Fatalf("%s: unsafe or missing failure: %v", name, err)
			}
		}
		_, err := parseConfig(args, secretEnvironment(map[string]string{"GOPAGER_SECRET_KEY_FILE": filepath.Join(directory, "missing")}))
		if err == nil || strings.Contains(err.Error(), directory) {
			t.Fatalf("missing key file error = %v", err)
		}
	})
	t.Run("file root", func(t *testing.T) {
		t.Parallel()
		config, err := parseConfig(args, secretEnvironment(map[string]string{"GOPAGER_SECRET_FILE_ROOT": directory}))
		if err != nil || config.PluginSecrets.FileRoot != directory {
			t.Fatalf("file root not bound: %+v, %v", config.PluginSecrets, err)
		}
	})
	t.Run("the key never prints", func(t *testing.T) {
		t.Parallel()
		config, err := parseConfig(args, secretEnvironment(map[string]string{"GOPAGER_SECRET_KEY": encodedKey(7)}))
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{fmt.Sprintf("%v", config), fmt.Sprintf("%+v", config), fmt.Sprintf("%#v", config.PluginSecrets)} {
			if strings.Contains(text, encodedKey(7)) || strings.Contains(text, "BwcH") {
				t.Fatalf("key printed: %s", text)
			}
		}
	})
}
