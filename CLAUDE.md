# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

`github.com/womat/golib/v2` is the shared groundwork of the HTTP services in this ecosystem:
`web` (middleware, bearer tokens from a token file, the `token` subcommand) and `xlog` (slog
setup). No `main` in the root module, no dependencies outside the standard library.

`demo/demo_app` is a **separate Go module** with its own `go.mod` and a
`replace github.com/womat/golib/v2 => ../..`, so it always builds against the local working copy.
`go build ./...` at the root ignores it.

**v2 since 16.10.2026.** v2 removed what v1 had deprecated: `crypt`, `jwt_util` (gone), and
`keyvalue`, `gpio`, `manchester`, `mqtt`, which live on as `github.com/womat/keyvalue`, `…/gpio`,
`…/manchester` and `…/mqtt` with their history. `web.Config` has only `Tokens`; API key,
`X-Api-Key`, JWT and `AppName` are gone. README.md, *Changes in v2*, is the migration guide.
v1 ends at tag `v1.5.5`; all five services import v2 since 16.10.2026. A v1
fix would need a branch from that tag — there is none.

## Commands

Root module (library):

```sh
go build ./...
go vet ./...
go test -race ./...
go test ./web/ -run TestWithAuth -v     # single package / single test
```

Everything builds on macOS, Linux and Windows. `scripts/check.sh` runs `go fix`, `go vet`,
`golangci-lint` and `govulncheck` over both modules and generates `demo_app`'s certificate first;
`--check` reports pending `go fix` work without writing, `--skip=vuln` drops the step that needs a
network, `--goos=linux` checks for the platform that ships (default: the host). Prefer it over
running the tools by hand — they do not see `demo/demo_app` from the root.

`demo/demo_app`: `cd` there first, then use its `Makefile`. It cross-compiles for the Raspberry
Pi and `make deploy` scp's the binary to `$(PI_USER)@$(PI_HOST)`. `make help` lists the
per-architecture targets (`build_arm6`, `build_arm7`, `build_arm64`, `build_linux64`,
`build_mac_arm64`, `build_windows64`); `build_arm64_dev` additionally builds with `-tags swagger`.
`ensure_dev_certs` generates a self-signed dev cert into `app/certs/`. Swagger docs are
regenerated with `docs/generate.sh` and the `swag` version pinned in its `go.mod`.

There is one branch, `main`: work is committed to it, and a version is an annotated tag on it, for
the whole library at once (`git tag -a vX.Y.Z -m "..."; git push --tags`). Use a short-lived
feature branch for work that must not land on `main` yet.

**This repository always authenticates as `womat`, over SSH:** the remote is
`git@github.com:womat/golib.git`, no host alias. On the development machine the default SSH key
for `github.com` belongs to another account (`itd-mathe`); the key for `womat` comes from the git
config of the checkout's directory (`core.sshCommand`, via `includeIf gitdir:`), which every
private repository shares. An HTTPS remote — what `gh repo clone` creates — authenticates through
the keychain with the other account and is refused; clone with
`git clone git@github.com:womat/golib.git`, or fix a fresh clone with
`git remote set-url origin git@github.com:womat/golib.git`.

Verify with `git push --dry-run origin main`.

## Architecture

**`web` is middleware, not a framework.** It provides composable `http.Handler` wrappers —
`WithAuth` (bearer tokens stored as SHA-256 hashes in a token file, read with `ReadTokenFile`;
`TokenCommand` is the `token add/remove/list` subcommand for a service's `main`), `WithCORS` /
`HandlePreflight`, `WithIPFilter`, `WithLogging` — plus generic `Encode[T]`/`Decode[T]` JSON
helpers and `WriteError`/`ApiError` for uniform JSON error bodies. Applications assemble a plain
`http.ServeMux` and wrap it; see `demo/demo_app/app/routes.go` for the canonical wrapping order
(`WithCORS`, then `WithIPFilter`, then `WithLogging` — the last wrapper is the outermost, so
logging sees rejected requests too). `WithAuth` records the token's name for `WithLogging`, which
writes it as `user=` into the access log and promotes the first request of each caller and
client to Info. The CORS default is `Access-Control-Allow-Origin: *`, kept deliberately so
upgrades change no running service.

**`xlog`** wraps `slog` with destination (`stdout`/`stderr`/`null`/file) and level selection,
returning a `LoggerWrapper` (embeds `*slog.Logger`) whose `Close()` releases the file handle.

**Option pattern.** `web`'s CORS, IP-filter and logging middleware and `xlog` use variadic
functional options (`Option func(*T)`, `WithX(...)`). New configurable constructors follow the
same shape. No package reaches for `slog.Default()` on its own: `web.WithLogging` takes the
logger positionally and hands it down the request context.

**`demo/demo_app` is the application template**, not just a sample. It shows the intended
service skeleton: YAML config with defaults and `${NAME}` env expansion (`app/config.go`), a
token file (`webserver.tokenFile`, relative to the config file) with the `demo_app token`
subcommand, an `App` struct owning the wait group, HTTP server, context and restart/shutdown
channels (`app/app.go`), signal-driven graceful shutdown and config reload on `SIGHUP` (which
rereads the token file), and version/build metadata injected via `-ldflags`. Swagger is compiled
in only under the `swagger` build tag (`app/swagger.go` vs `app/swagger_stub.go`).

Its lifecycle matches the five services (smartmeter's `cmd/main.go` and `app/app.go` are the
closest): `run` subscribes the signals once for the process and hands them to every `App`; a
`SIGHUP` is checked with `checkReload` before anything is torn down; a new config that fails to
start falls back to `lastGood`; a web server that stops on its own restarts the `App` via
`serverErr`; a failed `Run` releases what it acquired (`abort`). A config that does not load or
validate at start ends it with exit code 78 (`EX_CONFIG`), which the unit in its README keeps
systemd from restarting. When starting a new service in this ecosystem, copy this structure, and
keep it in step with the services when one of them changes it.

## Conventions

- Every package has a package-level doc comment with a runnable `# Example usage` section
  (a godoc heading, not a plain line), and is covered by a `README.md`. Both are primary
  documentation: keep them updated when an API changes. The doc comment lives in the
  package's main file (`<pkg>.go` — `web/web.go` exists for nothing else), never in a
  separate `doc.go`. Every README ends with a *Not addressed* section listing what the package
  deliberately does not do; when you decide against fixing something, that is where the
  decision goes.
- Sentinel errors are exported package vars (`ErrUnauthorized`, `ErrTokenExists`, …),
  compared with `errors.Is`. `web`'s carry no package prefix, because those texts can reach a
  client. Don't change an existing message — `tadl`, `s0meter`, `smartmeter`, `modbusgateway`
  and `relayhat` consume this library.
- Every resource-owning type (`xlog.LoggerWrapper`) has an explicit `Close()` that callers must
  defer.
- Commit subjects follow Conventional Commits, `type(scope): description` with an optional scope (`fix: default port 8443`, `feat(ui): …`), types `feat`, `fix`, `docu`, `chore`, `refactor`.

## Known state

`go vet ./...`, `go test ./...` and `scripts/check.sh` are clean on macOS and Linux.

**`demo/demo_app` does not build from a fresh clone.** `app/webservices.go` embeds
`certs/dev_cert.pem` and `certs/dev_key.pem`, and `app/certs/` is gitignored on purpose —
run `make ensure_dev_certs` (needs `openssl`) before `go build`. This is why the CI job for
that module generates the certificate as its first step.

Linter configuration is `.golangci.yml` at the root — `demo_app` finds it by walking up, so there
is one file. The default linter set is kept and every exclusion states its reason: `errcheck`
ignores `xlog.LoggerWrapper.Close`, because this repository's own convention is that callers
`defer` it and a deferred call has nowhere to return an error to; test files are exempt from
`errcheck` entirely. **Do not widen these exclusions to make a new finding go away; fix the
finding or record the decision.**

**CI:** `.github/workflows/ci.yml`, on pushes to `main` and on pull requests, on Linux. Actions
are pinned to a commit SHA with the release in a comment — never a movable tag like `@v7`;
dependabot updates the pins. Six jobs: `format` (gofmt over the whole tree), `checks`
(`scripts/check.sh --check`, so the CI and the local command cannot drift apart), `library` (vet
plus `go test -race -cover`), `cross` (Pi targets, Linux amd64/386, Windows, macOS), `demo_app`
(certificate, vet, race tests, `-tags swagger`) and `demo_app_cross` (one target per `build_*`
recipe in its Makefile). When adding a demo module, extend the matrices: a new module is
invisible to every existing job.

Every job takes its Go version from the `go.mod` of the module it builds, and **both modules
declare the same `go 1.27`**, deliberately supporting one version rather than a range. Keep them
in step. All five consumers (`tadl`, `s0meter`, `smartmeter`, `modbusgateway`, `relayhat`) and
the split-out modules (`gpio`, `manchester`, `mqtt`, `keyvalue`) declare `go 1.27` as well.

**Dependencies:** `.github/dependabot.yml`, weekly, one `gomod` entry per module plus the
GitHub Actions themselves, with updates grouped (`golang.org/x/*`, the swagger generator,
everything else). The root module has no dependencies; `demo_app` has yaml and swagger. When
updating by hand, raise the direct requirements and let `go mod tidy` settle the rest —
`go get -u ./...` in `demo/demo_app` drags the whole swagger graph forward, which affects only
`docs/generate.sh` and has broken the tagged build before.
