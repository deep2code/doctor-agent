package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/doctor-agent/internal/database"
)

// legacyHash reproduces the pre-PBKDF2 verifier format ("16 hex salt chars +
// 64 hex sha256(salt||password) chars") so migration can be tested without a
// database row.
func legacyHash(salt, password string) string {
	sum := sha256.Sum256([]byte(salt + password))
	return salt + hex.EncodeToString(sum[:])
}

func TestHashPasswordRoundTrip(t *testing.T) {
	h, err := hashPassword("hunter2!!")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if !strings.HasPrefix(h, pbkdf2Prefix) {
		t.Fatalf("hash not in pbkdf2 format: %q", h)
	}
	if h2, err := hashPassword("hunter2!!"); err != nil {
		t.Fatalf("hashPassword second call: %v", err)
	} else if h == h2 {
		t.Error("two hashes of the same password are identical (salt is not random)")
	}

	matched, legacy := verifyPassword("hunter2!!", h)
	if !matched || legacy {
		t.Fatalf("verifyPassword = (%v, %v), want (true, false)", matched, legacy)
	}
	if matched, _ := verifyPassword("hunter2!", h); matched {
		t.Error("wrong password accepted")
	}
}

func TestVerifyPasswordAcceptsLegacyAndFlagsUpgrade(t *testing.T) {
	const salt = "0123456789abcdef"
	stored := legacyHash(salt, "guessme")

	matched, legacy := verifyPassword("guessme", stored)
	if !matched || !legacy {
		t.Fatalf("verifyPassword(legacy) = (%v, %v), want (true, true)", matched, legacy)
	}
	if matched, legacy := verifyPassword("guessme1", stored); matched || legacy {
		t.Fatalf("verifyPassword(wrong, legacy) = (%v, %v), want (false, false)", matched, legacy)
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"short",
		"pbkdf2$sha256$notanumber$00$00",
		"pbkdf2$sha256$210000$nothex$00",
		strings.Repeat("0", legacySaltChars+64), // legacy shape, valid hex, wrong length checks
		pbkdf2Prefix + "210000$00",              // truncated field list
	}
	for _, c := range cases {
		if matched, legacy := verifyPassword("anything", c); matched || legacy {
			t.Errorf("verifyPassword(%q) = (%v, %v), want (false, false)", c, matched, legacy)
		}
	}
}

// A verifier carrying an iteration count of 0 must not be treated as a
// zero-cost match — pbkdf2 with iter<=0 is invalid, not free.
func TestVerifyPasswordRejectsZeroIterations(t *testing.T) {
	stored := pbkdf2Prefix + "0$" + strings.Repeat("00", 16) + "$" + strings.Repeat("00", 32)
	if matched, _ := verifyPassword("anything", stored); matched {
		t.Error("iterations=0 accepted")
	}
}

func TestAdminCreateUserInputValidate(t *testing.T) {
	base := func() *AdminCreateUserInput {
		return &AdminCreateUserInput{Username: "nurse01", Password: "abcdef", Nickname: "护士"}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	// Nickname falls back to the username.
	in := base()
	in.Nickname = ""
	if err := in.Validate(); err != nil || in.Nickname != "nurse01" {
		t.Fatalf("nickname fallback = (%q, %v)", in.Nickname, err)
	}

	for name, mutate := range map[string]func(*AdminCreateUserInput){
		"empty username": func(i *AdminCreateUserInput) { i.Username = " " },
		"short username": func(i *AdminCreateUserInput) { i.Username = "ab" },
		"long username":  func(i *AdminCreateUserInput) { i.Username = strings.Repeat("a", 33) },
		"empty password": func(i *AdminCreateUserInput) { i.Password = "" },
		"short password": func(i *AdminCreateUserInput) { i.Password = "12345" },
	} {
		in := base()
		mutate(in)
		if err := in.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestAdminCreateUserRequiresAdmin(t *testing.T) {
	svc := &Service{}
	// A non-admin (or absent) creator must be rejected before any DB access.
	if _, err := svc.AdminCreateUser(&AdminCreateUserInput{}, nil); err == nil {
		t.Error("nil creator accepted")
	}
	if _, err := svc.AdminCreateUser(&AdminCreateUserInput{}, &database.User{}); err == nil {
		t.Error("non-admin creator accepted")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	svc := NewService(nil, "unit-test-secret")
	tok := svc.IssueToken("user-42")

	got, err := svc.VerifyToken(tok)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if got != "user-42" {
		t.Errorf("VerifyToken userID = %q, want %q", got, "user-42")
	}
	// A second token for the same user differs (expiry timestamp), and both
	// verify — issuing must not invalidate what was already handed out.
	if _, err := svc.VerifyToken(svc.IssueToken("user-42")); err != nil {
		t.Errorf("second token rejected: %v", err)
	}
}

// A token must never cross secrets: an attacker who can read one deployment's
// AUTH_SECRET (or a test fixture that leaked) cannot use it where another is
// configured, and a restarted process with a new random key trusts nothing.
func TestTokenRejectedByAnotherSecret(t *testing.T) {
	tok := NewService(nil, "secret-a").IssueToken("user-1")
	if _, err := NewService(nil, "secret-b").VerifyToken(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("VerifyToken with the wrong secret = %v, want ErrInvalidToken", err)
	}
}

// The only thing separating two accounts is the user id inside the payload, so
// editing it (or the signature) must break verification — otherwise a low
// privilege token becomes an admin one.
func TestTokenRejectsTampering(t *testing.T) {
	svc := NewService(nil, "secret-a")
	token := svc.IssueToken("user-1")
	encoded, sig, found := strings.Cut(token, ".")
	if !found {
		t.Fatalf("token has no separator: %q", token)
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	t.Run("payload rewritten to another account", func(t *testing.T) {
		payload := strings.Replace(string(raw), "user-1", "admin!", 1)
		bad := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
		if _, err := svc.VerifyToken(bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("VerifyToken = %v, want ErrInvalidToken", err)
		}
	})
	t.Run("signature flipped", func(t *testing.T) {
		flips := []byte(sig)
		if flips[0] == '0' {
			flips[0] = '1'
		} else {
			flips[0] = '0'
		}
		bad := encoded + "." + string(flips)
		if _, err := svc.VerifyToken(bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("VerifyToken = %v, want ErrInvalidToken", err)
		}
	})
}

func TestTokenExpiry(t *testing.T) {
	svc := NewService(nil, "secret-a")
	mk := func(expiry int64) string {
		payload := "user-1|" + strconv.FormatInt(expiry, 10)
		return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
			hex.EncodeToString(svc.mac(payload))
	}
	if _, err := svc.VerifyToken(mk(time.Now().Add(-time.Second).Unix())); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expired token = %v, want ErrInvalidToken", err)
	}
	if uid, err := svc.VerifyToken(mk(time.Now().Add(time.Hour).Unix())); err != nil || uid != "user-1" {
		t.Errorf("future token = (%q, %v), want (\"user-1\", nil)", uid, err)
	}
}

func TestVerifyTokenRejectsMalformed(t *testing.T) {
	svc := NewService(nil, "secret-a")
	cases := []string{
		"", "   ", "no-separator", ".", "a.", ".b",
		"not-base64!!!" + "." + strings.Repeat("0", 64),
		base64.RawURLEncoding.EncodeToString([]byte("user-1")) + "." + strings.Repeat("0", 64), // no expiry field
	}
	for _, c := range cases {
		if _, err := svc.VerifyToken(c); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("VerifyToken(%q) = %v, want ErrInvalidToken", c, err)
		}
	}
}

// With AUTH_SECRET unset the service still works locally, but the key is random
// per process: tokens issued before a restart must stop verifying.
func TestEphemeralSecretDoesNotCrossServices(t *testing.T) {
	first := NewService(nil, "")
	second := NewService(nil, "")
	tok := first.IssueToken("user-1")
	if _, err := first.VerifyToken(tok); err != nil {
		t.Fatalf("same-process token rejected: %v", err)
	}
	if _, err := second.VerifyToken(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token survived a key change: %v", err)
	}
}
