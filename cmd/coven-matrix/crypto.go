// ABOUTME: Encryption setup for coven-matrix bridge
// ABOUTME: Configures E2EE with recovery key for Matrix rooms using mautrix crypto

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
)

// CryptoManager handles Matrix E2EE setup and lifecycle.
type CryptoManager struct {
	helper *cryptohelper.CryptoHelper
	logger *slog.Logger
}

// SetupCrypto initializes E2EE for the Matrix client.
// If recoveryKey is empty, encryption is still enabled but without cross-signing.
// The dataDir is used to store the SQLite crypto database.
// If a device ID mismatch is detected, the crypto database is automatically reset.
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

	// Try to create and init crypto helper, with auto-recovery on device ID mismatch
	helper, err := initCryptoHelper(ctx, client, storeKey, dbPath, logger)
	if err != nil {
		return nil, err
	}

	// Wire up the crypto helper to the client for automatic encryption of outgoing messages
	client.Crypto = helper

	manager := &CryptoManager{
		helper: helper,
		logger: logger,
	}

	// If recovery key is provided, verify with it for cross-signing
	if recoveryKey != "" {
		if err := manager.verifyWithRecoveryKey(ctx, recoveryKey); err != nil {
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

// verifyWithRecoveryKey attempts to verify the device using the provided recovery key.
// This enables cross-signing verification with other devices.
func (cm *CryptoManager) verifyWithRecoveryKey(ctx context.Context, recoveryKey string) error {
	machine := cm.helper.Machine()
	if machine == nil {
		return fmt.Errorf("crypto machine not initialized")
	}

	cm.logger.Info("verifying device with recovery key")

	// Use the Olm machine's recovery key verification
	if err := machine.VerifyWithRecoveryKey(ctx, recoveryKey); err != nil {
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
// Example: @covenbot:matrix.org -> covenbot_matrix.org
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
	h := sha256.Sum256([]byte("coven-matrix-crypto:" + userID))
	return h[:]
}

// initCryptoHelper creates and initializes the crypto helper, with auto-recovery
// on device ID mismatch. This handles the case where a new login creates a new
// device ID but the crypto store still has keys for the old device.
func initCryptoHelper(ctx context.Context, client *mautrix.Client, storeKey []byte, dbPath string, logger *slog.Logger) (*cryptohelper.CryptoHelper, error) {
	// Check for device ID mismatch BEFORE creating helper to avoid DB lock issues
	if needsReset, err := checkDeviceIDMismatch(dbPath, client.DeviceID.String()); err != nil {
		logger.Debug("could not check device ID", "error", err)
	} else if needsReset {
		logger.Warn("device ID mismatch detected, resetting crypto database before init")
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing old crypto database: %w", err)
		}
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
		logger.Info("crypto database reset")
	}

	helper, err := cryptohelper.NewCryptoHelper(client, storeKey, dbPath)
	if err != nil {
		return nil, fmt.Errorf("creating crypto helper: %w", err)
	}

	if err := helper.Init(ctx); err != nil {
		return nil, fmt.Errorf("initializing crypto helper: %w", err)
	}

	return helper, nil
}

// checkDeviceIDMismatch opens the crypto database and checks if the stored device ID
// matches the current client device ID. Returns true if DB exists and has a different device ID.
func checkDeviceIDMismatch(dbPath string, currentDeviceID string) (bool, error) {
	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return false, nil // No DB, no mismatch
	}

	// Open database directly to check stored device ID
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return false, err
	}
	defer db.Close()

	// Query for stored device ID - mautrix stores it in crypto_account table
	var storedDeviceID string
	err = db.QueryRow("SELECT device_id FROM crypto_account LIMIT 1").Scan(&storedDeviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // No account stored yet
		}
		return false, err
	}

	return storedDeviceID != currentDeviceID, nil
}

// isDeviceIDMismatch checks if the error is due to a device ID mismatch.
func isDeviceIDMismatch(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "mismatching device ID")
}
