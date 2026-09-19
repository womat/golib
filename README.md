# golib

`golib` contains all shareable Go packages of womat.

The library targets small Linux services and Raspberry Pi applications: GPIO access,
Manchester line coding, HTTP middleware, logging, MQTT, crypto and configuration helpers.
Everything is plain standard-library Go with a handful of well-known dependencies.

    go get github.com/womat/golib

Requires Go 1.25 or later.

## Packages

| Package              | Purpose                                                                                     |
|----------------------|---------------------------------------------------------------------------------------------|
| `gpio`               | Backend-agnostic `Pin` interface: levels, modes, pull resistors, edge events                  |
| `gpio/rpi`           | Linux implementation using the GPIO character device (`go-gpiocdev`)                          |
| `gpio/rpiemu`        | In-memory GPIO emulator for tests and development without hardware                            |
| `manchester/encoder` | Manchester encoder (IEEE 802.3 / Thomas), configurable bit order, sync bytes, async sending   |
| `manchester/decoder` | Manchester decoder with automatic clock discovery and tolerance handling                      |
| `web`                | Composable `http.Handler` middleware: auth, CORS, IP filter, logging, JSON helpers            |
| `jwt_util`           | Generation and validation of signed JWTs with issuer/subject/ID checks                        |
| `crypt`              | bcrypt hashing, AES-256 symmetric encryption, Ed25519 key files, `EncryptedString`            |
| `xlog`               | `log/slog` wrapper with destination and level selection                                       |
| `mqtt`               | Thread-safe Eclipse Paho client wrapper with reconnect handling                               |
| `keyvalue`           | Generic `map[string]any` record with converting typed accessors                               |

Every package carries a doc comment with a runnable usage example — `go doc github.com/womat/golib/<pkg>`
is the fastest way to get started.

## GPIO

`gpio` defines the interface only; it never touches hardware. Depend on `gpio.Pin` in your
own code and pick a backend at construction time — `rpi` on a Raspberry Pi, `rpiemu`
everywhere else.

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

`rpiemu.NewPin` accepts the same options, so tests can drive the exact same code path
without a Pi. The emulator additionally offers `Drive`, which simulates the outside world
changing the line and, unlike `SetValue`, works on input pins:

```go
pin, _ := rpiemu.NewPin(17, rpiemu.WithMode(gpio.Input))
events, _ := pin.WatchCh(gpio.RisingEdge | gpio.FallingEdge)

pin.Drive(gpio.High) // delivers a rising edge to the watcher
```

Note that `gpio/rpi` only builds on Linux.

## Manchester encoding

Encoder and decoder are independent of `gpio`: the encoder writes through a
`func(Level) error` callback, the decoder reads its own `Event` values from a channel.
The application provides the glue, which keeps the codec usable over any transport.

Sending:

```go
enc, err := encoder.New(50,
    func(level encoder.Level) error { return pin.SetValue(gpio.Level(level)) },
    encoder.WithBitOrder(encoder.LSBFirst),
    encoder.WithSyncBytes(2),
)
if err != nil {
    log.Fatal(err)
}
defer enc.Close()

if _, err := enc.Send([]byte("Hello World")); err != nil {
    log.Fatal(err)
}
enc.Wait()
```

Receiving:

```go
dec, err := decoder.New(events, 50, decoder.WithManchesterEncoding(decoder.IEEE))
if err != nil {
    log.Fatal(err)
}
defer dec.Close()

for bit := range dec.Bits() {
    // bit is decoder.High, decoder.Low or decoder.Invalid
}
```

The decoder starts in a clock-discovery phase that derives the bit period from the timing
of incoming edges, then decodes bits with a timing tolerance. Both packages support
IEEE 802.3 and Differential Manchester (Thomas).

## Web services

`web` is middleware, not a framework. Build a plain `http.ServeMux` and wrap it:

```go
mux := http.NewServeMux()
mux.Handle("OPTIONS /", web.HandlePreflight())
mux.Handle("GET /version", app.HandleVersion())
mux.Handle("GET /health", web.WithAuth(app.HandleHealth(), cfg))

handler := web.WithCORS(mux)
handler = web.WithIPFilter(handler, cfg.AllowedIPs, cfg.BlockedIPs)
```

`WithAuth` accepts either an API key in the `X-API-Key` header or a JWT issued by
`jwt_util`. `Encode[T]` / `Decode[T]` handle JSON bodies, and `WriteError` produces a
uniform JSON error response while logging the details.

## Logging

```go
logger, err := xlog.Init("stdout", "debug") // "stdout", "stderr", "null" or a file path
if err != nil {
    panic(err)
}
defer logger.Close()
```

Packages that log accept an injected `*slog.Logger` through a `WithLogger` option instead
of reaching for the global logger.

## Crypto

`crypt.Hash` / `crypt.Compare` wrap bcrypt. `crypt.EncryptedString` marshals as ciphertext
via the `TextMarshaler` and `BinaryMarshaler` interfaces, so encrypted secrets can live in
YAML or JSON configuration without ever being written back in clear text.

The symmetric encryption ships with a compiled-in default AES key for convenience — call
`SetKey` with your own 256-bit key in production.

## Demos

Each directory under `demo/` is a standalone Go module with a `replace` directive pointing
at this working copy, so demos always build against the local library:

| Demo                  | Shows                                                       |
|-----------------------|-------------------------------------------------------------|
| `blinker`             | Minimal GPIO output                                         |
| `listener`            | Watching GPIO edge events                                   |
| `manchester_sender`   | Manchester transmission over a GPIO pin                     |
| `manchester_listener` | Manchester reception and decoding                           |
| `demo_app`            | Full service skeleton: config, HTTPS, auth, health, Swagger |

Build and deploy from inside a demo directory; every demo ships a `Makefile` that
cross-compiles for the Raspberry Pi and can scp the binary to a target host:

    cd demo/demo_app
    make help          # list all targets
    make build_arm64   # cross-compile for a 64-bit Pi
    make deploy        # build and copy to $(PI_USER)@$(PI_HOST)

Adjust `PI_USER`, `PI_HOST` and `PI_PATH` in the `Makefile` before deploying.

`demo_app` is meant as a template for new services: YAML configuration with defaults and
environment expansion, graceful shutdown and config reload on OS signals, build metadata
injected through `-ldflags`, and a Swagger UI that is compiled in only with
`-tags swagger`, so production binaries carry no documentation endpoint.

## Development

    go build ./...
    go vet ./...
    go test ./...
    go test ./crypt/... -run TestSymCrypt -v

`gpio/rpi` depends on the Linux GPIO character device and therefore does not build on
macOS or Windows; use `GOOS=linux go build ./gpio/rpi/` for a compile check.

## Tagging a new version

First fetch all tags and display them:

    git fetch --tags
    git tag -l

Then create a new tag for the whole library:

    git tag -a v1.0.6 -m "Release 1.0.6"
    git push --tags

See the [Go module documentation](https://go.dev/doc/modules/managing-source) for more information.

## License

See [LICENSE](LICENSE).
