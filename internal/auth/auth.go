// Package auth holds the account rules that need no HTTP or database:
// username and password policy, password hashing and session tokens.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the work factor for new hashes. Tests may lower it.
var BcryptCost = 12

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

// Error messages are English sentences; the web app uses them as i18n keys.
var (
	ErrUsername        = errors.New("Username must be 3 to 32 letters, digits or underscores")
	ErrPasswordLength  = errors.New("Password must be at least 10 characters")
	ErrPasswordTooLong = errors.New("Password must be at most 72 bytes")
	ErrPasswordClasses = errors.New("Password must use at least 3 of: uppercase letters, lowercase letters, digits, symbols")
	ErrPasswordName    = errors.New("Password must not contain the username")
	ErrPasswordCommon  = errors.New("This password is too common. Choose another one")
)

func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return ErrUsername
	}
	return nil
}

//go:embed weak-passwords.txt
var weakList string

var weak = func() map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(weakList, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			m[line] = true
		}
	}
	return m
}()

var leet = strings.NewReplacer("@", "a", "4", "a", "0", "o", "1", "i", "!", "i", "3", "e", "$", "s", "5", "s", "7", "t", "+", "t", "8", "b", "9", "g")

// ValidatePassword applies the account password policy.
func ValidatePassword(username, password string) error {
	if utf8.RuneCountInString(password) < 10 {
		return ErrPasswordLength
	}
	if len(password) > 72 { // bcrypt reads at most 72 bytes
		return ErrPasswordTooLong
	}
	var upper, lower, digit, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	classes := 0
	for _, ok := range []bool{upper, lower, digit, symbol} {
		if ok {
			classes++
		}
	}
	if classes < 3 {
		return ErrPasswordClasses
	}
	lowerPw := strings.ToLower(password)
	if username != "" && strings.Contains(lowerPw, strings.ToLower(username)) {
		return ErrPasswordName
	}
	if isCommon(lowerPw) {
		return ErrPasswordCommon
	}
	return nil
}

// isCommon matches the list directly, after undoing leetspeak, and after
// stripping the digits and symbols people append to a common word.
func isCommon(pw string) bool {
	if weak[pw] {
		return true
	}
	letters := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) {
				return r
			}
			return -1
		}, s)
	}
	notLetter := func(r rune) bool { return !unicode.IsLetter(r) }
	core := strings.TrimFunc(pw, notLetter)
	for _, candidate := range []string{core, leet.Replace(core), letters(core), letters(leet.Replace(core)), letters(pw)} {
		if candidate == "" || weak[candidate] || repeated(candidate) {
			return true
		}
	}
	return false
}

// repeated is true for a word made of one short unit (aaaa, abab, abcabc).
func repeated(s string) bool {
	for n := 1; n <= len(s)/2; n++ {
		if len(s)%n == 0 && strings.Repeat(s[:n], len(s)/n) == s {
			return true
		}
	}
	return false
}

func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	return string(h), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy password for unknown users"), 12)

// DummyCheck spends the same time as a real comparison, so a missing user
// cannot be told apart by timing.
func DummyCheck(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// NewToken returns a random session token and the hash stored for it.
func NewToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RandomPassword is used by the reset-password command.
func RandomPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Base64 of 12 bytes is 16 characters; the fixed suffix guarantees all classes.
	return base64.RawURLEncoding.EncodeToString(b) + "-Aa9", nil
}
