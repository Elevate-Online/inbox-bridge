package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyDirMovesOldConfig(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "google-multi-auth")
	dir := filepath.Join(root, "inbox-bridge")
	if err := os.MkdirAll(filepath.Join(legacy, "tokens"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "tokens", "a@gmail.com.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacyDir(legacy, dir)

	if _, err := os.Stat(filepath.Join(dir, "tokens", "a@gmail.com.json")); err != nil {
		t.Fatalf("token not found under the new directory: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists (err=%v)", err)
	}
}

func TestMigrateLegacyDirLeavesExistingNewConfig(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "google-multi-auth")
	dir := filepath.Join(root, "inbox-bridge")
	for _, d := range []string{legacy, dir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	migrateLegacyDir(legacy, dir)

	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy directory should be untouched when the new one exists: %v", err)
	}
}
