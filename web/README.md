# web

Composable `http.Handler` middleware for the services in this ecosystem, plus
the JSON helpers they all repeat: authentication with bearer tokens from a
token file, CORS, an IP filter, request logging, and one uniform error body.
Since v2 the token file is the only authentication: the API key, the
`X-Api-Key` header and JWTs are gone.

It is middleware, not a framework. Applications assemble a plain
`http.ServeMux` and wrap it; `demo/demo_app/app/routes.go` is the canonical
example. The [ordering](#ordering) matters, and the CORS default is `*` — read
both sections before putting this on a public interface.

## API

| Function | Comment |
|---|---|
| `func WithAuth(h http.Handler, config Config) http.Handler` | `Authorization: Bearer <token>` whose hash is in `Config.Tokens`, see [Tokens](#tokens); otherwise 401 with `WWW-Authenticate: Bearer` |
| `type Config struct{ Tokens []Token }` | the accepted tokens; an empty `Config` rejects everything |
| `type Token struct{ Name string; Hash [32]byte }` | one accepted bearer token: the name of its caller and the SHA-256 of the token |
| `func ReadTokenFile(path string) ([]Token, error)` | read a token file; a malformed line, a duplicate name or a duplicate hash is an error |
| `func CheckTokenFile(path string) error` | `ErrTokenFileWritable` if the running process could change the token file, its directory or one above; log it as a warning |
| `func AddTokenToFile(path, name string, hash [32]byte) error` / `func RemoveTokenFromFile(path, name string) error` | change a token file in place, atomically, keeping comments, mode, owner and group; `ErrInvalidTokenName`, `ErrTokenExists`, `ErrTokenNotFound`; add refuses a hash already in the file |
| `func NewToken() (string, error)` / `func HashToken(token string) [32]byte` / `func ValidTokenName(name string) bool` | 32 random bytes as hex; its hash; names are 1–64 of `A-Z a-z 0-9 . _ -` |
| `type TokenCommand struct{ … }` / `func (TokenCommand) Run(args []string) int` | the `token add/remove/list` subcommand for a service's `main` |
| `func WithCORS(next http.Handler, opts ...CORSOption) http.Handler` | adds the CORS headers and answers preflights itself (204) |
| `func WithAllowedOrigins(origins ...string) CORSOption` | restrict to a list, echo only on a match, set `Vary: Origin` |
| `func WithAllowedMethods(methods ...string) CORSOption` | replace the method list |
| `func WithAllowedHeaders(headers ...string) CORSOption` | replace the request-header list |
| `func HandlePreflight() http.Handler` | answers `OPTIONS` with 204; not needed behind `WithCORS` |
| `func ValidateIPList(entries []string) error` | the first entry `WithIPFilter` could not use; call it when validating the config |
| `func WithIPFilter(h http.Handler, allowedIPs, blockedIPs []string, opts ...IPFilterOption) http.Handler` | IP and CIDR allow/block lists, parsed once; 403 on rejection |
| `func WithIPFilterLogger(logger *slog.Logger) IPFilterOption` | where to report unusable list entries, which are found while the middleware is built |
| `func WithLogging(h http.Handler, logger *slog.Logger, opts ...LogOption) http.Handler` | one entry per request: method, path, status, size, duration; level follows the status |
| `func WithBodyLogging(maxBytes int) LogOption` | additionally log request and response body, truncated, textual types only |
| `func WithLogLevel(level slog.Level) LogOption` | write every entry at one level instead of following the status |
| `func WithSuccessLevel(level slog.Level) LogOption` | level for responses below 400, e.g. `Debug` for a polled service; from 400 the status still decides |
| `func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context` | put a logger where `WriteError` and `WithIPFilter` find it |
| `func LoggerFrom(ctx context.Context) *slog.Logger` | that logger, or `slog.Default()` when there is none |
| `func Encode[T any](w http.ResponseWriter, status int, v T)` | JSON response; a marshal failure yields a 500 with `ErrInternal`, never the marshal error |
| `func Decode[T any](w http.ResponseWriter, r *http.Request) (T, error)` | JSON request body, limited to 1 MiB, trailing data refused |
| `func WriteError(w http.ResponseWriter, r *http.Request, status int, err error, reason ...error)` | uniform error body, logged with its reasons |
| `func NewApiError(err error) ApiError` / `type ApiError struct{ Error string }` | the JSON error shape: `{"error":"…"}` |
| `func UserFrom(ctx context.Context) (string, bool)` | who `WithAuth` authorized the request as: the name of the token |

Sentinel errors: `ErrUnauthorized`, `ErrForbidden`, `ErrInternal`, and for
token files `ErrTokenExists`, `ErrTokenNotFound`, `ErrInvalidTokenName`,
`ErrTokenFileWritable`. They
carry no package prefix in their message (`"not authorized"`,
`"forbidden"`, `"internal server error"`) — the text reaches the client, where a
prefix would leak the implementation.

## Tokens

A service accepts one bearer token per caller and stores only its SHA-256
hash, in a token file next to its config:

```
# /opt/<app>/etc/token-hashes — one "<name> <sha256>" per line
nodered 3f2a…c91e
grafana 8b04…d7a2
```

Whoever reads the file cannot call the service with it, so it needs no more
protection than the config: owned by root, readable by the service's group,
**writable by nobody else** — write access means adding a token of one's own.
The token itself exists only with the caller, e.g. in Node-RED's encrypted
credentials.

```go
tokens, err := web.ReadTokenFile(cfg.Webserver.TokenFile) // stop on error
webCfg := web.Config{Tokens: tokens}

if err := web.CheckTokenFile(cfg.Webserver.TokenFile); err != nil {
    slog.Warn("Token file is not protected", "error", err) // keep running
}
```

`CheckTokenFile` catches the layout that breaks the last rule: the service
owns the file or its directory, or may write either. It checks every
directory above as well, up to the root: a service that owns `/opt/<service>`
can rename `etc` and put its own token file in place. It is a warning, not an
error, because a development checkout always looks like that, and because an
old layout should not stop a running service on upgrade. Run as root, it
checks nothing.

`WithAuth` hashes the bearer token of a request and compares it with every
entry, in constant time. A match puts the token's name into the request, and
`WithLogging` writes it into the access log as `user=nodered`, next to
`request.client`. Every log line therefore says who called, and a token that
is no longer in use shows by its absence.

### The `token` subcommand

`TokenCommand` is the command line for the file, ready to be hung into a
service's `main` (see its doc comment):

```
<app> token add <name>            write the hash, then print the token once
<app> token add <name> -dry-run   print token and line, write nothing
<app> token remove <name>         delete the line
<app> token list                  print the names, never a hash
```

`-config` works as it does for the service and defaults to the same file; the
token file is the one that config names. The token goes to stdout alone, so
`TOKEN=$(<app> token add nodered)` works; everything else goes to stderr.
`add` writes before it prints: a token that is shown but stored nowhere cannot
happen. A new token file gets mode 0640 and the group of the config file, so
the service reads it through its group; its owner is whoever ran `add`, root
on a server. The owner of the config file is deliberately not copied: where it
is the service user, the service could write its own token file. Changes
apply after a restart of the service; a service that rereads its token file
on reload says so through `TokenCommand.Apply`.

Rotating without a gap: `add nodered-2`, move the caller to the new token,
`remove nodered`, restart.

## Ordering

```go
mux := http.NewServeMux()
mux.Handle("GET /version", app.handleVersion())         // public
mux.Handle("GET /health", web.WithAuth(app.handleHealth(), webCfg))

handler := web.WithCORS(mux)
handler = web.WithIPFilter(handler, allowedIPs, blockedIPs)
handler = web.WithLogging(handler, logger)
```

**Preflight must not meet `WithAuth`.** A browser sends the `OPTIONS` preflight
*without* credentials; behind the auth middleware it would earn a 401, and
every cross-origin call would fail with a message that says nothing about the
real cause. `WithCORS` therefore answers it itself: an `OPTIONS` request with
`Access-Control-Request-Method` gets 204 and the CORS headers and never reaches
the mux. Any other `OPTIONS` request is passed on.

**No `OPTIONS /` route.** Services used to register
`mux.Handle("OPTIONS /", web.HandlePreflight())`. That pattern matches every
path, so the mux answers an unknown path with 405 instead of 404. Behind
`WithCORS` the route is not needed; drop it, and the mux answers 404 for an
unknown path and 405 for a known one asked with the wrong method. Keeping it
changes nothing else.

## CORS

Without options, `WithCORS` sets these headers on every response:

| Header | Default |
|---|---|
| `Access-Control-Allow-Origin` | `*` |
| `Access-Control-Allow-Methods` | `POST, GET, OPTIONS, PUT, PATCH, DELETE` |
| `Access-Control-Allow-Headers` | `Content-Type, Authorization` (v1 also listed `X-Api-Key`) |
| `Access-Control-Max-Age` | `86400` (one day; not configurable) |

That is the historical behaviour and it is kept as the default so that upgrading
the library changes no running service. `WithAllowedOrigins`,
`WithAllowedMethods` and `WithAllowedHeaders` each *replace* the corresponding
list — a custom header list must repeat `Content-Type` and the auth headers if
it still needs them.

It is workable for an API that is guarded by a bearer token and an IP filter, and
wrong for one that relies on browser credentials: with `*`, any page on the
internet can read the responses its visitor's browser is allowed to fetch.
Restrict it per service:

```go
web.WithCORS(mux, web.WithAllowedOrigins("https://dashboard.example"))
```

With a list, the request's `Origin` is echoed back only when it is on it, and
`Vary: Origin` is set so that a cache in front cannot serve one origin's
response to another.

## IP filter

Both lists accept plain addresses (`127.0.0.1`, `::1`) and CIDR networks
(`192.168.0.0/16`). They are parsed once, when the middleware is built:

- An unusable entry — a typo'd CIDR, a hostname — is logged at error level and
  skipped. It cannot silently turn into a rule that matches nothing.
- If that leaves a *configured* allowlist without a single usable entry,
  everything is rejected. The safe direction, and loud in the log.
- Block wins over allow. An empty allowlist allows everything; two empty lists
  return the handler unwrapped.
- A request whose `RemoteAddr` cannot be parsed is rejected with 403, like any
  other rejection.
- A link-local client is matched by its address alone: the zone in its
  `RemoteAddr` (`fe80::1%eth0`) is ignored, since the lists cannot name one.

**Behind a reverse proxy the filter sees the proxy.** It reads `r.RemoteAddr`
and nothing else — deliberately, because `X-Forwarded-For` can be set by anyone
who can reach the port. The consequence is that behind nginx or a load balancer
the filter allowlists the proxy for every request and is effectively off. Filter
at the proxy in that setup.

## Logging

`WithLogging` writes one entry per request: method, path, status, response
size, duration. **The level follows the response status** — `Error` from 500,
`Warn` from 400, `Info` below — so the access log is visible at the level a
service actually runs at; `WithLogLevel` puts every entry at one level. The
status is the one the response went out with, not a 1xx sent ahead of it.
A service that is polled every few seconds can put
only the successful requests at `Debug` with `WithSuccessLevel`, and keep the
rejected ones visible as warnings; the first successful request of each
caller from each client is still logged at `Info` (`firstAccess=true`), so the
normal log shows who uses the service. Every entry names the client
(`request.client`) and, once `WithAuth` accepted the request, the caller
(`user`). **Bodies are not logged** unless `WithBodyLogging` asks
for it, and then only up to its byte limit and only for `application/json`,
`application/xml` and `text/*` — a body carries passwords and tokens, and a log
file is the wrong place for them.

The wrapper around `http.ResponseWriter` passes `Unwrap`, `Flush` and `Hijack`
through, so server-sent events and WebSocket upgrades keep working with the
middleware in the chain.

A `nil` logger falls back to `slog.Default()` instead of panicking on the first
request — but passing one explicitly is the point of the parameter.

### One logger for the whole chain

`WithLogging` puts its logger into the request context, and `WriteError` and
`WithIPFilter` read it back with `LoggerFrom`. Wrap with `WithLogging` outermost
and every entry this package writes goes to the application's logger; without
it they fall back to `slog.Default()`.

```go
handler := web.WithIPFilter(mux, allowed, blocked, web.WithIPFilterLogger(logger))
handler = web.WithLogging(handler, logger)   // outermost: seeds the context
```

The IP filter needs the extra option because it parses its lists while the
middleware is built, before there is a request whose context could carry
anything. Its rejections at request time need no option.

## Errors

`WriteError` sends `{"error":"…"}` and logs the details. From status 500 on, the
client gets `ErrInternal` and nothing else — a wrapped database or filesystem
error stays in the log, where the optional `reason` arguments land as well.
Below 500 the error text is passed on, since it describes the caller's own
mistake. The level follows the status: `Error` from 500, `Warn` below. Only the
path is logged, never the query string, which regularly carries tokens. The
entry goes to the logger in the request context, see
[One logger for the whole chain](#one-logger-for-the-whole-chain).

## Testing

```sh
go test -race ./web/
```

Coverage is 93.9 %, with `net/http/httptest` and no network. The error
branches of the atomic file replacement (a failing write, chmod or sync) are
not reached.

## Not addressed

Known and deliberately left alone:

- **No rate limiting.** Failed authentication shows only as a 401 with its
  `request.client` in the access log, and only where `WithLogging` is in the
  chain.
- **`web` does not watch the token file.** `ReadTokenFile` reads it when the
  service calls it — at start, and on `SIGHUP` in services built like
  `demo_app`, which rebuild everything on reload. Nothing notices a change on
  its own.
- **No lock around `AddTokenToFile` and `RemoveTokenFromFile`.** Two `token`
  commands at the same moment can lose one change: both read, both replace.
  It is a command an administrator runs by hand, one at a time.
- **`token add -dry-run` does not check the token file.** It only prints the
  line, also when the name is taken; pasted in by hand, that line stops the
  service at its next start with the duplicate named. The dry run is the way
  out when the token file cannot be written, so it does not depend on reading it.
- **A token file that is a symlink becomes a regular file** on `add` or
  `remove`: the rename replaces the link, and its old target is left as it
  was. Point the config at the real file instead.
- **`HashToken` is plain SHA-256, unsalted.** Right for the random 32-byte
  tokens `NewToken` makes, which cannot be enumerated; wrong for anything a
  person chose. Do not use it for passwords.
- **`Config` is not validated.** An empty `Config` silently rejects every
  request instead of reporting that the middleware was never configured.
- **`Decode` does not reject unknown fields.** Extra JSON keys are ignored
  rather than refused.
- **`X-Forwarded-For` is ignored**, see above.
- **No `Enabled` check before the work.** `WithLogging` builds its attributes
  and, with `WithBodyLogging`, buffers the bodies even when the handler would
  discard the entry. Since the entry is written at `Info` and above, that
  work is almost always used; body logging is opt-in and meant for debugging.
- **`logResponse` does not implement `io.ReaderFrom`.** With this middleware in
  the chain, `http.ServeFile` and `io.Copy` lose the `sendfile` path.
- **`Encode` does not log a marshal error.** It has no request to find a
  logger in; the client gets `ErrInternal`, the cause is lost. A value that
  cannot be marshalled is a programming error that shows in the first test.
- **The hijack error is not a sentinel.** `logResponse.Hijack` returns a plain
  `errors.New` when the underlying writer cannot be hijacked.
- **`WithCORS` sets `Allow-Methods`, `Allow-Headers` and `Max-Age` even when the
  origin is not on the list** and `Allow-Origin` is deliberately withheld.
- **Exported API that no service uses stays in v2.** An audit on 22.10.2026
  (the five services, tests included, and `demo_app`) found these unused
  outside `web`: the options `WithBodyLogging`, `WithLogLevel`,
  `WithAllowedOrigins`, `WithAllowedHeaders`; `Decode`, `NewApiError`;
  `ContextWithLogger`, `LoggerFrom`; and, used only by `TokenCommand`,
  `NewToken`, `ValidTokenName`, `RemoveTokenFromFile`, `ErrTokenExists`,
  `ErrTokenNotFound`, `ErrInvalidTokenName`, `ErrTokenFileWritable`.
  Removing or unexporting them breaks v2 and saves no code worth a new major
  version and five changed import paths; they are the candidates for a v3.
  `HashToken` and `AddTokenToFile` are not: the services' tests build their
  tokens and token files with them.
- **`HandlePreflight` stays while the services use it.** All but s0meter
  still register `OPTIONS /` with it, which behind `WithCORS` turns 404 into 405.
  Once they drop that route, it can go with the v3 candidates.
- **The success level defaults to `Info`, though every service sets
  `WithSuccessLevel(slog.LevelDebug)`.** Changing the default changes the log
  of any caller that relies on it; a v3 can make `Debug` the default and drop
  the option.
