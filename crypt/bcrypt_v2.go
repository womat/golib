package crypt

import "golang.org/x/crypto/bcrypt"

const DefaultCost = bcrypt.DefaultCost

// Hash generates a bcrypt hash from plainText.
func Hash(plainText string, cost int) (string, error) {
	if cost < bcrypt.MinCost {
		cost = bcrypt.MinCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plainText), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Compare reports whether plainText matches the bcrypt hash.
func Compare(hashedText, plainText string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedText), []byte(plainText)) == nil
}
