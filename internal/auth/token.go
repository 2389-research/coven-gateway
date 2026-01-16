// ABOUTME: JWT token verification for authenticating gRPC requests
// ABOUTME: Uses HS256 signing with configurable secret

package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token errors
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
	ErrMissingClaim = errors.New("missing required claim")
)

// TokenVerifier defines the interface for token verification
type TokenVerifier interface {
	Verify(tokenString string) (principalID string, err error)
}

// JWTVerifier implements TokenVerifier using HS256 signed JWTs
type JWTVerifier struct {
	secret []byte
}

// NewJWTVerifier creates a new JWT verifier with the given secret
func NewJWTVerifier(secret []byte) *JWTVerifier {
	return &JWTVerifier{secret: secret}
}

// Verify validates the token and extracts the principal ID from the "sub" claim
func (v *JWTVerifier) Verify(tokenString string) (principalID string, err error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method is HS256
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.secret, nil
	})

	if err != nil {
		// Check if it's specifically an expiration error
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpiredToken
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return "", ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", ErrInvalidToken
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", fmt.Errorf("%w: sub", ErrMissingClaim)
	}

	return sub, nil
}

// Generate creates a new JWT token for the given principal ID with expiration
func (v *JWTVerifier) Generate(principalID string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": principalID,
		"iat": now.Unix(),
		"exp": now.Add(expiresIn).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(v.secret)
}
