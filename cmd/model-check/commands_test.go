package main

import (
	"os"
	"path/filepath"
	"testing"

	"model-check/internal/secretbox"
)

func TestServerNeedsAMasterKey(t *testing.T) {
	t.Setenv("MODEL_CHECK_SECRET_KEY_FILE", "")
	t.Setenv("MODEL_CHECK_SECRET_KEY", "")
	if _, err := loadSecretBox(); err == nil {
		t.Fatal("started without a master key")
	}
	t.Setenv("MODEL_CHECK_SECRET_KEY", "not-a-key")
	if _, err := loadSecretBox(); err == nil {
		t.Fatal("malformed key accepted")
	}
	key, _ := secretbox.NewKey()
	file := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(file, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_CHECK_SECRET_KEY_FILE", file)
	if _, err := loadSecretBox(); err != nil {
		t.Fatalf("key file: %v", err)
	}
	t.Setenv("MODEL_CHECK_SECRET_KEY_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := loadSecretBox(); err == nil {
		t.Fatal("missing key file accepted")
	}
}
