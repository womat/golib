# jwt_util

Signed JWTs for the services in this ecosystem: one function to mint a token,
one to validate it, and five sentinel errors that say why a token was refused.

It is a narrow wrapper around `github.com/golang-jwt/jwt/v5`, not a general JWT
library. The shape is fixed — HS256, a shared secret, a `user` claim next to the
registered ones — and the point of it is that `web.WithAuth` and the token
issuer cannot drift apart.

## API

| Function | Comment |
|---|---|
| `func GenerateToken(user, issuer, subject, id, secret string, lifetime time.Duration) (string, error)` | signs a token with HS256; the error is the signing error |
| `func ValidateToken(tokenString string, issuer, subject, id, secret string) (*Claims, error)` | checks signature, expiry, issuer, subject and ID |
| `type Claims struct{ User string; jwt.RegisteredClaims }` | what `ValidateToken` returns on success |

Sentinel errors: `ErrInvalidToken`, `ErrExpiredToken`, `ErrInvalidIssuer`,
`ErrInvalidSubject`, `ErrInvalidID`. Unlike `gpio` and `mqtt` their messages
carry no package prefix.

```go
const (
    issuer  = "demo_app" // the application name
    subject = "auth"
    id      = "demo_app-token-1"
)

token, err := jwt_util.GenerateToken("wolfgang", issuer, subject, id, secret, time.Hour)
if err != nil {
    return err
}

claims, err := jwt_util.ValidateToken(token, issuer, subject, id, secret)
switch {
case errors.Is(err, jwt_util.ErrExpiredToken):
    // ask for a new token
case err != nil:
    // refuse
default:
    log.Println("authenticated:", claims.User)
}
```

## What the four string arguments are for

`GenerateToken` and `ValidateToken` take the same four strings, and both sides
must agree on all of them:

- **`issuer`** — the application that minted the token, by convention its
  module name.
- **`subject`** — what the token is for. `web.WithAuth` hardcodes `"auth"`, so
  a token meant for that middleware must use it.
- **`id`** — a per-application token identifier. It is the mechanism that stops
  a token minted for one service from being accepted by another that happens to
  share the secret.
- **`secret`** — the HMAC key. A wrong secret fails the signature check and
  reads as `ErrInvalidToken`, indistinguishable from a forged token.

`ValidateToken` checks them in order: signature and expiry first, then issuer,
subject and ID. Only the first failure is reported.

## Use through web.WithAuth

`web.WithAuth` is the usual consumer. It reads the `Authorization: Bearer`
header and calls:

```go
jwt_util.ValidateToken(token, config.AppName, "auth", config.JwtID, config.JwtSecret)
```

So a service's `web.Config` fixes three of the four arguments: `AppName` is the
issuer, `JwtID` the ID, `JwtSecret` the secret, and the subject is always
`"auth"`. `WithAuth` discards the error and answers 401 — it does not
distinguish an expired token from an invalid one, and it does not log which it
was. Code that needs the difference must call `ValidateToken` itself.

`WithAuth` stores the validated user in the request context under an unexported
key, so `Claims.User` is not reachable from a handler. See the *Not addressed*
section of [`web/README.md`](../web/README.md).

## Testing

```sh
go test -race ./jwt_util/
```

Coverage is 92.3 % of statements: a valid token plus one case per rejection
reason (tampered token, wrong issuer, wrong subject, wrong ID, wrong secret,
expired). The tests assert that an error occurred, not *which* sentinel it was,
so a wiring mistake that reported `ErrInvalidToken` for an expired token would
pass.

## Not addressed

Known and deliberately left alone:

- **HS256 only when signing, any HMAC when validating.** `GenerateToken`
  always uses HS256; `ValidateToken` accepts any `SigningMethodHMAC`, so a
  token signed HS384 or HS512 with the same secret is honoured. Asymmetric
  algorithms are rejected, which is the attack that matters.
- **No key rotation.** One secret, no `kid` header, no list of acceptable keys.
  Rotating means restarting both sides at once.
- **No refresh tokens and no revocation.** A token is valid until it expires;
  the only way to invalidate one early is to change the secret or the ID, which
  invalidates all of them.
- **No audience check.** `aud` is neither set nor verified — `id` does that job
  instead.
- **The `id` is not unique per token.** Despite the name it is a per-application
  constant, so it cannot be used to detect replay of an individual token.

Full API: `go doc github.com/womat/golib/jwt_util`.
