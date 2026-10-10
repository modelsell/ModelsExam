package auth

import (
	"errors"
	"strings"
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
		pw   string
		want error
	}{
		{"abc1234", ErrPasswordLength},
		{"12345678901", ErrPasswordClasses},  // digits only
		{"onlyletters", ErrPasswordClasses},  // letters only
		{"!!!!@@@@####", ErrPasswordClasses}, // symbols only
		{"password1", ErrPasswordCommon},
		{"abc12345", ErrPasswordCommon},
		{"Password123!", ErrPasswordCommon},
		{"P@ssw0rd2024", ErrPasswordCommon},
		{"Qwerty12345", ErrPasswordCommon},
		{"woaini1314", ErrPasswordCommon},
		{"abcabcabc123", ErrPasswordCommon},
		{"1qaz2wsx3edc", ErrPasswordCommon},
		{"kettle92violet", nil}, // lowercase letters and digits are enough
		{"bob7kettle", nil},
		{"Tr4in-Cactus-Lamp", nil},
		{"Aa1-" + strings.Repeat("x", 70), ErrPasswordTooLong},
	}
	for _, c := range cases {
		if got := ValidatePassword(c.pw); !errors.Is(got, c.want) {
			t.Errorf("ValidatePassword(%q) = %v, want %v", c.pw, got, c.want)
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
	if err != nil || ValidatePassword(pw) != nil {
		t.Fatalf("random password %q rejected: %v", pw, ValidatePassword(pw))
	}
}
