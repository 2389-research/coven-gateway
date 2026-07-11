// ABOUTME: Tests for webauthn store helpers: storeWebAuthnCredential, lookupCredentialUser, finalizeWebAuthnLogin.
// ABOUTME: Exercises the round-trip between webauthn credentials and the SQLite store.

package webadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// =============================================================================
// storeWebAuthnCredential
// =============================================================================

func TestStoreWebAuthnCredential_RoundTrip(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	// Create a user that the credential will be associated with
	user := createAdminUserWithPassword(t, a, "wa-user", "hunter2hunter2")

	cred := &webauthn.Credential{
		ID:              []byte("credential-id-for-test"),
		PublicKey:       []byte("mock-public-key-data"),
		AttestationType: "none",
		Authenticator: webauthn.Authenticator{
			SignCount: 3,
		},
	}

	credID, err := a.storeWebAuthnCredential(ctx, user.ID, cred)
	if err != nil {
		t.Fatalf("storeWebAuthnCredential: %v", err)
	}
	if credID == "" {
		t.Error("expected non-empty credential ID")
	}

	// Verify it can be retrieved via the credential ID bytes
	stored, err := a.store.GetWebAuthnCredentialByCredentialID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("GetWebAuthnCredentialByCredentialID: %v", err)
	}
	if stored.UserID != user.ID {
		t.Errorf("UserID = %q, want %q", stored.UserID, user.ID)
	}
	if stored.SignCount != 3 {
		t.Errorf("SignCount = %d, want 3", stored.SignCount)
	}
}

func TestStoreWebAuthnCredential_WithTransports(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	user := createAdminUserWithPassword(t, a, "wa-transport-user", "hunter2hunter2")

	cred := &webauthn.Credential{
		ID:        []byte("cred-with-transports"),
		PublicKey: []byte("pk-data"),
		Transport: []protocol.AuthenticatorTransport{"usb", "nfc"},
	}

	_, err := a.storeWebAuthnCredential(ctx, user.ID, cred)
	if err != nil {
		t.Fatalf("storeWebAuthnCredential with transports: %v", err)
	}
}

// =============================================================================
// lookupCredentialUser
// =============================================================================

func TestLookupCredentialUser_Found(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	user := createAdminUserWithPassword(t, a, "lookup-user", "hunter2hunter2")
	credBytes := []byte("lookup-cred-id")

	cred := &webauthn.Credential{
		ID:        credBytes,
		PublicKey: []byte("some-public-key"),
	}

	if _, err := a.storeWebAuthnCredential(ctx, user.ID, cred); err != nil {
		t.Fatalf("storeWebAuthnCredential: %v", err)
	}

	storedCred, foundUser, err := a.lookupCredentialUser(ctx, credBytes)
	if err != nil {
		t.Fatalf("lookupCredentialUser: %v", err)
	}
	if storedCred == nil {
		t.Fatal("expected non-nil stored credential")
	}
	if foundUser.ID != user.ID {
		t.Errorf("user.ID = %q, want %q", foundUser.ID, user.ID)
	}
}

func TestLookupCredentialUser_NotFound_ReturnsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	_, _, err := a.lookupCredentialUser(ctx, []byte("nonexistent-credential-id"))
	if err == nil {
		t.Error("expected error for nonexistent credential")
	}
}

// =============================================================================
// finalizeWebAuthnLogin
// =============================================================================

func TestFinalizeWebAuthnLogin_CreatesSession(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	user := createAdminUserWithPassword(t, a, "finalize-user", "hunter2hunter2")

	// Store a credential so we can update its sign count
	credBytes := []byte("finalize-cred-id")
	cred := &webauthn.Credential{
		ID:        credBytes,
		PublicKey: []byte("pk"),
	}
	storedCredID, err := a.storeWebAuthnCredential(ctx, user.ID, cred)
	if err != nil {
		t.Fatalf("storeWebAuthnCredential: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/passkey/login/finish", nil)

	err = a.finalizeWebAuthnLogin(rec, req, storedCredID, 10, user.ID)
	if err != nil {
		t.Fatalf("finalizeWebAuthnLogin: %v", err)
	}

	// Verify session cookie was set (createSession sets a session cookie)
	cookies := rec.Result().Cookies()
	hasCookie := false
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			hasCookie = true
			break
		}
	}
	if !hasCookie {
		t.Errorf("expected %q cookie to be set after finalizeWebAuthnLogin; got %d cookies", SessionCookieName, len(cookies))
	}
}
