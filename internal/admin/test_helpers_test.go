// ABOUTME: Shared test helpers for admin package tests
// ABOUTME: Provides common test resources like test secrets to ensure consistent test execution

package admin

// testSecret is a 32-byte secret that meets MinSecretLength requirement.
// This is shared across test files to avoid fragile dependencies on file evaluation order.
var testSecret = []byte("admin-token-test-secret-32bytes!")
