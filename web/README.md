# web

Composable `http.Handler` middleware for the services in this ecosystem, plus
the JSON helpers they all repeat: API-key and JWT authentication, CORS, an IP
filter, request logging, and one uniform error body.

It is middleware, not a framework. Applications assemble a plain
`http.ServeMux` and wrap it; `demo/demo_app/app/routes.go` is the canonical
example. The [ordering](#ordering) matters, and the CORS default is `*` — read
both sections before putting this on a public interface.

## API

| Function | Comment |
|---|---|
| `func WithAuth(h http.Handler, config Config) http.Handler` | API key via `X-Api-Key`, or JWT via `Authorization: Bearer`; 401 otherwise |
| `type Config struct{ ApiKey, JwtSecret, JwtID, AppName string }` | empty fields disable that mechanism — an empty `Config` rejects everything |
| `func WithCORS(next http.Handler, opts ...CORSOption) http.Handler` | adds the CORS headers; **does not** answer preflights |
| `func WithAllowedOrigins(origins ...string) CORSOption` | restrict to a list, echo only on a match, set `Vary: Origin` |
| `func WithAllowedMethods(methods ...string) CORSOption` | replace the method list |
| `func WithAllowedHeaders(headers ...string) CORSOption` | replace the request-header list |
| `func HandlePreflight() http.Handler` | answers `OPTIONS` with 204; register it as its own route |
| `func WithIPFilter(h http.Handler, allowedIPs, blockedIPs []string) http.Handler` | IP and CIDR allow/block lists, parsed once; 403 on rejection |
| `func WithLogging(h http.Handler, logger *slog.Logger, opts ...LogOption) http.Handler` | one debug entry per request: method, path, status, size, duration |
| `func WithBodyLogging(maxBytes int) LogOption` | additionally log request and response body, truncated, textual types only |
| `func Encode[T any](w http.ResponseWriter, status int, v T)` | JSON response; a marshal failure still yields valid JSON and a 500 |
| `func Decode[T any](w http.ResponseWriter, r *http.Request) (T, error)` | JSON request body, limited to 1 MiB, trailing data refused |
| `func WriteError(w, r, status int, err error, reason ...error)` | uniform error body, logged with its reasons |
| `func NewApiError(err error) ApiError` / `type ApiError struct{ Error string }` | the JSON error shape: `{"error":"…"}` |

Sentinel errors: `ErrUnauthorized`, `ErrForbidden`, `ErrInternal`.

## Ordering

```go
mux := http.NewServeMux()
mux.Handle("OPTIONS /", web.HandlePreflight())          // before any auth
mux.Handle("GET /version", app.HandleVersion())         // public
mux.Handle("GET /health", web.WithAuth(app.HandleHealth(), webCfg))

handler := web.WithCORS(mux)
handler = web.WithIPFilter(handler, allowedIPs, blockedIPs)
handler = web.WithLogging(handler, logger)
```

Two traps are worth naming:

**Preflight must not meet `WithAuth`.** A browser sends the `OPTIONS` preflight
*without* credentials. Registered behind the auth middleware it earns a 401, and
every cross-origin call fails with a message that says nothing about the real
cause. Register it as its own route, as above.

**`WithCORS` does not answer `OPTIONS`.** It only sets headers and passes the
request on — that is why `HandlePreflight` exists as a separate handler.

## CORS

Without options, `WithCORS` answers every origin with `Access-Control-Allow-Origin: *`
and advertises `Authorization` and `X-Api-Key` as allowed request headers. That
is the historical behaviour and it is kept as the default so that upgrading the
library changes no running service.

It is workable for an API that is guarded by an API key and an IP filter, and
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

**Behind a reverse proxy the filter sees the proxy.** It reads `r.RemoteAddr`
and nothing else — deliberately, because `X-Forwarded-For` can be set by anyone
who can reach the port. The consequence is that behind nginx or a load balancer
the filter allowlists the proxy for every request and is effectively off. Filter
at the proxy in that setup.

## Logging

`WithLogging` writes one `Debug` entry per request: method, path, status,
response size, duration. **Bodies are not logged** unless `WithBodyLogging` asks
for it, and then only up to its byte limit and only for `application/json`,
`application/xml` and `text/*` — a body carries passwords and tokens, and a log
file is the wrong place for them.

The wrapper around `http.ResponseWriter` passes `Unwrap`, `Flush` and `Hijack`
through, so server-sent events and WebSocket upgrades keep working with the
middleware in the chain.

## Errors

`WriteError` sends `{"error":"…"}` and logs the details. From status 500 on, the
client gets `ErrInternal` and nothing else — a wrapped database or filesystem
error stays in the log, where the optional `reason` arguments land as well.
Below 500 the error text is passed on, since it describes the caller's own
mistake. The level follows the status: `Error` from 500, `Warn` below. Only the
path is logged, never the query string, which regularly carries tokens.

## Testing

```sh
go test -race ./web/
```

Coverage is 95.9 %, with `net/http/httptest` and no network.

## Not addressed

Known and deliberately left alone:

- **The length of the API key stays observable.** The comparison is constant
  time (`crypto/subtle`), but a key of a different length is still rejected
  faster than a wrong one of the same length.
- **No rate limiting and no logging of failed authentication.** A brute-force
  attempt against the API key is invisible, while the IP filter does log its
  rejections.
- **The authenticated user cannot be read.** `WithAuth` puts it into the request
  context under an unexported key and there is no exported getter, so the
  context plumbing is unusable from outside the package.
- **`Config` is not validated.** An empty `Config` silently rejects every
  request instead of reporting that the middleware was never configured.
- **`Decode` does not reject unknown fields.** Extra JSON keys are ignored
  rather than refused.
- **`X-Forwarded-For` is ignored**, see above.
