// ABOUTME: Tests for SSH public key authentication
// ABOUTME: Covers signature verification, fingerprint computation, and metadata extraction

package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// generateTestKeyPair creates a new ed25519 key pair for testing
func generateTestKeyPair(t *testing.T) (ssh.Signer, ssh.PublicKey, string) {
	t.Helper()

	// Generate ed25519 key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Convert to SSH signer
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	// Get public key in authorized_keys format
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to create SSH public key: %v", err)
	}

	pubkeyStr := string(ssh.MarshalAuthorizedKey(sshPub))

	return signer, sshPub, pubkeyStr
}

// signMessage creates an SSH signature over a message
func signMessage(t *testing.T, signer ssh.Signer, message string) string {
	t.Helper()

	sig, err := signer.Sign(rand.Reader, []byte(message))
	if err != nil {
		t.Fatalf("failed to sign message: %v", err)
	}

	return base64.StdEncoding.EncodeToString(ssh.Marshal(sig))
}

func TestSSHVerifier_ValidSignature(t *testing.T) {
	signer, _, pubkeyStr := generateTestKeyPair(t)
	verifier := NewSSHVerifier()

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessage(t, signer, message)

	req := &SSHAuthRequest{
		Pubkey:    pubkeyStr,
		Signature: signature,
		Timestamp: timestamp,
		Nonce:     nonce,
	}

	fingerprint, err := verifier.Verify(req)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if fingerprint == "" {
		t.Error("Verify() returned empty fingerprint")
	}

	// Fingerprint should be 64 hex chars (SHA256)
	if len(fingerprint) != 64 {
		t.Errorf("fingerprint length = %d, want 64", len(fingerprint))
	}
}

func TestSSHVerifier_ExpiredSignature(t *testing.T) {
	signer, _, pubkeyStr := generateTestKeyPair(t)
	verifier := NewSSHVerifier()

	// Use a timestamp from 10 minutes ago (beyond the 5 minute limit)
	timestamp := time.Now().Add(-10 * time.Minute).Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessage(t, signer, message)

	req := &SSHAuthRequest{
		Pubkey:    pubkeyStr,
		Signature: signature,
		Timestamp: timestamp,
		Nonce:     nonce,
	}

	_, err := verifier.Verify(req)
	if err == nil {
		t.Error("Verify() should reject expired signature")
	}
}

func TestSSHVerifier_FutureTimestamp(t *testing.T) {
	signer, _, pubkeyStr := generateTestKeyPair(t)
	verifier := NewSSHVerifier()

	// Use a timestamp 2 minutes in the future (beyond the 1 minute tolerance)
	timestamp := time.Now().Add(2 * time.Minute).Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessage(t, signer, message)

	req := &SSHAuthRequest{
		Pubkey:    pubkeyStr,
		Signature: signature,
		Timestamp: timestamp,
		Nonce:     nonce,
	}

	_, err := verifier.Verify(req)
	if err == nil {
		t.Error("Verify() should reject future timestamp")
	}
}

func TestSSHVerifier_InvalidPublicKey(t *testing.T) {
	verifier := NewSSHVerifier()

	req := &SSHAuthRequest{
		Pubkey:    "not-a-valid-public-key",
		Signature: "dGVzdA==",
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce",
	}

	_, err := verifier.Verify(req)
	if err == nil {
		t.Error("Verify() should reject invalid public key")
	}
}

func TestSSHVerifier_InvalidSignature(t *testing.T) {
	_, _, pubkeyStr := generateTestKeyPair(t)
	verifier := NewSSHVerifier()

	req := &SSHAuthRequest{
		Pubkey:    pubkeyStr,
		Signature: "not-valid-base64!!!",
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce",
	}

	_, err := verifier.Verify(req)
	if err == nil {
		t.Error("Verify() should reject invalid signature encoding")
	}
}

func TestSSHVerifier_WrongKey(t *testing.T) {
	// Sign with one key, but send a different public key
	signer1, _, _ := generateTestKeyPair(t)
	_, _, pubkeyStr2 := generateTestKeyPair(t)

	verifier := NewSSHVerifier()

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessage(t, signer1, message)

	req := &SSHAuthRequest{
		Pubkey:    pubkeyStr2, // Different key than what signed
		Signature: signature,
		Timestamp: timestamp,
		Nonce:     nonce,
	}

	_, err := verifier.Verify(req)
	if err == nil {
		t.Error("Verify() should reject signature from wrong key")
	}
}

func TestSSHVerifier_TamperedMessage(t *testing.T) {
	signer, _, pubkeyStr := generateTestKeyPair(t)
	verifier := NewSSHVerifier()

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessage(t, signer, message)

	// Use a different nonce than what was signed
	req := &SSHAuthRequest{
		Pubkey:    pubkeyStr,
		Signature: signature,
		Timestamp: timestamp,
		Nonce:     "different-nonce", // Tampered
	}

	_, err := verifier.Verify(req)
	if err == nil {
		t.Error("Verify() should reject tampered message")
	}
}

func TestComputeFingerprint_Consistent(t *testing.T) {
	_, pubkey, _ := generateTestKeyPair(t)

	fp1 := ComputeFingerprint(pubkey)
	fp2 := ComputeFingerprint(pubkey)

	if fp1 != fp2 {
		t.Errorf("ComputeFingerprint() not consistent: %s != %s", fp1, fp2)
	}
}

func TestComputeFingerprint_Unique(t *testing.T) {
	_, pubkey1, _ := generateTestKeyPair(t)
	_, pubkey2, _ := generateTestKeyPair(t)

	fp1 := ComputeFingerprint(pubkey1)
	fp2 := ComputeFingerprint(pubkey2)

	if fp1 == fp2 {
		t.Error("ComputeFingerprint() should produce unique fingerprints for different keys")
	}
}

func TestParseFingerprintFromKey(t *testing.T) {
	_, pubkey, pubkeyStr := generateTestKeyPair(t)

	expected := ComputeFingerprint(pubkey)
	got, err := ParseFingerprintFromKey(pubkeyStr)
	if err != nil {
		t.Fatalf("ParseFingerprintFromKey() error = %v", err)
	}

	if got != expected {
		t.Errorf("ParseFingerprintFromKey() = %s, want %s", got, expected)
	}
}

func TestParseFingerprintFromKey_Invalid(t *testing.T) {
	_, err := ParseFingerprintFromKey("not a valid key")
	if err == nil {
		t.Error("ParseFingerprintFromKey() should error on invalid key")
	}
}

func TestExtractSSHAuthFromMetadata_AllPresent(t *testing.T) {
	md := map[string][]string{
		SSHPubkeyHeader:    {"ssh-ed25519 AAAA..."},
		SSHSignatureHeader: {"dGVzdA=="},
		SSHTimestampHeader: {"1234567890"},
		SSHNonceHeader:     {"test-nonce"},
	}

	req := ExtractSSHAuthFromMetadata(md)
	if req == nil {
		t.Fatal("ExtractSSHAuthFromMetadata() returned nil")
	}

	if req.Pubkey != "ssh-ed25519 AAAA..." {
		t.Errorf("Pubkey = %s, want ssh-ed25519 AAAA...", req.Pubkey)
	}
	if req.Signature != "dGVzdA==" {
		t.Errorf("Signature = %s, want dGVzdA==", req.Signature)
	}
	if req.Timestamp != 1234567890 {
		t.Errorf("Timestamp = %d, want 1234567890", req.Timestamp)
	}
	if req.Nonce != "test-nonce" {
		t.Errorf("Nonce = %s, want test-nonce", req.Nonce)
	}
}

func TestExtractSSHAuthFromMetadata_NoHeaders(t *testing.T) {
	md := map[string][]string{
		"authorization": {"Bearer token"},
	}

	req := ExtractSSHAuthFromMetadata(md)
	if req != nil {
		t.Error("ExtractSSHAuthFromMetadata() should return nil when no SSH headers")
	}
}

func TestExtractSSHAuthFromMetadata_PartialHeaders(t *testing.T) {
	// If any SSH header is present, treat it as SSH auth attempt
	md := map[string][]string{
		SSHPubkeyHeader: {"ssh-ed25519 AAAA..."},
		// Missing other headers
	}

	req := ExtractSSHAuthFromMetadata(md)
	if req == nil {
		t.Fatal("ExtractSSHAuthFromMetadata() should return non-nil for partial SSH headers")
	}

	if req.Pubkey != "ssh-ed25519 AAAA..." {
		t.Errorf("Pubkey = %s, want ssh-ed25519 AAAA...", req.Pubkey)
	}
}
