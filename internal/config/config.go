// Package config holds shared configuration paths and constants for
// inbox-bridge: where on disk the OAuth client credentials and
// per-account tokens live, and which Gmail OAuth scopes are requested.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Scopes lists the OAuth scopes requested when connecting a Gmail account.
var Scopes = []string{"https://www.googleapis.com/auth/gmail.readonly"}

// ConfigDir returns ~/.config/inbox-bridge, creating no directories
// itself -- callers create it (and tokens/) with the right permissions when
// they actually need to write.
//
// The tool was called google-multi-auth until v0.3.0. If only the old
// ~/.config/google-multi-auth exists, it is moved to the new path the first
// time this runs, so accounts connected under the old name keep working.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "inbox-bridge")
	migrateLegacyDir(filepath.Join(home, ".config", "google-multi-auth"), dir)
	return dir, nil
}

// migrateLegacyDir moves legacy to dir when dir does not exist yet and
// legacy does. Failures are ignored: the caller then sees an empty new
// directory, the same as a fresh install, and can reconnect.
func migrateLegacyDir(legacy, dir string) {
	if _, err := os.Stat(dir); err == nil || !os.IsNotExist(err) {
		return
	}
	if info, err := os.Stat(legacy); err != nil || !info.IsDir() {
		return
	}
	_ = os.Rename(legacy, dir)
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
