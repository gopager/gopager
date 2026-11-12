package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// maxSecretKeyFileBytes bounds the master key file; a valid key is 44 characters of Base64.
const maxSecretKeyFileBytes = 1024

// pluginSecretSettings are the operator's Secret inputs. The cipher redacts itself in every textual
// form, so the settings are safe in logs and %v output.
type pluginSecretSettings struct {
	// Cipher seals and opens stored Secrets; nil when no master key is configured.
	Cipher *processing.SecretCipher
	// FileRoot is the directory file: references are relative to; empty rejects file references.
	FileRoot string
}

// parsePluginSecrets reads the master key (GOPAGER_SECRET_KEY_FILE wins over GOPAGER_SECRET_KEY)
// and the Secret file root. Errors name only the variable, never its content.
func parsePluginSecrets(getenv func(string) string) (pluginSecretSettings, error) {
	var settings pluginSecretSettings
	encoded, variable := getenv("GOPAGER_SECRET_KEY"), "GOPAGER_SECRET_KEY"
	configured := encoded != ""
	if path := getenv("GOPAGER_SECRET_KEY_FILE"); path != "" {
		content, err := readSecretKeyFile(path)
		if err != nil {
			return settings, invalidSecretSetting("GOPAGER_SECRET_KEY_FILE")
		}
		// A named key file is a deliberate setting: an empty one must fail startup instead of
		// silently running without a key.
		encoded, variable, configured = content, "GOPAGER_SECRET_KEY_FILE", true
	}
	if configured {
		key, err := processing.ParseSecretKey(encoded)
		if err != nil {
			return settings, invalidSecretSetting(variable)
		}
		defer clear(key)
		if settings.Cipher, err = processing.NewSecretCipher(key); err != nil {
			return settings, invalidSecretSetting(variable)
		}
	}
	if root := getenv("GOPAGER_SECRET_FILE_ROOT"); root != "" {
		if strings.TrimSpace(root) != root {
			return settings, invalidSecretSetting("GOPAGER_SECRET_FILE_ROOT")
		}
		opened, err := os.OpenRoot(root)
		if err != nil {
			return settings, invalidSecretSetting("GOPAGER_SECRET_FILE_ROOT")
		}
		if err := opened.Close(); err != nil {
			return settings, invalidSecretSetting("GOPAGER_SECRET_FILE_ROOT")
		}
		settings.FileRoot = root
	}
	return settings, nil
}

// invalidSecretSetting logs which variable is invalid, never its content, and returns the failure.
func invalidSecretSetting(variable string) error {
	slog.Error("invalid plugin Secret setting", "variable", variable)
	return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid "+variable))
}

// readSecretKeyFile returns the bounded content of the key file.
func readSecretKeyFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxSecretKeyFileBytes+1))
	if err != nil || len(content) > maxSecretKeyFileBytes {
		return "", errors.New("unreadable key file")
	}
	return string(content), nil
}
