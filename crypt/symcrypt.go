// Package crypt collects the cryptographic helpers the services in this
// ecosystem share: bcrypt password hashing, AES-256-GCM symmetric encryption,
// Ed25519 key-file generation, and EncryptedString, a string type that
// marshals as ciphertext so a secret never lands in a YAML or JSON file in
// clear text.
//
// The symmetric encryption ships with a compiled-in default key that is
// public - it is in this repository. SymCrypt can be pointed at another key
// with SetKey, but EncryptedString cannot: SetKey is a method on SymCrypt and
// EncryptedString always uses the default. Treat EncryptedString as protection
// against accidental disclosure (a config file in a screenshot, a struct dumped
// to a log), never against anyone who can read the file. Read the "Security
// model" section of README.md before relying on any of it, including what
// SetKey does with a key that is not exactly 32 bytes.
//
// # Example usage
//
//	func main() {
//	    // Password hashing.
//	    hashed, err := crypt.Hash("s3cret", crypt.DefaultCost)
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    ok := crypt.Compare(hashed, "s3cret") // true
//
//	    // Symmetric encryption with an explicit key.
//	    cipherText := crypt.NewSymmetricEncryption().
//	        SetKey(os.Getenv("AES_KEY")).
//	        SetPlainText("connection string").
//	        GetCypherBase64()
//
//	    plain, err := crypt.NewSymmetricEncryption().
//	        SetKey(os.Getenv("AES_KEY")).
//	        SetCypherBase64(cipherText).
//	        GetPlainText()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // A secret in a config struct. Marshals as ciphertext, but only with
//	    // the compiled-in default key - see the note above.
//	    type config struct {
//	        Password crypt.EncryptedString `yaml:"password"`
//	    }
//	    cfg := config{Password: crypt.NewEncryptedString("s3cret")}
//	    _ = cfg.Password.Value() // "s3cret"
//
//	    // Ed25519 key pair, written to disk.
//	    pub, err := crypt.GenerateEd25519KeyFiles("./certs", "id_ed25519")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    _, _, _ = ok, plain, pub
//	}
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Do not change this constant. Use your own key in production environment.
const defaultSymmetricKey = "42aaee02Qb687X4d7cA9521T82464264"

// keyLen is the required AES key length in bytes (256 bit).
const keyLen = 32

var errCipherTextTooShort = errors.New("cipher text too short")

const (
	reset         = iota
	hasPlainText  = 1
	hasCipherText = 2
	hasEncrypted  = 4
	hasDecrypted  = 8
)

// SymCrypt provides AES-GCM symmetric encryption and decryption.
//
// Usage:
//
//	cipher := crypt.NewSymmetricEncryption().SetPlainText("mySecret").GetCypherBase64()
//	plain, err := crypt.NewSymmetricEncryption().SetCypherBase64(cipher).GetPlainText()
type SymCrypt struct {
	plainText string // readable text
	key       string // 32 char - key
	cipher    []byte // scrambled (encrypted) byte slice of plainText
	flag      int    // flag to see it the plaintext is already encrypted
}

// NewSymmetricEncryption returns a new SymCrypt with the default key.
// Use SetKey to override the key in production environments.
func NewSymmetricEncryption() *SymCrypt {
	return &SymCrypt{
		key:  defaultSymmetricKey,
		flag: reset,
	}
}

// SetKey sets the AES encryption key.
// The key is padded or truncated to exactly 32 bytes (256 bit).
func (s *SymCrypt) SetKey(key string) *SymCrypt {
	switch l := len(key); {
	case l < keyLen:
		s.key = key + defaultSymmetricKey[:keyLen-l]
	case l > keyLen:
		s.key = key[:keyLen]
	default:
		s.key = key
	}
	return s
}

// SetPlainText sets the plain text to be encrypted.
func (s *SymCrypt) SetPlainText(plainText string) *SymCrypt {
	s.plainText = plainText
	s.flag = hasPlainText
	return s
}

// GetCypherBase64 returns the encrypted plain text as a base64 encoded string.
// Returns an empty string on error.
func (s *SymCrypt) GetCypherBase64() string {
	if len(s.cipher) < 1 {
		_ = s.encrypt()
	}
	return base64.StdEncoding.EncodeToString(s.cipher)
}

// SetCypherBase64 adds s.cipher - but as base64 string.
func (s *SymCrypt) SetCypherBase64(base64String string) *SymCrypt {
	b, err := base64.StdEncoding.DecodeString(base64String)
	if err == nil {
		s.flag = hasCipherText
		s.cipher = b
	}
	return s
}

// GetPlainText returns the decrypted plain text.
func (s *SymCrypt) GetPlainText() (string, error) {
	err := s.decrypt()
	return s.plainText, err
}

// encrypt encrypts s.plainText and stores the result in s.cipher.
func (s *SymCrypt) encrypt() error {
	if s.flag&hasPlainText == hasPlainText && s.flag&hasEncrypted == hasEncrypted {
		return nil
	}

	if s.flag&hasPlainText != hasPlainText {
		return nil
	}

	cipher, err := s.byteEncrypt()
	if err != nil {
		return err
	}

	s.cipher = cipher
	s.flag |= hasEncrypted
	return nil
}

// decrypt decrypts s.cipher and stores the result in s.plainText.
func (s *SymCrypt) decrypt() error {
	if s.flag&hasCipherText == hasCipherText && s.flag&hasDecrypted == hasDecrypted {
		return nil
	}

	if s.flag&hasCipherText != hasCipherText {
		return errors.New("no cipher text")
	}

	b, err := s.byteDecrypt()
	if err != nil {
		return err
	}

	s.plainText = string(b)
	s.flag |= hasCipherText | hasPlainText | hasEncrypted | hasDecrypted
	return nil
}

// byteEncrypt encrypts and authenticates s.plainText using AES-GCM.
func (s *SymCrypt) byteEncrypt() ([]byte, error) {

	c, err := aes.NewCipher([]byte(s.key))
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, []byte(s.plainText), nil), nil
}

// byteDecrypt decrypts and authenticates s.cipher using AES-GCM.
func (s *SymCrypt) byteDecrypt() ([]byte, error) {

	c, err := aes.NewCipher([]byte(s.key))
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(s.cipher) < nonceSize {
		return nil, errCipherTextTooShort
	}

	nonce, ciphertext := s.cipher[:nonceSize], s.cipher[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
