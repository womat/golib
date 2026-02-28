package jwt_util

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken   = errors.New("token is invalid")
	ErrExpiredToken   = errors.New("token has expired")
	ErrInvalidIssuer  = errors.New("token has invalid issuer")
	ErrInvalidSubject = errors.New("token has invalid subject")
	ErrInvalidID      = errors.New("token has invalid ID")
)

// Claims represents the JWT token claims.
type Claims struct {
	User string `json:"user"`
	jwt.RegisteredClaims
}

// GenerateToken generates a signed JWT token for the given user.
//
//	username: the user's name
//	issuer: the token issuer, e.g. the application name
//	subject: the token subject, e.g. auth
//	id: unique app token id to prevent token reuse
//	secret: the secret used to sign the token
//	lifetime: the token lifetime
func GenerateToken(user, issuer, subject, id, secret string, lifetime time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		User: user,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        id,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken parses and validates a JWT token string.
// Returns the claims or an error if the token is invalid, expired, or has unexpected issuer/subject/id.
//
//	tokenString: the token string
//	issuer: the token issuer, e.g. the application name
//	subject: the token subject, e.g. auth
//	id: unique app token id
//	secret: the secret used to sign the token
func ValidateToken(tokenString string, issuer, subject, id, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.Issuer != issuer {
		return nil, ErrInvalidIssuer
	}

	if claims.Subject != subject {
		return nil, ErrInvalidSubject
	}

	if claims.ID != id {
		return nil, ErrInvalidID
	}

	return claims, nil
}
