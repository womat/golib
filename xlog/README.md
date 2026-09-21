# xlog

A thin wrapper around `log/slog`: pick an output destination and a level from
two strings, get a `*slog.Logger` back, and close it again when the service
shuts down.

That is the whole point. Every service in this ecosystem reads its log
destination and level from a config file, and none of them should repeat the
`os.OpenFile`, the handler construction and the file handle's lifetime.

## API

| Function | Comment |
|---|---|
| `func Init(dest, logLevel string, opts ...Option) (*LoggerWrapper, error)` | opens the destination and builds the handler; the error is the one from opening the file |
| `func WithSource(enabled bool) Option` | force the source file and line on or off |
| `type LoggerWrapper struct{ *slog.Logger; … }` | embeds the logger, so it *is* a `*slog.Logger` |
| `func (l *LoggerWrapper) Close() error` | releases the file handle; no-op for the three named destinations |

```go
logger, err := xlog.Init(cfg.LogDestination, cfg.LogLevel)
if err != nil {
    panic(err) // before the logger exists there is nowhere to report this
}
defer logger.Close()

slog.SetDefault(logger.Logger) // optional, but what the services do
```

## Destinations and levels

| `dest` | Output |
|---|---|
| `stdout` | `os.Stdout` |
| `stderr` | `os.Stderr` |
| `null` | discarded |
| anything else | treated as a file path, created if missing, appended to otherwise |

| `logLevel` | Level |
|---|---|
| `debug` | `slog.LevelDebug` |
| `info` | `slog.LevelInfo` |
| `warning`, `warn` | `slog.LevelWarn` |
| `error` | `slog.LevelError` |
| anything else | `slog.LevelInfo` |

Both arguments are matched case-insensitively and with surrounding blanks
trimmed, so a stray `"Stdout "` from a hand-edited YAML file writes to stdout
rather than creating a file named `Stdout `. An unrecognised *level* falls back
to `info` silently — there is no way to tell a typo from an explicit `info`.
An unrecognised *destination* cannot fall back, because every string is a valid
file name; that is why `logDestination` is worth validating in the application,
as `demo/demo_app/app/config.go` does.

The format is always `slog`'s text handler. There is no JSON option.

## Source info

Without `WithSource`, the source file and line are added for the `debug` level
and omitted for every other. That coupling is convenient and occasionally
wrong — a service logging at `info` in production may still want to know which
line produced an entry. `WithSource(true)` or `WithSource(false)` decides it
explicitly and overrides the level-based default.

## Close

`Close` releases the file handle and, in the same locked step, points the
handler's writer at `io.Discard`.

That second half matters. `slog` discards write errors from its handler, so a
logger left pointing at a closed file accepts every call and loses every line
without a word. Closing the destination and leaving the logger usable-but-silent
is the honest behaviour: after `Close` the logger still works, it just writes
nowhere.

`Close` is safe to call more than once and from several goroutines, and it is a
no-op for `stdout`, `stderr` and `null`. The writer carries a mutex, so writes
are serialised against each other and against `Close`.

## Testing

```sh
go test -race ./xlog/
```

Coverage is 100 % of statements. The file destination is exercised in a
temporary directory — an earlier version of the example test did not, which is
how `xlog/app.log` once ended up committed to the repository.

## Not addressed

Known and deliberately left alone:

- **No log rotation.** The file is opened in append mode and grows without
  bound. Rotate it outside the process (`logrotate`, systemd) and restart or
  `SIGHUP` the service, which is what the config reload in `demo_app` is for.
- **No JSON handler.** Text only.
- **No level change at runtime.** The level is fixed by `Init`; changing it
  means building a new logger, which in `demo_app` happens on config reload.
- **`Init` does not set the default logger.** Callers that want
  `slog.Info(...)` to work package-wide must call `slog.SetDefault` themselves.

Full API: `go doc github.com/womat/golib/xlog`.
