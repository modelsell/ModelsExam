package secretbox

import (
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	box, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("sk-live-secret", "cred-1|7")
	if err != nil || !Sealed(sealed) || strings.Contains(sealed, "sk-live-secret") {
		t.Fatalf("seal: %q %v", sealed, err)
	}
	again, _ := box.Seal("sk-live-secret", "cred-1|7")
	if again == sealed {
		t.Fatal("each seal needs its own nonce")
	}
	if plain, err := box.Open(sealed, "cred-1|7"); err != nil || plain != "sk-live-secret" {
		t.Fatalf("open: %q %v", plain, err)
	}
	if _, err := box.Open(sealed, "cred-2|7"); err == nil {
		t.Fatal("a ciphertext moved to another row must not decrypt")
	}
	other, _ := NewKey()
	otherBox, _ := New(other)
	if _, err := otherBox.Open(sealed, "cred-1|7"); err == nil {
		t.Fatal("another master key must not decrypt")
	}
	if _, err := box.Open("sk-plaintext", "cred-1|7"); err == nil {
		t.Fatal("plaintext is not a sealed value")
	}
}

func TestBadKeys(t *testing.T) {
	for _, k := range []string{"", "short", "c2hvcnQ=", strings.Repeat("A", 60)} {
		if _, err := New(k); err == nil {
			t.Errorf("key %q accepted", k)
		}
	}
}
