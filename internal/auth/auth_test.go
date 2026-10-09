package auth

import (
	"errors"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{"abc", "Alice_01", "a_very_long_name_of_32_chars_xyz"} {
		if ValidateUsername(ok) != nil {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "ab", "with space", "dash-name", "名字abc", "a_very_long_name_of_33_chars_xyzw"} {
		if ValidateUsername(bad) == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		user, pw string
		want     error
	}{
		{"alice", "Short1!", ErrPasswordLength},
		{"alice", "alllowercaseletters", ErrPasswordClasses},
		{"alice", "lowercase123456", ErrPasswordClasses},
		{"alice", "Alice-Rocks-2026", ErrPasswordName},
		{"bob", "Password123!", ErrPasswordCommon},
		{"bob", "P@ssw0rd2024", ErrPasswordCommon},
		{"bob", "Qwerty12345!", ErrPasswordCommon},
		{"bob", "Admin@123456", ErrPasswordCommon},
		{"bob", "Woaini1314!!", ErrPasswordCommon},
		{"bob", "Abcabcabc123", ErrPasswordCommon},
		{"bob", "1qaz2wsx3edC", ErrPasswordCommon},
		{"bob", "Tr4in-Cactus-Lamp", nil},
		{"bob", "violet Kettle 92", nil},
		{"bob", string(make([]byte, 0)) + "Aa1-" + "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", ErrPasswordTooLong},
	}
	for _, c := range cases {
		if got := ValidatePassword(c.user, c.pw); !errors.Is(got, c.want) {
			t.Errorf("ValidatePassword(%q, %q) = %v, want %v", c.user, c.pw, got, c.want)
		}
	}
}

func TestHashAndTokens(t *testing.T) {
	BcryptCost = 4
	defer func() { BcryptCost = 12 }()
	h, err := HashPassword("Tr4in-Cactus-Lamp")
	if err != nil || !CheckPassword(h, "Tr4in-Cactus-Lamp") || CheckPassword(h, "wrong") {
		t.Fatal("hash round trip failed")
	}
	tok, hash, err := NewToken()
	if err != nil || len(tok) < 40 || HashToken(tok) != hash || len(hash) != 64 {
		t.Fatalf("token %q hash %q err %v", tok, hash, err)
	}
	pw, err := RandomPassword()
	if err != nil || ValidatePassword("someone", pw) != nil {
		t.Fatalf("random password %q rejected: %v", pw, ValidatePassword("someone", pw))
	}
}
