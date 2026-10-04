// Package tokenstore persists and retrieves per-account OAuth tokens under
// the tokens directory managed by internal/config.
package tokenstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/oauth2"

	"inbox-bridge/internal/config"
)

// ListAccounts returns the emails of all connected accounts, sorted, based
// on the *.json files present in the tokens directory. A missing tokens
// directory is treated as "no accounts yet", not an error.
func ListAccounts() ([]string, error) {
	dir, err := config.TokensDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading tokens directory %s: %w", dir, err)
	}

	accounts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		accounts = append(accounts, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(accounts)
	return accounts, nil
}

// validAccountID reports whether id is safe to use as a token file's base
// name -- it must not be empty, ".", "..", or contain a path separator,
// which would otherwise let a caller-supplied account escape the tokens
// directory via filepath.Join.
func validAccountID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`)
}

// SaveToken writes tok as JSON to <TokensDir>/<email>.json, creating the
// tokens directory if necessary.
func SaveToken(email string, tok *oauth2.Token) error {
	if !validAccountID(email) {
		return fmt.Errorf("invalid account identifier %q", email)
	}

	dir, err := config.TokensDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating tokens directory %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("setting permissions on tokens directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding token for %s: %w", email, err)
	}

	path := filepath.Join(dir, email+".json")
	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("setting permissions on token file %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing token file %s: %w", path, err)
	}
	return nil
}

// LoadToken reads the stored token for email. If no token file exists it
// returns (nil, nil) rather than an error, so callers can distinguish "not
// connected" from a real I/O failure.
func LoadToken(email string) (*oauth2.Token, error) {
	if !validAccountID(email) {
		return nil, fmt.Errorf("invalid account identifier %q", email)
	}

	dir, err := config.TokensDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, email+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading token file %s: %w", path, err)
	}

	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parsing token file %s: %w", path, err)
	}
	return &tok, nil
}

// RemoveAccount deletes the stored token for email. It returns false (not
// an error) if there was no token file for that account.
func RemoveAccount(email string) (bool, error) {
	if !validAccountID(email) {
		return false, fmt.Errorf("invalid account identifier %q", email)
	}

	dir, err := config.TokensDir()
	if err != nil {
		return false, err
	}

	path := filepath.Join(dir, email+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("removing token file %s: %w", path, err)
	}
	return true, nil
}
