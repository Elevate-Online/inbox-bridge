// Package desktopconfig registers google-multi-auth as an MCP server inside
// the Claude Desktop app's own config file, preserving anything already
// there.
package desktopconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ConfigPath returns the platform-specific location of Claude Desktop's
// config file.
func ConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json"), nil
	default:
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
	}
}

// Register adds (or updates) the "google-multi-auth" entry under
// mcpServers in Claude Desktop's config file, pointing at binaryPath with no
// arguments (the binary defaults to serving MCP over stdio). Any other keys
// or servers already present in the file are preserved. If the file exists
// it is backed up first; if it exists but contains invalid JSON, a warning
// is printed and registration proceeds from an empty config (after still
// backing up the unreadable file).
func Register(binaryPath string) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrInit(path)
	if err != nil {
		return err
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		if _, present := cfg["mcpServers"]; present {
			fmt.Fprintf(os.Stderr, "warning: %s has an mcpServers key that is not an object; replacing it\n", path)
		}
		servers = map[string]any{}
	}

	servers["google-multi-auth"] = map[string]any{
		"command": binaryPath,
	}
	cfg["mcpServers"] = servers

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding Claude Desktop config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating Claude Desktop config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing Claude Desktop config %s: %w", path, err)
	}

	return nil
}

// loadOrInit reads and parses the config file at path, returning an empty
// config if it does not exist yet. If the file exists but cannot be parsed
// as JSON, it is backed up, a warning is printed, and an empty config is
// returned so registration can still proceed.
func loadOrInit(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading Claude Desktop config %s: %w", path, err)
	}

	if err := backup(path, data); err != nil {
		return nil, err
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s is not valid JSON, starting from an empty config (original backed up): %v\n", path, err)
		return map[string]any{}, nil
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// backup copies the existing config file's contents to a timestamped
// sibling path before it gets overwritten.
func backup(path string, data []byte) error {
	backupPath := fmt.Sprintf("%s.bak-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("backing up Claude Desktop config to %s: %w", backupPath, err)
	}
	return nil
}
