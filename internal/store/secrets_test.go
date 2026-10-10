package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"model-check/internal/secretbox"
)

func rawSecret(t *testing.T, s *Store, id string) string {
	t.Helper()
	var c Credential
	if err := s.db.Select("secret").Where("id = ?", id).First(&c).Error; err != nil {
		t.Fatal(err)
	}
	return c.Secret
}

func TestSavedKeysAreEncryptedAtRest(t *testing.T) {
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "enc")
	c := newCredential(t, s, u.ID, time.Now().Add(time.Hour))
	stored := rawSecret(t, s, c.ID)
	if strings.Contains(stored, "sk-secret-value-123") || !secretbox.Sealed(stored) {
		t.Fatalf("secret column holds plaintext: %q", stored)
	}
	if got, err := s.CredentialSecret(ctx, u.ID, c.ID); err != nil || got != "sk-secret-value-123" {
		t.Fatalf("runner read: %q %v", got, err)
	}
	// A ciphertext copied into another row does not decrypt.
	other := newCredential(t, s, u.ID, time.Now().Add(time.Hour))
	if err := s.db.Model(&Credential{}).Where("id = ?", other.ID).Update("secret", stored).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CredentialSecret(ctx, u.ID, other.ID); err == nil {
		t.Fatal("swapped ciphertext decrypted")
	}
}

func TestPlaintextRowsAreMigrated(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	u := newUser(t, s, "legacy")
	// Without a master key nothing can be saved or read.
	if err := s.CreateCredential(ctx, &Credential{ID: uuid.NewString(), UserID: u.ID, Secret: "x"}); !errors.Is(err, ErrNoSecretKey) {
		t.Fatalf("save without key: %v", err)
	}
	// A row written before encryption existed.
	id := uuid.NewString()
	if err := s.db.Create(&Credential{ID: id, UserID: u.ID, Provider: "claude", Secret: "sk-old-plaintext-1", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CredentialSecret(ctx, u.ID, id); !errors.Is(err, ErrNoSecretKey) {
		t.Fatalf("read without key: %v", err)
	}
	box := testBox(t)
	if n, err := s.UseSecretBox(ctx, box); err != nil || n != 1 {
		t.Fatalf("migration: %d %v", n, err)
	}
	if stored := rawSecret(t, s, id); strings.Contains(stored, "sk-old-plaintext-1") || !secretbox.Sealed(stored) {
		t.Fatalf("not migrated: %q", stored)
	}
	if n, err := s.UseSecretBox(ctx, box); err != nil || n != 0 {
		t.Fatalf("second migration must change nothing: %d %v", n, err)
	}
	// Starting with a different key must fail instead of mixing keys.
	if _, err := s.UseSecretBox(ctx, testBox(t)); err == nil {
		t.Fatal("wrong master key accepted")
	}
	if _, err := s.UseSecretBox(ctx, box); err != nil {
		t.Fatal(err)
	}
	// Rotation re-encrypts every row under the new key.
	next := testBox(t)
	if n, err := s.RotateSecretKey(ctx, next); err != nil || n != 1 {
		t.Fatalf("rotate: %d %v", n, err)
	}
	if got, err := s.CredentialSecret(ctx, u.ID, id); err != nil || got != "sk-old-plaintext-1" {
		t.Fatalf("after rotation: %q %v", got, err)
	}
	if _, err := box.Open(rawSecret(t, s, id), secretAAD(id, u.ID)); err == nil {
		t.Fatal("old key still opens the rotated value")
	}
}
