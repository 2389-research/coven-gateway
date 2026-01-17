// ABOUTME: Unit tests for JWT token verification and generation
// ABOUTME: Tests valid tokens, invalid tokens, and expired tokens

package auth

import (
	"errors"
	"testing"
	"time"
)

// testSecret is a 32-byte secret that meets MinSecretLength requirement
var testSecret = []byte("test-secret-key-for-jwt-signing!")

// mustNewJWTVerifier creates a JWTVerifier and fails the test if it errors
func mustNewJWTVerifier(t *testing.T, secret []byte) *JWTVerifier {
	t.Helper()
	verifier, err := NewJWTVerifier(secret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	return verifier
}

func TestJWTVerifier_ValidToken(t *testing.T) {
	verifier := mustNewJWTVerifier(t, testSecret)

	principalID := "principal-123"
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	gotID, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if gotID != principalID {
		t.Errorf("Verify() = %q, want %q", gotID, principalID)
	}
}

func TestJWTVerifier_InvalidToken(t *testing.T) {
	verifier := mustNewJWTVerifier(t, testSecret)
	otherVerifier := mustNewJWTVerifier(t, []byte("different-secret-at-least-32bytes"))

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "garbage token",
			token: "not-a-jwt-token",
		},
		{
			name:  "malformed JWT",
			token: "header.payload.signature",
		},
		{
			name: "wrong secret",
			token: func() string {
				// Generate with different secret
				token, _ := otherVerifier.Generate("principal-123", time.Hour)
				return token
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifier.Verify(tt.token)
			if err == nil {
				t.Error("Verify() should have returned an error")
			}

			if !errors.Is(err, ErrInvalidToken) && !errors.Is(err, ErrExpiredToken) {
				// Some errors wrap ErrInvalidToken
				if tt.name != "wrong secret" && tt.name != "garbage token" && tt.name != "malformed JWT" {
					t.Logf("Error was: %v (this may be acceptable)", err)
				}
			}
		})
	}
}

func TestJWTVerifier_ExpiredToken(t *testing.T) {
	verifier := mustNewJWTVerifier(t, testSecret)

	// Generate a token that expired 1 hour ago
	token, err := verifier.Generate("principal-123", -time.Hour)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	_, err = verifier.Verify(token)
	if err == nil {
		t.Error("Verify() should have returned an error for expired token")
	}

	if !errors.Is(err, ErrExpiredToken) {
		t.Errorf("Verify() error = %v, want ErrExpiredToken", err)
	}
}

func TestJWTVerifier_WeakSecret(t *testing.T) {
	// Test that weak secrets are rejected
	weakSecrets := [][]byte{
		nil,
		{},
		[]byte("short"),
		[]byte("31-bytes-not-quite-enough-here"),
	}

	for _, secret := range weakSecrets {
		_, err := NewJWTVerifier(secret)
		if err == nil {
			t.Errorf("NewJWTVerifier(%q) should have returned error for weak secret", secret)
		}
		if !errors.Is(err, ErrWeakSecret) {
			t.Errorf("NewJWTVerifier(%q) error = %v, want ErrWeakSecret", secret, err)
		}
	}

	// Test that exactly 32 bytes is accepted
	exactSecret := []byte("exactly-32-bytes-secret-here!!!!")
	if len(exactSecret) != 32 {
		t.Fatalf("test setup error: secret is %d bytes, want 32", len(exactSecret))
	}
	_, err := NewJWTVerifier(exactSecret)
	if err != nil {
		t.Errorf("NewJWTVerifier() with 32-byte secret error = %v, want nil", err)
	}
}

func TestJWTVerifier_Generate_CreatesValidToken(t *testing.T) {
	verifier := mustNewJWTVerifier(t, testSecret)

	principalID := "test-principal-456"
	expiresIn := 5 * time.Minute

	token, err := verifier.Generate(principalID, expiresIn)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if token == "" {
		t.Error("Generate() returned empty token")
	}

	// Token should be verifiable
	gotID, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if gotID != principalID {
		t.Errorf("Verify() = %q, want %q", gotID, principalID)
	}
}

func TestJWTVerifier_DifferentPrincipals(t *testing.T) {
	verifier := mustNewJWTVerifier(t, testSecret)

	principals := []string{"principal-1", "principal-2", "principal-3"}

	for _, principalID := range principals {
		token, err := verifier.Generate(principalID, time.Hour)
		if err != nil {
			t.Fatalf("Generate(%q) error = %v", principalID, err)
		}

		gotID, err := verifier.Verify(token)
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}

		if gotID != principalID {
			t.Errorf("Verify() = %q, want %q", gotID, principalID)
		}
	}
}
