# keyvalue

A `map[string]any` with converting typed accessors, for data whose shape is
only known at runtime — a decoded JSON document, a row from a database, a set
of measured values.

```go
r := keyvalue.NewRecord()
r.Set("power", "1234.5")
r.Set("enabled", 1)

r.Float64("power") // 1234.5 — parsed from the string
r.Int("power")     // 0      — Atoi does not accept "1234.5"
r.Bool("enabled")  // true
r.String("power")  // "1234.5"
```

Accessors never fail. A missing key, an unsupported type and an unconvertible
value all read as the zero value of the requested type. Use `Exists` or
`Value` where the difference matters:

```go
if v, ok := r.Value("power"); ok {
    // v is the raw any, no conversion applied
}
```

## Conversion rules

Only `bool`, `int`, `int64`, `float64` and `string` are understood as source
types. Everything else — including `int32`, `uint64` and `[]byte`, which a
database driver may well hand out — reads as the zero value.

Three rules are worth knowing before they surprise you:

**`Bool` is true for any non-zero number**, with `NaN` as the one exception —
"not a number" is not a truth value. Strings that name a truth value are
recognised as such; everything else is read as a number and follows the same
rule, so `"2"` and `2` cannot disagree.

| value | `Bool` |
|-------|--------|
| `1`, `2`, `-1`, `0.1`, `int64(7)` | `true` |
| `"1"`, `"2"`, `"-1"`, `"0.5"` | `true` |
| `"true"`, `"yes"`, `"on"` (any case) | `true` |
| `0`, `0.0`, `-0.0` | `false` |
| `math.NaN()`, `"NaN"` | `false` |
| `"false"`, `"no"`, `"off"`, `""`, `"a"` | `false` |

**`Int` is as wide as the platform's `int`.** On a 32-bit platform — which
includes the `GOARCH=arm` builds for the Raspberry Pi 1, Zero and the 32-bit
Pi 2/3/4 targets — a value that does not fit reads as `0`, not as a truncated
number. Use `Int64` where the range matters.

**Strings are parsed strictly per target type.** `Int("3.9")` is `0` because
`strconv.Atoi` rejects it, while `Float64("3.9")` is `3.9` and `Int` of the
float `3.9` is `3`. A float outside the `int64` range, an infinity or a NaN
reads as `0` rather than as whatever the platform's conversion happens to
produce.

## Creating a record

Use `NewRecord()` or a `Record{}` literal before writing. The zero value of
`Record` is a nil map: reading from it is harmless and yields zero values
throughout — `Int`, `Bool`, `Exists`, `Copy` and `GetSortedKeys` all work — but
`Set` panics with `assignment to entry in nil map`. It is the one place where
the package does not simply hand back a zero value.

```go
var r keyvalue.Record // nil map
r.Int("x")            // 0, fine
r.Set("x", 1)         // panic

r = keyvalue.NewRecord()
r.Set("x", 1)         // fine
```

## Concurrency

`Record` is a plain map and **not safe for concurrent use**. Reading from
several goroutines is fine as long as nobody writes; a concurrent `Set` is a
data race and aborts the process with `concurrent map writes`.

Guard it with a mutex, or hand out `Copy()`. The copy owns its own map, so it
can go to another goroutine — but the values are shared, so a map, slice or
pointer inside the record must not be modified from both sides.

## Testing

```sh
go test ./keyvalue/
```

The 32-bit behaviour is covered by `TestIntNarrowingOnSmallPlatforms`, which
adapts to `strconv.IntSize`. To exercise it for real:

```sh
GOOS=linux GOARCH=arm GOARM=7 go test -c -o /tmp/kv.test ./keyvalue/
docker run --rm --platform linux/arm/v7 -v /tmp/kv.test:/kv.test:ro alpine /kv.test
```

Full API: `go doc github.com/womat/golib/keyvalue`.
