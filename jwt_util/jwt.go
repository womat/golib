// Package jwt_util generates and validates signed JWTs for the services in
// this ecosystem. Tokens are signed with HS256 from a shared secret and carry
// a user name next to the registered claims; validation checks the signature,
// the expiry, and the issuer, subject and ID the caller expects, so a token
// minted for one application cannot be replayed against another.
//
// web.WithAuth is the usual consumer: it reads Authorization: Bearer and calls
// ValidateToken with the JwtSecret, JwtID and AppName from its web.Config.
//
// Every failure is reported through one of the package's sentinel errors, so a
// caller can tell an expired token from a forged one with errors.Is.
//
// # Example usage
//
//	func main() {
//	    const (
//	        issuer  = "demo_app" // the application name
//	        subject = "auth"
//	        id      = "demo_app-token-1" // rejects tokens minted for someone else
//	    )
//	    secret := os.Getenv("JWT_SECRET")
//
//	    token, err := jwt_util.GenerateToken("wolfgang", issuer, subject, id, secret, time.Hour)
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    claims, err := jwt_util.ValidateToken(token, issuer, subject, id, secret)
//	    switch {
//	    case errors.Is(err, jwt_util.ErrExpiredToken):
//	        log.Println("token expired, ask for a new one")
//	    case err != nil:
//	        log.Println("rejected:", err)
//	    default:
//	        log.Println("authenticated:", claims.User)
//	    }
//	}
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
		User:      user,
		Issuer:    issuer,
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        id,
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
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
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
