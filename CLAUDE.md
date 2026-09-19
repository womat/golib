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
go test ./crypt/... -run TestSymCrypt -v     # single package / single test
```

`gpio/rpi` wraps `go-gpiocdev` and only compiles on Linux; on macOS `go vet ./...` /
`go build ./...` will fail there. Use `GOOS=linux go build ./gpio/rpi/` for a compile check
(vet still needs a Linux toolchain for the cgo-free uapi package, so errors from
`go-gpiocdev` internals on darwin are expected, not regressions).

Demos: `cd demo/<name>` first, then use its `Makefile`. All demos cross-compile for
Raspberry Pi and `make deploy` scp's the binary to `$(PI_USER)@$(PI_HOST)` — edit those
variables before deploying. `demo/demo_app` has the richest Makefile: `make help` lists
per-architecture targets (`build_arm6/7/8`, `build_arm64`, `build_linux64`,
`build_mac_arm64`, `build_windows64`), and `build_arm64_dev` additionally builds with
`-tags swagger`. `ensure_dev_certs` generates a self-signed dev cert into `app/certs/`.

Version tagging is done on the whole library at once (`git tag -a vX.Y.Z -m "..."; git push --tags`).

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

**Option pattern.** `encoder`, `decoder`, `rpi`, `rpiemu` and `mqtt` all use variadic
functional options (`Option func(*T)`, `WithX(...)`). New configurable constructors should
follow the same shape. Packages that log accept an injected `*slog.Logger` via
`WithLogger` rather than using the global logger.

**`web` is middleware, not a framework.** It provides composable `http.Handler` wrappers —
`WithAuth` (API key via `X-API-Key` or JWT, delegating to `jwt_util`), `WithCORS` /
`HandlePreflight`, `WithIPFilter` — plus generic `Encode[T]`/`Decode[T]` JSON helpers and
`WriteError`/`ApiError` for uniform JSON error bodies. Applications assemble a plain
`http.ServeMux` and wrap it; see `demo/demo_app/app/routes.go` for the canonical ordering
(CORS → IP filter → logging).

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
logs — note `crypt` ships a compiled-in default AES key, so production code must call
`SetKey`. `mqtt` is a thread-safe Paho wrapper with reconnect handling.

## Conventions

- Package-level doc comments carry a runnable `Example usage` block; keep them updated when
  an API changes — they are the primary documentation here.
- Sentinel errors are exported package vars (`ErrInvalidLevel`, `ErrUnauthorized`, …) and
  prefixed with the package name in their message.
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
