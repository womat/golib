// Package keyvalue provides a generic key-value record type with converting accessors.
// Values can be retrieved as bool, int, int64, float64, or string, and are converted
// as needed. Nothing is checked at compile time: an accessor that cannot deliver what
// was asked for returns the zero value rather than an error.
// For raw access without conversion, use the Value method.
//
// # Example usage
//
//	func main() {
//	    r := keyvalue.NewRecord()
//	    r.Set("port", 8443)
//	    r.Set("debug", "yes")
//	    r.Set("factor", "1.5")
//
//	    port := r.Int("port")        // 8443
//	    debug := r.Bool("debug")     // true
//	    factor := r.Float64("factor") // 1.5
//	    missing := r.String("host")  // "" - no error, no panic
//
//	    if !r.Exists("host") {
//	        // tell a missing key from a zero value
//	    }
//
//	    for _, k := range r.GetSortedKeys() {
//	        fmt.Println(k, r.Value(k))
//	    }
//	    _, _, _, _ = port, debug, factor, missing
//	}
//
// # Conversion
//
// Only bool, int, int64, float64 and string are understood as source types.
// Every other type - the remaining integer widths a database driver may hand
// out among them - reads as the zero value of the requested type, exactly like
// a missing key. The accessors never report an error, so use Exists or Value
// where the difference matters.
//
// Three rules are easy to get wrong:
//
//   - Bool treats any non-zero number as true, NaN excepted. Strings that name
//     a truth value ("true", "yes", "on" and their negatives) are recognised as
//     such, anything else is read as a number and follows the same rule.
//   - Int returns the platform's int. On a 32-bit platform, which includes the
//     GOARCH=arm builds for the Raspberry Pi, a value that does not fit reads
//     as 0 rather than as a truncated number. Use Int64 where the range
//     matters.
//   - Float64 also converts from bool, and an int64 beyond 2^53 does not
//     survive the conversion: it returns the nearest representable value and
//     therefore disagrees with Int64 on the same key.
//
// # Concurrency
//
// Record is a plain map and is not safe for concurrent use. Reading from
// several goroutines is fine as long as nobody writes; a concurrent Set is a
// data race and will abort the process with "concurrent map writes". Guard it
// with a mutex, or hand out copies made with Copy.
package keyvalue

import (
	"maps"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Record is a generic key-value map.
type Record map[string]any

// NewRecord creates a new empty Record.
//
// Use it, or a Record{} literal, before writing: the zero value of Record is a
// nil map. Reading from one is harmless and yields zero values throughout, but
// Set panics on it - the one place where this package does not simply hand
// back a zero value.
func NewRecord() Record {
	return make(Record)
}

// Exists reports whether key exists in the record.
func (r Record) Exists(key string) bool {
	_, ok := r[key]
	return ok
}

// Value returns the raw value for key and whether the key exists.
// Use this for special cases where type conversion is not desired, or to tell
// a missing key from one holding a zero value.
func (r Record) Value(key string) (any, bool) {
	v, ok := r[key]
	return v, ok
}

// Set sets the value for key.
//
// It panics if the Record is the nil zero value; see NewRecord.
func (r Record) Set(key string, value any) {
	r[key] = value
}

// Bool returns the bool value for key.
// Converts from string, int, int64, and float64 if necessary.
// Returns false if not found or not convertible.
//
// A number is true when it is non-zero, so 2 and -1 are true and 0 is false.
// NaN is the exception and reads as false: it is not a truth value.
//
// For strings, "true", "yes", "on" and "false", "no", "off" are recognised
// regardless of case. Anything else is parsed as a number and follows the same
// rule, so "2" is true and "0" is false.
func (r Record) Bool(key string) bool {
	switch i := r[key].(type) {
	case bool:
		return i
	case int:
		return i != 0
	case int64:
		return i != 0
	case float64:
		return isTrue(i)
	case string:
		switch strings.ToLower(i) {
		case "true", "yes", "1", "on":
			return true
		case "false", "no", "0", "off":
			return false
		}

		// ParseFloat accepts "NaN" and the infinities, so the result goes
		// through the same check as a float value.
		v, err := strconv.ParseFloat(i, 64)
		return err == nil && isTrue(v)
	}

	return false
}

// isTrue reports whether f counts as true: non-zero, but not NaN.
func isTrue(f float64) bool {
	return !math.IsNaN(f) && f != 0
}

// Float64 returns the float64 value for key.
// Converts from string, int, int64, and bool if necessary.
// Returns 0.0 if not found or not convertible.
//
// An integer beyond 2^53 cannot be represented exactly and is rounded, so
// Float64 and Int64 of the same entry may disagree. Use Int64 where the exact
// value matters.
func (r Record) Float64(key string) float64 {
	switch i := r[key].(type) {
	case float64:
		return i
	case int64:
		return float64(i)
	case int:
		return float64(i)
	case string:
		if v, err := strconv.ParseFloat(i, 64); err == nil {
			return v
		}
	case bool:
		if i {
			return 1.0
		}
	}
	return 0.0
}

// Int returns the int value for key.
// Converts from string, int64, float64, and bool if necessary.
// Returns 0 if not found or not convertible.
//
// A value outside the range of int counts as not convertible. On a 32-bit
// platform - which includes the GOARCH=arm builds for the Raspberry Pi - that
// range is considerably smaller than the one of Int64.
//
// A string is parsed as an integer, not as a number: "3.7" is not convertible
// and yields 0, while the float64 3.7 is truncated to 3. The same value can
// therefore read differently depending on whether it arrived as text or as a
// number - relevant for YAML and JSON, where quoting decides.
func (r Record) Int(key string) int {
	switch i := r[key].(type) {
	case float64:
		return narrowToInt(floatToInt64(i))
	case int64:
		return narrowToInt(i)
	case int:
		return i
	case string:
		if v, err := strconv.Atoi(i); err == nil {
			return v
		}
	case bool:
		if i {
			return 1
		}
	}
	return 0
}

// Int64 returns the int64 value for key.
// Converts from string, int, float64, and bool if necessary.
// Returns 0 if not found or not convertible, which includes a float outside
// the range of int64, an infinity, and NaN.
//
// As with Int, a string is parsed as an integer: "3.7" yields 0, the float64
// 3.7 yields 3.
func (r Record) Int64(key string) int64 {
	switch i := r[key].(type) {
	case float64:
		return floatToInt64(i)
	case int64:
		return i
	case int:
		return int64(i)
	case string:
		if v, err := strconv.ParseInt(i, 10, 64); err == nil {
			return v
		}
	case bool:
		if i {
			return 1
		}
	}
	return 0
}

// String returns the string value for key.
// Converts from bool, int, int64, and float64 if necessary.
// Returns "" if not found or not convertible.
//
// A float is formatted without an exponent, so a very large or very small
// value becomes a long run of digits: 1e-30 reads as
// "0.000000000000000000000000000001". The result parses back to the same
// float, but it is not meant for display.
func (r Record) String(key string) string {
	switch v := r[key].(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// Copy returns a shallow copy of the record.
//
// The map itself is independent of the original, so the copy can be handed to
// another goroutine. The values are not: a map, slice or pointer stays shared
// with the original and is not safe to modify from both sides.
func (r Record) Copy() Record {
	record := make(Record, len(r))
	maps.Copy(record, r)
	return record
}

// GetSortedKeys returns the sorted keys as a slice
func (r Record) GetSortedKeys() []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// floatToInt64 converts f to an int64, truncating towards zero.
//
// It returns 0 for NaN and for anything outside the int64 range. Go leaves
// such a conversion implementation defined - on arm64 it saturates to
// MaxInt64, on other platforms the result may differ - and a silently wrong
// number is worse than the zero the accessors document for a value that
// cannot be converted.
func floatToInt64(f float64) int64 {
	const (
		// math.MaxInt64 is not exactly representable as a float64; the
		// conversion rounds up to 2^63, so the comparison has to exclude it.
		upperBound = float64(math.MaxInt64) // 2^63
		lowerBound = float64(math.MinInt64) // -2^63, exactly representable
	)

	if math.IsNaN(f) || f >= upperBound || f < lowerBound {
		return 0
	}

	return int64(f)
}

// narrowToInt converts v to an int, returning 0 if it does not fit.
//
// On a 64-bit platform this never rejects anything; on a 32-bit one it is what
// keeps a truncated value from being mistaken for a real one.
func narrowToInt(v int64) int {
	if int64(int(v)) != v {
		return 0
	}

	return int(v)
}
