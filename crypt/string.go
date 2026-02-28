package crypt

// EncryptedString stores a symmetrically encrypted string value.
// It implements the MarshalText/UnmarshalText and MarshalBinary/UnmarshalBinary interfaces
// for transparent serialization/deserialization (e.g. YAML, JSON, TOML).
type EncryptedString struct {
	value string
}

// NewEncryptedString creates a new EncryptedString by encrypting the given plain text.
func NewEncryptedString(plainTextValue string) EncryptedString {
	c := NewSymmetricEncryption()
	c.SetPlainText(plainTextValue)
	return EncryptedString{value: c.GetCypherBase64()}
}

// NewDecryptedString decrypts an encrypted string and returns the plain text.
func NewDecryptedString(encryptedValue string) string {
	s := &EncryptedString{value: encryptedValue}
	return s.Value()
}

// Value returns the decrypted plain text value.
func (v *EncryptedString) Value() string {
	c := NewSymmetricEncryption()
	c.SetCypherBase64(v.value)
	pt, _ := c.GetPlainText()
	return pt
}

// String implements fmt.Stringer and returns the encrypted value.
// Note: returns encrypted value to prevent accidental plaintext logging.
func (v *EncryptedString) String() string {
	return v.value
}

// MarshalText implements encoding.TextMarshaler.
func (v *EncryptedString) MarshalText() ([]byte, error) {
	return []byte(v.value), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *EncryptedString) UnmarshalText(text []byte) error {
	v.value = string(text)
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (v *EncryptedString) MarshalBinary() ([]byte, error) {
	return []byte(v.value), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (v *EncryptedString) UnmarshalBinary(data []byte) error {
	v.value = string(data)
	return nil
}
