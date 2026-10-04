// Package config holds shared configuration paths and constants for
// google-multi-auth: where on disk the OAuth client credentials and
// per-account tokens live, and which Gmail OAuth scopes are requested.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Scopes lists the OAuth scopes requested when connecting a Gmail account.
var Scopes = []string{"https://www.googleapis.com/auth/gmail.readonly"}

// ConfigDir returns ~/.config/google-multi-auth, creating no directories
// itself -- callers create it (and tokens/) with the right permissions when
// they actually need to write.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "google-multi-auth"), nil
}

// ClientFile returns the path to the stored OAuth client credentials.
func ClientFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "client.json"), nil
}

// TokensDir returns the path to the directory holding one JSON token file
// per connected account.
func TokensDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tokens"), nil
}
