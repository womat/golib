# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

`github.com/womat/golib` is a library of shareable Go packages (no `main` in the root module).
Everything under `demo/` is a **separate Go module** with its own `go.mod` and a
`replace github.com/womat/golib => ../..`, so demos always build against the local working copy.
Working directories matter: `go build ./...` at the root ignores the demos entirely.

## Commands

Root module (library):

```sh
go build ./...
go vet ./...
go test ./...
go test ./crypt/... -run TestAES -v     # single package / single test
```

`gpio/rpi` wraps `go-gpiocdev` and only compiles on Linux; on macOS `go vet ./...` /
`go build ./...` will fail there. Use `GOOS=linux go build ./gpio/rpi/` for a compile check
(vet still needs a Linux toolchain for the cgo-free uapi package, so errors from
`go-gpiocdev` internals on darwin are expected, not regressions).

Demos: `cd demo/<name>` first, then use its `Makefile`. All demos cross-compile for
Raspberry Pi and `make deploy` scp's the binary to `$(PI_USER)@$(PI_HOST)` — edit those
variables before deploying. `demo/demo_app` has the richest Makefile: `make help` lists
per-architecture targets (`build_arm6`, `build_arm7`, `build_arm64`, `build_linux64`,
`build_mac_arm64`, `build_windows64`), and `build_arm64_dev` additionally builds with
`-tags swagger`. `ensure_dev_certs` generates a self-signed dev cert into `app/certs/`.

Version tagging is done on the whole library at once (`git tag -a vX.Y.Z -m "..."; git push --tags`).

**This repository always authenticates as `womat`.** The machine's `gh` may have another
account active (`itd-mathe`, for instance) and the macOS keychain may hold its credential,
in which case a push either fails or would land under the wrong identity. `.git/config` is
not versioned, so a fresh clone needs this once:

```sh
git config --local --replace-all credential.helper ""
git config --local --add credential.helper \
  '!f() { test "$1" = get && printf "username=womat\npassword=%s\n" "$(gh auth token -u womat)"; }; f'
git config --local credential.username womat
```

The empty first value resets the inherited helper list for this repository, so the keychain
entry of another account cannot answer first; the helper then takes the token from
`gh auth token -u womat` without switching the globally active `gh` account. Verify with
`git push --dry-run origin develop`.

## Architecture

**Interface + backends.** `gpio` defines the `Pin` interface (levels, modes, pulls, edge
events) and contains no hardware code. Two implementations satisfy it: `gpio/rpi` (Linux
character device via go-gpiocdev) and `gpio/rpiemu` (in-memory emulator for tests and
development on non-Pi machines). Code that consumes GPIO should depend on `gpio.Pin`, never
on a backend, so the emulator can be substituted. Mode, pull resistor and debounce are
fixed at construction through each backend's options — the interface has no setters.

The emulator adds one method beyond the interface: `Drive(level)` simulates the outside
world changing the line and works on input pins, which `SetValue` deliberately refuses.
That is how receiving code (the Manchester decoder, for instance) is tested without
hardware; `rpiemu.NewPin` returns `rpiemu.Pin`, which embeds `gpio.Pin`.

Both backends share `gpio/internal/watch`: the channel a consumer ranges over, the edge
mask, drop counting and the shutdown that closes the channel. It exists because that logic
is where the concurrency bugs live, and `gpio/rpi` itself cannot be tested off a Pi —
keeping the delivery there down to a one-line call means the untestable part is a thin
adapter over code that is tested under `-race`.

**Manchester signal chain.** `manchester/encoder` and `manchester/decoder` are deliberately
decoupled from `gpio`: the encoder takes a `SetValue func(Level) error` callback, and the
decoder consumes a channel of its own `decoder.Event` values. The demos do the glue — a
goroutine translating `gpio.Event` into `decoder.Event` (see
`demo/manchester_listener/cmd/main.go`). Keep that boundary: don't import `gpio` from
`manchester/*`.

The decoder works in two phases — clock discovery (derives bit period from sampled edge
timings) followed by bit decoding with timing tolerance — and runs asynchronously; bits
arrive on `Bits()`, and `Close()` performs the ordered shutdown. Both packages support
IEEE 802.3 and Differential Manchester (Thomas) encodings, selected via `With...` options.

**Option pattern.** `encoder`, `decoder`, `rpi`, `rpiemu`, `mqtt`, `xlog` and `web`'s CORS
and logging middleware all use variadic functional options (`Option func(*T)`,
`WithX(...)`). New configurable constructors should follow the same shape. No package
reaches for `slog.Default()`: `mqtt` and `decoder` take a `*slog.Logger` through
`WithLogger`, `web.WithLogging` takes it positionally, and `encoder` has no logger at all
and reports transmission errors through `WithErrorHandler` (unset, they are dropped).

**`web` is middleware, not a framework.** It provides composable `http.Handler` wrappers —
`WithAuth` (API key via `X-Api-Key` or JWT, delegating to `jwt_util`), `WithCORS` /
`HandlePreflight`, `WithIPFilter`, `WithLogging` — plus generic `Encode[T]`/`Decode[T]`
JSON helpers and `WriteError`/`ApiError` for uniform JSON error bodies. Applications
assemble a plain `http.ServeMux` and wrap it; see `demo/demo_app/app/routes.go` for the
canonical wrapping order (`WithCORS`, then `WithIPFilter`, then `WithLogging` — the last
wrapper is the outermost, so logging sees rejected requests too). Note the CORS default is
`Access-Control-Allow-Origin: *`, kept deliberately so upgrades change no running service.

**`demo/demo_app` is the application template**, not just a sample. It shows the intended
service skeleton: YAML config with defaults + env expansion (`app/config.go`), an `App`
struct owning the wait group, HTTP server, context and restart/shutdown channels
(`app/app.go`), signal-driven graceful shutdown and config hot-reload, and version/build
metadata injected via `-ldflags`. Swagger is compiled in only under the `swagger` build tag
(`app/swagger.go` vs `app/swagger_stub.go`) so production builds carry no docs UI. When
starting a new service in this ecosystem, copy this structure.

**Support packages.** `xlog` wraps `slog` with destination (`stdout`/`stderr`/`null`/file)
and level selection, returning a wrapper whose `Close()` releases the file handle.
`keyvalue.Record` is a `map[string]any` with converting typed accessors. `crypt` provides
bcrypt hashing, AES-256 symmetric encryption, Ed25519 key-file generation, and
`EncryptedString`, which marshals as ciphertext so plaintext never lands in YAML/JSON or
logs. **`crypt` ships a compiled-in, public default AES key, and `EncryptedString` cannot
be moved off it** — `SetKey` is a method on `SymCrypt`, and it pads a short key with a
prefix of that same default. Treat `EncryptedString` as protection against accidental
disclosure only; `crypt/README.md` has the full security model, and the code is
deliberately frozen (see the *Not addressed* section there). `jwt_util` mints and validates
HS256 tokens and is what `web.WithAuth` delegates to. `mqtt` is a thread-safe Paho wrapper
with reconnect handling.

## Conventions

- Every package has a package-level doc comment with a runnable `# Example usage` section
  (a godoc heading, not a plain line), and is covered by a `README.md`. Both are primary
  documentation: keep them updated when an API changes. The doc comment lives in the
  package's main file (`<pkg>.go` — `web/web.go` exists for nothing else), never in a
  separate `doc.go`. Not every package has its own README: `gpio/README.md` covers `rpi`
  and `rpiemu`, `manchester/README.md` covers `encoder` and `decoder`. Every README ends
  with a *Not addressed* section listing what the package deliberately does not do; when
  you decide against fixing something, that is where the decision goes.
- Sentinel errors are exported package vars (`ErrInvalidLevel`, `ErrUnauthorized`, …),
  compared with `errors.Is`. `gpio` and `mqtt` prefix the message with the package name;
  `web`, `jwt_util` and `manchester/encoder` do not, because those texts can reach a
  client. Don't change an existing message — `signit`, `sqlite4router`, `tadl` and
  `s0meter` consume this library.
- Every resource-owning type (`Pin`, `Encoder`, `Decoder`, `LoggerWrapper`, mqtt `Handler`)
  has an explicit `Close()`/`Disconnect()` that callers must defer.
- Commit messages follow `type() description`, e.g. `fix() default port 8443`,
  `docu() update README.md`.

## Known state

`go vet ./...` and `go test ./...` are clean except for `gpio/rpi`, which cannot be built on
macOS because `go-gpiocdev` is Linux-only — see the note under Commands. `GOOS=linux go vet
./...` covers the whole repository including that package, and is worth running before
committing changes under `gpio/`.

`gpio/rpi` has no tests: exercising it needs a real GPIO chip. Its example is compiled but
deliberately carries no `Output:` comment, so `go test` does not try to run it.

**`demo/demo_app` does not build from a fresh clone.** `app/webservices.go` embeds
`certs/dev_cert.pem` and `certs/dev_key.pem`, and `app/certs/` is gitignored on purpose —
run `make ensure_dev_certs` (needs `openssl`) before `go build`. This is why the CI job for
that module generates the certificate as its first step.

**CI:** `.github/workflows/ci.yml`, on pushes to `main`/`develop` and on pull requests.
Six jobs: `format` (gofmt over the whole tree), `library` (vet plus `go test -race -cover`),
`cross` (the five Linux targets the library can be built for — Windows and macOS are
impossible because of `gpio/rpi`), `demos` (the four small modules), `demo_app`
(certificate, vet, race tests, `-tags swagger`) and `demo_app_cross` (one target per
`build_*` recipe in its Makefile). It runs on Linux precisely because the local checks
cannot be complete on macOS. When adding a package or a demo module, extend the matrices:
a new demo module is invisible to every existing job.

Every job takes its Go version from the `go.mod` of the module it builds, and **all six
modules declare the same `go 1.27`** — raised from 1.25.0/1.26 on 21.09.2026, deliberately
supporting one version rather than a range. Keep them in step; a module left behind silently
gets a different toolchain in CI. Of the consumers only `tadl` and `s0meter` import this
library (`signit` and `sqlite4router` carry their own `crypt` copies), and `tadl` still
declares `go 1.25.0` — it needs its own directive raised when it picks up the next tag.

**Dependencies:** `.github/dependabot.yml`, weekly, one `gomod` entry per module plus the
GitHub Actions themselves, with updates grouped (`golang.org/x/*`, the swagger generator,
everything else). It exists because by 21.09.2026 `golang.org/x/crypto` was nine minor
releases behind. When updating by hand, raise the direct requirements and let `go mod tidy`
settle the rest — `go get -u ./...` in `demo/demo_app` drags the whole swagger graph
forward, which affects only `docs/generate.sh` and has broken the tagged build before.
