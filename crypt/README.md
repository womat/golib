
# package crypt

| Function                                                                  | Comment                                                     |
|---------------------------------------------------------------------------|-------------------------------------------------------------|
| func NewBcrypt() Bcrypt                                                   | creates an bcrypt handler                                   |
| func (b *bcryptProperties) Encrypt(plainText string) (string, error)      | encrypt plaintext                                           |
| func (b *bcryptProperties) HashedText(hashedText string)                  | set a hashed text string                                    |
| func (b *bcryptProperties) PlainText(plainText string)                    | set a plain text string                                     |
| func (b *bcryptProperties) Cost(cost int)                                 | sets bcrypt costs                                           |
| func (b *bcryptProperties) Compare() bool                                 | checks the hash against the plaintext                       |                 
| func GenerateEd25519KeyFiles(dir string, filename string) (string, error) | generates ed25519 key files (id_ed25519 and id_ed25519.pub) |                 
| func NewEncryptedString(plainTextValue string) EncryptedString            | creates an Encrypted string                                 |                 
| func NewDecryptedString(encryptedValue string) string                     | decrypt an encrypted string                                 |                 
| func (v EncryptedString) Value() string                                   | returns the decrypted plainTextValue                        |                 
| func (v EncryptedString) String() string                                  | returns the decrypted plainTextValue                        |                 
| func (v EncryptedString) MarshalText() ([]byte, error)                    |                                                             |                 
| func (v *EncryptedString) UnmarshalText(text []byte) error                |                                                             |                 
| func (v EncryptedString) MarshalBinary() ([]byte, error)                  |                                                             |                 
| func NewSymmetricEncryption() *SymCrypt                                   | creates an SymCrypt handler                                 |                 
| func (s *SymCrypt) SetKey(key string) *SymCrypt                           | set an AES key                                              |                 
| func (s *SymCrypt) SetPlainText(plainText string) *SymCrypt               | set plain text to encrypt.                                  |                 
| func (s *SymCrypt) GetCypherBase64() string                               | returns the encrypted data stream as base64 encoded         |                 
| func (s *SymCrypt) SetCypherBase64(base64String string) *SymCrypt         | set cipher text as base64 string.                           |                 
| func (s *SymCrypt) GetPlainText() (string, error)                         | returns the plaintext                                       |                 
