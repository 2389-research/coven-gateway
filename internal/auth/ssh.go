// ABOUTME: SSH public key authentication for agents
// ABOUTME: Verifies signatures over timestamp|nonce to authenticate agent connections

package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// SSHAuthMaxAge is the maximum age of a signature timestamp (5 minutes)
	SSHAuthMaxAge = 5 * time.Minute

	// SSH auth metadata keys
	SSHPubkeyHeader    = "x-ssh-pubkey"
	SSHSignatureHeader = "x-ssh-signature"
	SSHTimestampHeader = "x-ssh-timestamp"
	SSHNonceHeader     = "x-ssh-nonce"
)

// SSHAuthRequest contains the data sent by an agent for SSH authentication
type SSHAuthRequest struct {
	Pubkey    string // Full public key (e.g., "ssh-ed25519 AAAA...")
	Signature string // Base64-encoded signature over "timestamp|nonce"
	Timestamp int64  // Unix timestamp
	Nonce     string // Random string to prevent replay
}

// SSHVerifier verifies SSH signatures for agent authentication
type SSHVerifier struct {
	maxAge time.Duration
}

// NewSSHVerifier creates a new SSH signature verifier
func NewSSHVerifier() *SSHVerifier {
	return &SSHVerifier{
		maxAge: SSHAuthMaxAge,
	}
}

// Verify checks the SSH signature and returns the pubkey fingerprint if valid.
// The signature must be over the string "timestamp|nonce".
func (v *SSHVerifier) Verify(req *SSHAuthRequest) (fingerprint string, err error) {
	// Parse the public key
	pubkey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.Pubkey))
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}

	// Check timestamp is recent
	signedAt := time.Unix(req.Timestamp, 0)
	age := time.Since(signedAt)
	if age < 0 {
		// Timestamp is in the future - allow small clock skew
		if age < -time.Minute {
			return "", fmt.Errorf("timestamp is in the future")
		}
	} else if age > v.maxAge {
		return "", fmt.Errorf("signature expired (age: %v, max: %v)", age, v.maxAge)
	}

	// Build the message that was signed: "timestamp|nonce"
	message := fmt.Sprintf("%d|%s", req.Timestamp, req.Nonce)

	// Decode the signature
	sigBytes, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		return "", fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Parse the SSH signature
	sig := new(ssh.Signature)
	if err := ssh.Unmarshal(sigBytes, sig); err != nil {
		return "", fmt.Errorf("invalid signature format: %w", err)
	}

	// Verify the signature
	if err := pubkey.Verify([]byte(message), sig); err != nil {
		return "", fmt.Errorf("signature verification failed: %w", err)
	}

	// Compute the fingerprint (SHA256 of the public key)
	fp := ComputeFingerprint(pubkey)

	return fp, nil
}

// ComputeFingerprint computes the SHA256 fingerprint of a public key.
// Returns lowercase hex encoding without colons.
func ComputeFingerprint(pubkey ssh.PublicKey) string {
	hash := sha256.Sum256(pubkey.Marshal())
	return fmt.Sprintf("%x", hash[:])
}

// ParseFingerprintFromKey parses a public key string and returns its fingerprint.
// Useful for registering agents.
func ParseFingerprintFromKey(pubkeyStr string) (string, error) {
	pubkey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubkeyStr))
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}
	return ComputeFingerprint(pubkey), nil
}

// ExtractSSHAuthFromMetadata extracts SSH auth fields from gRPC metadata.
// Returns nil if no SSH auth headers are present.
func ExtractSSHAuthFromMetadata(md map[string][]string) *SSHAuthRequest {
	getPrimary := func(key string) string {
		if vals, ok := md[key]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}

	pubkey := getPrimary(SSHPubkeyHeader)
	signature := getPrimary(SSHSignatureHeader)
	timestampStr := getPrimary(SSHTimestampHeader)
	nonce := getPrimary(SSHNonceHeader)

	// If any SSH header is present, treat it as SSH auth attempt
	if pubkey == "" && signature == "" && timestampStr == "" && nonce == "" {
		return nil
	}

	timestamp, _ := strconv.ParseInt(timestampStr, 10, 64)

	return &SSHAuthRequest{
		Pubkey:    strings.TrimSpace(pubkey),
		Signature: strings.TrimSpace(signature),
		Timestamp: timestamp,
		Nonce:     strings.TrimSpace(nonce),
	}
}
