// ABOUTME: Encryption setup for fold-matrix bridge
// ABOUTME: Configures E2EE with recovery key for Matrix rooms using mautrix crypto

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
)

// CryptoManager handles Matrix E2EE setup and lifecycle.
type CryptoManager struct {
	helper      *cryptohelper.CryptoHelper
	recoveryKey string
	logger      *slog.Logger
}

// SetupCrypto initializes E2EE for the Matrix client.
// If recoveryKey is empty, encryption is still enabled but without cross-signing.
// The dataDir is used to store the SQLite crypto database.
func SetupCrypto(ctx context.Context, client *mautrix.Client, userID string, recoveryKey string, dataDir string, logger *slog.Logger) (*CryptoManager, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	// Include user ID in db path for isolation (like gorp-rs)
	userSlug := slugify(userID)
	dbPath := filepath.Join(dataDir, fmt.Sprintf("matrix-crypto-%s.db", userSlug))
	logger.Info("setting up encryption", "db", dbPath, "user", userSlug)

	// Derive store key from user ID for per-user isolation.
	// This ensures each user's crypto store has a unique encryption key.
	// Using nil would skip store encryption entirely (like gorp-rs does).
	storeKey := deriveStoreKey(userID)

	// Create crypto helper with SQLite database path.
	// The cryptohelper will automatically create the necessary stores.
	helper, err := cryptohelper.NewCryptoHelper(client, storeKey, dbPath)
	if err != nil {
		return nil, fmt.Errorf("creating crypto helper: %w", err)
	}

	// Initialize the crypto helper
	if err := helper.Init(ctx); err != nil {
		return nil, fmt.Errorf("initializing crypto helper: %w", err)
	}

	manager := &CryptoManager{
		helper:      helper,
		recoveryKey: recoveryKey,
		logger:      logger,
	}

	// If recovery key is provided, verify with it for cross-signing
	if recoveryKey != "" {
		if err := manager.verifyWithRecoveryKey(ctx); err != nil {
			// Log the error but don't fail - encryption still works without cross-signing
			logger.Warn("failed to verify with recovery key", "error", err)
			logger.Info("encryption enabled without cross-signing verification")
		} else {
			logger.Info("encryption initialized with cross-signing verification")
		}
	} else {
		logger.Info("encryption initialized (no recovery key - cross-signing disabled)")
	}

	return manager, nil
}

// verifyWithRecoveryKey attempts to verify the device using the configured recovery key.
// This enables cross-signing verification with other devices.
func (cm *CryptoManager) verifyWithRecoveryKey(ctx context.Context) error {
	machine := cm.helper.Machine()
	if machine == nil {
		return fmt.Errorf("crypto machine not initialized")
	}

	cm.logger.Info("verifying device with recovery key")

	// Use the Olm machine's recovery key verification
	if err := machine.VerifyWithRecoveryKey(ctx, cm.recoveryKey); err != nil {
		return fmt.Errorf("recovery key verification failed: %w", err)
	}

	cm.logger.Info("device verified with recovery key")
	return nil
}

// Helper returns the underlying CryptoHelper for advanced operations.
func (cm *CryptoManager) Helper() *cryptohelper.CryptoHelper {
	return cm.helper
}

// Close cleans up crypto resources.
func (cm *CryptoManager) Close() error {
	if cm.helper != nil {
		return cm.helper.Close()
	}
	return nil
}

// slugify converts a Matrix user ID to a filesystem-safe string.
// Example: @foldbot:matrix.org -> foldbot_matrix.org
func slugify(userID string) string {
	// Remove leading @ and replace : with _
	s := userID
	if len(s) > 0 && s[0] == '@' {
		s = s[1:]
	}
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == ':' {
			result = append(result, '_')
		}
	}
	return string(result)
}

// deriveStoreKey creates a deterministic store encryption key from user ID.
// This ensures each user has a unique key without requiring external secrets.
func deriveStoreKey(userID string) []byte {
	// Use SHA-256 to derive a 32-byte key from the user ID.
	// This provides per-user isolation while being deterministic.
	h := sha256.Sum256([]byte("fold-matrix-crypto:" + userID))
	return h[:]
}
