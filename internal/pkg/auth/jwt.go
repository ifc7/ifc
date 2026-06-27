package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenMalformed = errors.New("token is malformed")
	ErrTokenNoExpiry  = errors.New("token has no expiration claim")
)

// TokenExpiry holds the parsed expiration details of a JWT.
type TokenExpiry struct {
	ExpiresAt time.Time
	ExpiresIn time.Duration // time remaining until expiry (negative if already expired)
	IsExpired bool
}

// GetTokenExpiry extracts the expiration time from a JWT access token without
// verifying the signature. This is safe for inspecting your own tokens where
// you trust the source but don't have the signing key handy (e.g. an opaque
// access token returned by an OAuth server).
//
// If you DO have the signing key, use GetTokenExpiryVerified instead.
func GetTokenExpiry(tokenString string) (*TokenExpiry, error) {
	// Parse without verification — we only care about the exp claim.
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTokenMalformed, err)
	}

	return extractExpiry(token)
}

// extractExpiry pulls the exp claim out of an already-parsed token.
func extractExpiry(token *jwt.Token) (*TokenExpiry, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrTokenMalformed
	}

	// GetExpirationTime handles numeric date (NumericDate) and float64 variants.
	expDate, err := claims.GetExpirationTime()
	if err != nil || expDate == nil {
		return nil, ErrTokenNoExpiry
	}

	expiresAt := expDate.Time
	expiresIn := time.Until(expiresAt)

	return &TokenExpiry{
		ExpiresAt: expiresAt,
		ExpiresIn: expiresIn,
		IsExpired: expiresIn <= 0,
	}, nil
}
