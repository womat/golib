# crypt

Four unrelated pieces of applied cryptography that services in this ecosystem
kept needing: bcrypt password hashing, AES-256-GCM symmetric encryption, an
`EncryptedString` that survives a round trip through YAML or JSON without ever
holding plain text, and an Ed25519 key-file generator in OpenSSH format.

Read [Security model](#security-model) before you use `SymCrypt` or
`EncryptedString` for anything that actually needs to stay secret. The short
version: the AES key is compiled in, and `EncryptedString` cannot be given a
different one.

## API

| Function | Comment |
|---|---|
| `func Hash(plainText string, cost int) (string, error)` | bcrypt hash; `cost` is clamped to `[bcrypt.MinCost, bcrypt.MaxCost]` |
| `func Compare(hashedText, plainText string) bool` | reports whether the plain text matches the hash — hash first |
| `const DefaultCost` | `bcrypt.DefaultCost` (10) |
| `func NewBcrypt() Bcrypt` | legacy stateful bcrypt handler, see [Two bcrypt APIs](#two-bcrypt-apis) |
| `Bcrypt.Encrypt(plainText string) (string, error)` | hash and remember the plain text |
| `Bcrypt.HashedText(hashedText string)` | set the hash to compare against |
| `Bcrypt.PlainText(plainText string)` | set the plain text to compare |
| `Bcrypt.Cost(cost int)` | set the cost, clamped to the legal range |
| `Bcrypt.Compare() bool` | compare the stored hash against the stored plain text |
| `func GenerateEd25519KeyFiles(dir, filename string) (string, error)` | write an OpenSSH key pair, returns the path of the **public** key |
| `func NewEncryptedString(plainTextValue string) EncryptedString` | encrypt with the default key |
| `func NewDecryptedString(encryptedValue string) string` | decrypt with the default key, `""` on failure |
| `func (v *EncryptedString) Value() string` | the decrypted plain text, `""` on failure |
| `func (v *EncryptedString) String() string` | the **encrypted** value, so a stray `%v` cannot leak the secret |
| `func (v *EncryptedString) MarshalText() ([]byte, error)` | writes the ciphertext |
| `func (v *EncryptedString) UnmarshalText(text []byte) error` | stores the input verbatim as ciphertext, no validation |
| `func (v *EncryptedString) MarshalBinary() ([]byte, error)` | as `MarshalText` |
| `func (v *EncryptedString) UnmarshalBinary(data []byte) error` | as `UnmarshalText` |
| `func NewSymmetricEncryption() *SymCrypt` | AES-256-GCM handler, primed with the default key |
| `func (s *SymCrypt) SetKey(key string) *SymCrypt` | set the AES key — padded or truncated to 32 bytes, see below |
| `func (s *SymCrypt) SetPlainText(plainText string) *SymCrypt` | set the plain text to encrypt |
| `func (s *SymCrypt) GetCypherBase64() string` | the ciphertext, base64 encoded; `""` on error |
| `func (s *SymCrypt) SetCypherBase64(b64 string) *SymCrypt` | set the ciphertext from base64; invalid input is ignored |
| `func (s *SymCrypt) GetPlainText() (string, error)` | decrypt and authenticate |

Note the receivers: every method of `EncryptedString` takes a pointer. That
matters when marshalling, see [Marshalling only works through a
pointer](#marshalling-only-works-through-a-pointer).

## Symmetric encryption

```go
cipher := crypt.NewSymmetricEncryption().SetPlainText("mySecret").GetCypherBase64()
plain, err := crypt.NewSymmetricEncryption().SetCypherBase64(cipher).GetPlainText()
```

The mode is AES-256-GCM, so the ciphertext is authenticated and there is no
padding. A fresh 12-byte nonce is drawn from `crypto/rand` for every
encryption and prepended to the ciphertext; the layout is
`nonce || ciphertext || tag`. Encrypting the same plain text twice therefore
yields two different results, which is correct and not a bug.

**Use one instance per value.** `SetPlainText` does not invalidate a
ciphertext that is already there, and `GetCypherBase64` only encrypts when the
ciphertext is empty:

```go
s := crypt.NewSymmetricEncryption()
a := s.SetPlainText("first").GetCypherBase64()
b := s.SetPlainText("second").GetCypherBase64()  // a == b — still "first"
```

## EncryptedString

```go
type Config struct {
    ApiKey crypt.EncryptedString `yaml:"apiKey"`
}

cfg := Config{ApiKey: crypt.NewEncryptedString("s3cr3t")}
out, _ := yaml.Marshal(&cfg)   // apiKey: BGmhuY0O2Tbdo3...
key := cfg.ApiKey.Value()      // "s3cr3t"
```

The value is encrypted once at construction and stays encrypted in memory. It
is decrypted only by `Value()`; `String()` deliberately returns the ciphertext
so that logging the struct cannot leak the secret.

Unmarshalling does not validate. A configuration file that carries plain text
where ciphertext was expected is accepted without complaint, and `Value()`
then returns `""`. A wrong key produces the same `""`, so an empty result is
never proof of an empty secret.

## Security model

**The AES key is compiled into the binary.** `defaultSymmetricKey`
(`symcrypt.go:67`) is a constant in this repository — public, and identical in
every program that links the package.

**`EncryptedString` always uses that default key.** `NewEncryptedString` and
`Value()` construct a `SymCrypt` without calling `SetKey`, and `SetKey` is a
method on `SymCrypt`, not a package-level function: there is no way to hand
`EncryptedString` a key of your own. So treat it as protection against
*accidental* disclosure — a secret that would otherwise sit in clear text in a
YAML file, a screenshot, or a struct dumped into a log. It is not protection
against anyone who can read that file: they can decrypt it with three lines of
code from this repository.

**`SetKey` is not a key derivation function.** It pads or truncates the string
to exactly 32 bytes (`SetKey`, `symcrypt.go:106-116`), and it pads with a prefix of the
default key:

| call | resulting key |
|---|---|
| `SetKey("")` | the default key, in full |
| `SetKey("secret")` | 6 secret bytes plus 26 publicly known ones |
| `SetKey(<40 bytes>)` | the first 32 bytes, the rest silently dropped |

There is no PBKDF2, scrypt or HKDF, and no salt. Pass 32 bytes of real
randomness, not a passphrase.

**Errors are swallowed in several places.** `GetCypherBase64` returns `""`
rather than an error; `SetCypherBase64` ignores invalid base64 and keeps the
previous state; encrypting without a plain text is a silent no-op; `Value()`
and `NewDecryptedString` turn a failed decryption into `""`. Only
`GetPlainText` reports anything, and it is the one call worth checking.

### Marshalling only works through a pointer

All methods of `EncryptedString` have pointer receivers, and `encoding/json`
and `yaml.v3` only find `MarshalText` on an addressable value. Marshal a
struct *by value* and the interface is not used; since the single field of
`EncryptedString` is unexported, what gets written is an empty object and the
secret is gone:

```go
json.Marshal(cfg)    // {"apiKey":{}}       — wrong
json.Marshal(&cfg)   // {"apiKey":"BGm..."} — right
```

Unmarshalling is unaffected, because a pointer is passed there anyway.

### Two bcrypt APIs

`Hash` and `Compare` are the ones to use. The `Bcrypt` interface returned by
`NewBcrypt` is older, keeps its state in the handler, and defaults to cost 4 —
that is `bcrypt.MinCost`, fast enough to be a poor choice for passwords. Its
`Compare` returns a bare `bool`, so a corrupt hash is indistinguishable from a
wrong password.

Both parameters of `Compare(hashedText, plainText string)` are strings and the
compiler will not catch a swap. The hash comes first.

### Ed25519 key files

`GenerateEd25519KeyFiles` writes the private key with mode `0600` and the
public key with `0644`, subject to the process umask; it returns the path of
the *public* key. The private key is stored unencrypted — there is no
passphrase option. The public key carries an RFC3339 timestamp as its comment
field. Existing files are not overwritten, but the check is an `os.Stat`
followed by an `os.WriteFile` without `O_EXCL`, so it does not hold against
concurrent callers.

## Testing

```sh
go test ./crypt/...
go test ./crypt/... -run TestAES -v
```

Coverage is 69.2 %. `string.go` and `bcrypt_v2.go` have **no tests at all** —
neither the YAML/JSON round trip documented above nor `Hash`/`Compare` is
covered. `TestAESError` passes for the wrong reason: `"x"` already fails
base64 decoding, so the "cipher text too short" branch is never reached.

## Not addressed

Known and deliberately left alone, so that a future major version does not
have to rediscover it:

- No package-level key setter, so `EncryptedString` is stuck with the compiled-in
  default key.
- `SetKey` pads and truncates instead of rejecting a key that is not 32 bytes.
- Pointer receivers on the marshal methods; value receivers would remove the
  by-value pitfall above.
- `bcrypt.go` duplicates `bcrypt_v2.go` with a worse default cost.
- `ed25519.go` imports the deprecated `golang.org/x/crypto/ed25519` instead of
  `crypto/ed25519`, and depends on `github.com/mikesmitty/edkey`, untouched
  since 2017. `ssh.MarshalPrivateKey` in the already-vendored
  `golang.org/x/crypto v0.48.0` does the same job and would drop the
  dependency.
- No package doc comment with the `Example usage` block this repository uses
  elsewhere.
