# golib

[![CI](https://github.com/womat/golib/actions/workflows/ci.yml/badge.svg)](https://github.com/womat/golib/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/womat/golib/v2.svg)](https://pkg.go.dev/github.com/womat/golib/v2)

`golib` is the shared groundwork of womat's small HTTPS services: HTTP
middleware with bearer-token authentication, logging, and a service template to
copy. Plain standard-library Go, no dependencies outside it.

    go get github.com/womat/golib/v2

Requires Go 1.27 or later. Builds on Linux, macOS and Windows; the services it
was written for run on a Raspberry Pi.

## Packages

| Package | Purpose |
|---|---|
| [`web`](web/README.md) | Composable `http.Handler` middleware: auth with hashed bearer tokens from a token file, the `token add/remove/list` subcommand, CORS, IP filter, access logging, JSON helpers |
| [`xlog`](xlog/README.md) | `log/slog` wrapper with destination and level selection |

What used to be here as well lives in modules of its own now:

| Former package | Now |
|---|---|
| `gpio`, `gpio/rpi`, `gpio/rpiemu`, `manchester/encoder`, `manchester/decoder`, `mqtt`, `keyvalue` | moved to modules of their own, with the same API |
| `crypt`, `jwt_util` | removed, no replacement (see [Changes in v2](#changes-in-v2)) |

Each package carries a doc comment with an `# Example usage` block, so
`go doc github.com/womat/golib/v2/<pkg>` is the fastest way in. **Read the
package README before putting it to work** — each one ends with a *Not
addressed* section naming what the package deliberately does not do.

## Getting started

A service reads its tokens, wraps a plain `http.ServeMux` and logs through
`xlog`:

```go
logger, err := xlog.Init("stdout", "info")
if err != nil {
    log.Fatal(err)
}
defer logger.Close()

tokens, err := web.ReadTokenFile("/opt/demo/etc/token-hashes")
if err != nil {
    log.Fatal(err)
}

mux := http.NewServeMux()
mux.Handle("GET /health", web.WithAuth(handleHealth(), web.Config{Tokens: tokens}))

// The last wrapper is the outermost one, so logging sees every request,
// rejected ones included.
handler := web.WithCORS(mux)
handler = web.WithIPFilter(handler, []string{"192.168.0.0/16"}, nil,
    web.WithIPFilterLogger(logger.Logger))
handler = web.WithLogging(handler, logger.Logger)
```

The tokens themselves are created with the service's own `token` subcommand,
which `web.TokenCommand` provides; see [`web/README.md`](web/README.md#tokens).
[`demo/demo_app`](demo/demo_app/README.md) is the complete version of this.

## Conventions

These hold across the library; the details are in each package's README.

**Functional options.** Configurable constructors take `Option func(*T)` values
built by `WithX(...)` functions — `web`'s CORS, IP filter and logging
middleware and `xlog` follow that shape. New ones should too. Options are applied at
construction; there are no setters afterwards.

**No global logger while serving.** `web.WithLogging` takes the logger as an
argument and hands it down the request context; nothing reaches for
`slog.Default()` on its own while it handles a request. The exception is
`WithIPFilter` while it is built: it reports unusable list entries to
`slog.Default()` unless `WithIPFilterLogger` names a logger.

**Every resource-owning type has a `Close`.** `xlog.LoggerWrapper` must be
closed by the caller, and `defer` right after construction is the intended
style.

**Sentinel errors are exported package vars** (`ErrTokenExists`,
`ErrTokenFileWritable`, …), compared with `errors.Is`. In `web` they carry no package
prefix, because those texts reach a client.

**No package changes its own module state.** The library has no `init`
functions that open files, start goroutines or read the environment.

## demo_app

[`demo/demo_app`](demo/demo_app/README.md) is a **template for new services**,
not just a sample: YAML configuration with defaults and environment expansion,
bearer tokens from a token file with a `token` subcommand, graceful shutdown and
config reload on `SIGHUP`, build metadata injected through `-ldflags`, and a
Swagger UI that is compiled in only with `-tags swagger`, so production binaries
carry no documentation endpoint. Copy it when starting a service in this
ecosystem — and read its README first; it is not operated anywhere, and its
development certificate is generated, not real.

It is a standalone Go module with a `replace` directive pointing at this working
copy. Build and deploy from inside its directory; its `Makefile`
cross-compiles for the Raspberry Pi and can scp the binary to a target host:

    cd demo/demo_app
    make help          # list all targets
    make build_arm64   # cross-compile for a 64-bit Pi
    make deploy        # build and copy to $(PI_USER)@$(PI_HOST)

The demos for GPIO and Manchester coding moved with their packages.

## Development

    go build ./...
    go vet ./...
    go test -race ./...

`demo/demo_app` is a separate module and is not covered by the above. It also
needs its development certificate generated first, because
`app/webservices.go` embeds it and `app/certs/` is deliberately not committed:

    cd demo/demo_app
    make ensure_dev_certs
    go build ./...

### Checking everything at once

`scripts/check.sh` runs `go fix`, `go vet`, `golangci-lint` and `govulncheck`
over both modules, generating `demo_app`'s certificate first:

    scripts/check.sh               # apply fixes, check everything
    scripts/check.sh --check       # report pending fixes, change nothing
    scripts/check.sh --skip=vuln   # leave out the step that needs a network
    scripts/check.sh --goos=darwin # check for another platform (default: linux)
    scripts/check.sh demo/demo_app

`golangci-lint` and `govulncheck` are not vendored; the script names the
`go install` line if one is missing. Linter behaviour is configured in
[`.golangci.yml`](.golangci.yml), where each exclusion carries the reason it
exists — `errcheck` against a deferred `Close` is excluded because this
library's own convention requires that `defer`.

### CI

`.github/workflows/ci.yml` runs on every push to `main` and on every pull
request, on Linux, the platform the services run on.

| Job | Covers |
|---|---|
| `format` | `gofmt` over the whole tree, `demo_app` included |
| `checks` | `scripts/check.sh --check`: pending `go fix` work, `golangci-lint`, `govulncheck` |
| `library` | `go vet` and `go test -race -cover`, then the `*AsRoot` tests under `sudo` |
| `cross` | the Raspberry Pi targets, Linux amd64 and 386, Windows, macOS |
| `demo_app` | certificate, vet, race tests, and the `swagger`-tagged build |
| `demo_app_cross` | one target per `build_*` recipe in its `Makefile` |

Every job takes its Go version from the `go.mod` of the module it builds, and
both modules declare the same one, so the version promised at the top of this
file is the version that is actually tested.

The actions are pinned to a commit SHA with the release in a comment, so a moved
tag cannot change what runs; `.github/dependabot.yml` proposes updates for them
and for the Go dependencies of both modules every week.

## Changes in v2

v2 changes the module path to `github.com/womat/golib/v2` and removes everything
v1 had marked `// Deprecated:`. v1 stays available at `v1.5.5`; it receives no
further changes.

To move a service from v1 to v2:

1. Change the imports from `github.com/womat/golib/<pkg>` to
   `github.com/womat/golib/v2/<pkg>` and run `go get github.com/womat/golib/v2`.
2. Replace the packages that moved out (`gpio`, `manchester`, `mqtt`,
   `keyvalue`) with the modules they moved to — the import paths change, the
   APIs do not. This README deliberately does not name those modules: lists of
   other repositories kept here went stale before (decided 23.10.2026).
3. `web.Config` has only `Tokens` left: delete `ApiKey`, `JwtSecret`, `JwtID`
   and `AppName`. Authentication is the token file — create the tokens with
   the service's `token add` subcommand (`web.TokenCommand`) and point the
   config at the file with `webserver.tokenFile`. Requests with `X-Api-Key` or
   a JWT get 401.
4. `WithCORS` no longer lists `X-Api-Key` in `Access-Control-Allow-Headers`;
   pass `WithAllowedHeaders` if a client still sends it.
5. `crypt` and `jwt_util` are gone without a replacement. Neither was in use;
   `crypt`'s compiled-in AES key made its `EncryptedString` protection against
   accidental disclosure only, and `jwt_util` existed for the JWT
   authentication `web` no longer has.

A further split of a package into its own module happens only when it needs a
version of its own.

### v2.1.0

**`web.ContextKey` was removed** although that breaks the API inside v2: any
code in the process could build `web.ContextKey("user")` and make `UserFrom`
report a caller `WithAuth` never authorized. None of the services that use this
library used the type. Read the caller with `web.UserFrom`.

New, and worth taking up when a service moves to v2.1.0:

- `web.CheckTokenFile` reports a token file the service could change itself;
  call it at start and log a failure as a warning.
- `web.ValidateIPList` lets the config check refuse an IP list entry that
  `WithIPFilter` would only log and skip.
- `WithCORS` answers preflights itself. Drop the `OPTIONS /` route: it turns
  the mux's 404 for an unknown path into 405.

Changed behaviour a service may notice:

- A token file with one hash under two names no longer loads; the access log
  could name the wrong caller.
- `token add` gives a new token file only the group of the config file, never
  its owner, and `AddTokenToFile` refuses a hash already in the file.
- `Encode` answers a value that cannot be marshalled with `ErrInternal`, not
  with the marshal error.
- `Decode` refuses a `}` or `]` after the JSON value.
- `WithIPFilter` matches a link-local client by its address; its zone
  (`fe80::1%eth0`) used to get it rejected.
- The access log reports the final status, not a 1xx sent before it, and body
  logging keeps the request body when reading it fails.

### v2.1.1

Changed behaviour a service may notice:

- `web.CheckTokenFile` checks every directory above the token file as well,
  up to the root. A layout that passed under v2.1.0 can now be reported — a
  directory above `etc` that belongs to the service user, or a service started
  from a checkout in a home directory; the message names the directory.
- `WithLogging` writes an access-log entry for a handler that panics, with
  status 500 at `Error`. The panic itself reaches `net/http` as before; the new
  line is not a new failure.
- `RemoveTokenFromFile` reports a name no token can have with
  `ErrInvalidTokenName`, not `ErrTokenNotFound`, and `token remove` says so.

## Tagging a new version

There is one branch, `main`; a version is an annotated tag on it, and tags apply
to the whole library at once. First fetch all tags and display them:

    git fetch --tags
    git tag -l

Then create a new tag:

    git tag -a vX.Y.Z -m "Release X.Y.Z"
    git push --tags

A fix for v1 would need a branch from tag `v1.5.5`; there is none, and none is
planned.

See the [Go module documentation](https://go.dev/doc/modules/managing-source)
for more information.

## License

See [LICENSE](LICENSE).
