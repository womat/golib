# golib

[![CI](https://github.com/womat/golib/actions/workflows/ci.yml/badge.svg)](https://github.com/womat/golib/actions/workflows/ci.yml)

`golib` contains all shareable Go packages of womat.

The library targets small Linux services and Raspberry Pi applications: GPIO
access, Manchester line coding, HTTP middleware, logging, MQTT, crypto and
configuration helpers. Everything is plain standard-library Go with a handful of
well-known dependencies.

    go get github.com/womat/golib

Requires Go 1.27 or later.

## Packages

| Package | Purpose |
|---|---|
| [`gpio`](gpio/README.md) | Backend-agnostic `Pin` interface: levels, modes, pull resistors, edge events |
| [`gpio/rpi`](gpio/README.md) | Linux implementation using the GPIO character device (`go-gpiocdev`) — **Linux only** |
| [`gpio/rpiemu`](gpio/README.md) | In-memory GPIO emulator for tests and development without hardware |
| [`manchester/encoder`](manchester/README.md) | Manchester encoder (IEEE 802.3 / Thomas), configurable bit order, sync bytes, async sending |
| [`manchester/decoder`](manchester/README.md) | Manchester decoder with automatic clock discovery and tolerance handling |
| [`web`](web/README.md) | Composable `http.Handler` middleware: auth, CORS, IP filter, logging, JSON helpers |
| [`jwt_util`](jwt_util/README.md) | Generation and validation of signed JWTs with issuer/subject/ID checks |
| [`crypt`](crypt/README.md) | bcrypt hashing, AES-256 symmetric encryption, Ed25519 key files, `EncryptedString` |
| [`xlog`](xlog/README.md) | `log/slog` wrapper with destination and level selection |
| [`mqtt`](mqtt/README.md) | Thread-safe Eclipse Paho client wrapper with reconnect handling |
| [`keyvalue`](keyvalue/README.md) | Generic `map[string]any` record with converting typed accessors |

Every package carries a doc comment with a runnable `# Example usage` block, so
`go doc github.com/womat/golib/<pkg>` is the fastest way to get started. Eight
READMEs cover all of them — `gpio/README.md` covers both GPIO backends and
`manchester/README.md` covers encoder and decoder together, because neither pair
is useful apart.

**Read the package README before putting a package to work.** Each one ends with
a *Not addressed* section naming what the package deliberately does not do, and
in the case of [`crypt`](crypt/README.md) that section is the difference between
a secret being protected and only looking protected.

## Getting started

Reading a GPIO pin shows the shape most of the library shares — an interface, a
backend chosen at construction, functional options, and an explicit `Close`:

```go
pin, err := rpi.NewPin(17,
    rpi.WithMode(gpio.Input),
    rpi.WithPullup(gpio.PullUp),
    rpi.WithDebounce(5*time.Millisecond),
)
if err != nil {
    log.Fatal(err)
}
defer pin.Close()

events, err := pin.WatchCh(gpio.RisingEdge | gpio.FallingEdge)
if err != nil {
    log.Fatal(err)
}

for evt := range events {
    fmt.Println(evt)
}
```

Swapping `rpi` for `rpiemu` runs the same code without a Raspberry Pi, which is
how the tests work. See [`gpio/README.md`](gpio/README.md).

## Conventions

These hold across the library; the details are in each package's README.

**Interface plus backends.** Where hardware or a transport is involved, the
abstraction and the implementation are separate packages. Depend on the
interface (`gpio.Pin`), never on a backend, so a test can substitute one.

**Functional options.** Configurable constructors take
`Option func(*T)` values built by `WithX(...)` functions — `encoder`, `decoder`,
`rpi`, `rpiemu`, `mqtt`, `web`'s CORS and logging middleware, and `xlog` all
follow that shape. New ones should too. Options are applied at construction;
there are no setters afterwards.

**No global logger.** A package that logs takes a `*slog.Logger` from the
caller: `mqtt` and `manchester/decoder` through a `WithLogger` option,
`web.WithLogging` as a positional argument. `manchester/encoder` has no logger
at all and reports transmission errors through `WithErrorHandler`. Nothing
reaches for `slog.Default()` on its own.

**Every resource-owning type has a `Close`.** `Pin`, `Encoder`, `Decoder`,
`LoggerWrapper` and mqtt's `Handler` (as `Disconnect`) must be closed by the
caller, and `defer` right after construction is the intended style.

**Sentinel errors are exported package vars.** `ErrInvalidLevel`,
`ErrUnauthorized`, `ErrNotConnected` and so on, compared with `errors.Is`. In
`gpio` and `mqtt` the message carries the package name as a prefix; in `web`,
`jwt_util` and `manchester/encoder` it does not, because those texts can reach a
client.

**No package changes its own module state.** The library has no `init`
functions that open files, start goroutines or read the environment.

## Demos

Each directory under `demo/` is a standalone Go module with a `replace`
directive pointing at this working copy, so demos always build against the local
library:

| Demo | Shows |
|---|---|
| `blinker` | Minimal GPIO output |
| `listener` | Watching GPIO edge events |
| `manchester_sender` | Manchester transmission over a GPIO pin |
| `manchester_listener` | Manchester reception and decoding |
| [`demo_app`](demo/demo_app/README.md) | Full service skeleton: config, HTTPS, auth, health, Swagger |

Build and deploy from inside a demo directory; every demo ships a `Makefile`
that cross-compiles for the Raspberry Pi and can scp the binary to a target
host:

    cd demo/demo_app
    make help          # list all targets
    make build_arm64   # cross-compile for a 64-bit Pi
    make deploy        # build and copy to $(PI_USER)@$(PI_HOST)

Adjust `PI_USER`, `PI_HOST` and `PI_PATH` in the `Makefile` before deploying.

`demo_app` is meant as a **template for new services**, not just a sample: YAML
configuration with defaults and environment expansion, graceful shutdown and
config reload on OS signals, build metadata injected through `-ldflags`, and a
Swagger UI that is compiled in only with `-tags swagger`, so production binaries
carry no documentation endpoint. Copy it when starting a service in this
ecosystem — and read [its README](demo/demo_app/README.md) first; it is not
operated anywhere, and its dev certificate is in the repository on purpose.

## Development

    go build ./...
    go vet ./...
    go test ./...
    go test ./crypt/... -run TestAES -v

`gpio/rpi` depends on the Linux GPIO character device and therefore does not
build on macOS or Windows; `go build ./...` and `go test ./...` fail there for
that package alone. Cover it with a cross-target check, which is worth running
before committing anything under `gpio/`:

    GOOS=linux go vet ./...

The demos are separate modules and are not covered by any of the above. Build
them from inside their own directory. `demo/demo_app` additionally needs its
development certificate generated first, because `app/webservices.go` embeds it
and `app/certs/` is deliberately not committed:

    cd demo/demo_app
    make ensure_dev_certs
    go build ./...

### Checking everything at once

`scripts/check.sh` runs `go fix`, `go vet`, `golangci-lint` and `govulncheck`
over all six modules:

    scripts/check.sh              # apply fixes, check everything
    scripts/check.sh --check      # report pending fixes, change nothing
    scripts/check.sh --skip=vuln  # leave out the step that needs a network
    scripts/check.sh demo/demo_app

It defaults to `GOOS=linux`, which is the only setting under which `gpio/rpi` is
checked at all — every one of these tools needs full type information, so on
macOS that one package otherwise aborts the whole run with errors from inside
`go-gpiocdev` that look like a broken tool. It also visits each module
separately, because a root-level `./...` never sees the demos, and generates
`demo_app`'s certificate first.

`golangci-lint` and `govulncheck` are not vendored; the script names the
`go install` line if one is missing.

### CI

`.github/workflows/ci.yml` runs on every push to `main` and `develop` and on
every pull request. It exists because the checks above cannot be complete on a
macOS machine: on a Linux runner `gpio/rpi` builds natively and is tested like
any other package.

| Job | Covers |
|---|---|
| `format` | `gofmt` over the whole tree, demos included |
| `library` | `go vet` and `go test -race -cover` |
| `cross` | the five Linux targets the library can be built for |
| `demos` | `go vet` and a build of each of the four small demo modules |
| `demo_app` | certificate, vet, race tests, and the `swagger`-tagged build |
| `demo_app_cross` | one target per `build_*` recipe in its `Makefile` |

Every job takes its Go version from the `go.mod` of the module it builds, and
all six modules declare the same one, so the version promised at the top of
this file is the version that is actually tested. There is no matrix over Go
versions: this library supports one.

## Tagging a new version

Tags apply to the whole library at once. First fetch all tags and display them:

    git fetch --tags
    git tag -l

Then create a new tag:

    git tag -a v1.0.6 -m "Release 1.0.6"
    git push --tags

See the [Go module documentation](https://go.dev/doc/modules/managing-source)
for more information.

## License

See [LICENSE](LICENSE).
