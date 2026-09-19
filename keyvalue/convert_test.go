package keyvalue

import (
	"math"
	"strconv"
	"testing"
)

// TestIntConversionOutOfRange pins down what happens to a value that does not
// fit the target type.
//
// Go leaves an out-of-range float-to-integer conversion implementation
// defined: on arm64 it saturates to MaxInt64, elsewhere it may produce
// something else entirely. The accessors document "returns 0 if not found or
// not convertible", and a value outside the range is exactly that.
func TestIntConversionOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"float above the int64 range", 1e30},
		{"float below the int64 range", -1e30},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		{"not a number", math.NaN()},
		{"string above the int64 range", "9223372036854775808"},
		{"string below the int64 range", "-9223372036854775809"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Record{"v": tt.value}

			if got := r.Int64("v"); got != 0 {
				t.Errorf("Int64() = %d, want 0", got)
			}
			if got := r.Int("v"); got != 0 {
				t.Errorf("Int() = %d, want 0", got)
			}
		})
	}
}

// TestIntNarrowingOnSmallPlatforms covers the case that only exists where int
// is 32 bits wide, which includes this repository's GOARCH=arm build targets
// (Raspberry Pi 1, Zero, and the 32-bit builds for Pi 2/3/4).
func TestIntNarrowingOnSmallPlatforms(t *testing.T) {
	// A variable, not a constant: int(const) is evaluated at compile time and
	// would fail the 32-bit build even in the branch that never runs there.
	var tooBigFor32Bit int64 = 4306644447701 // a phone number, as it happens

	r := Record{"v": tooBigFor32Bit}

	// Int64 is unaffected by the platform.
	if got := r.Int64("v"); got != tooBigFor32Bit {
		t.Errorf("Int64() = %d, want %d", got, tooBigFor32Bit)
	}

	got := r.Int("v")
	switch strconv.IntSize {
	case 64:
		if got != int(tooBigFor32Bit) {
			t.Errorf("Int() = %d, want %d on a 64-bit platform", got, tooBigFor32Bit)
		}
	default:
		if got != 0 {
			t.Errorf("Int() = %d, want 0: the value does not fit a %d-bit int, "+
				"and a silently truncated phone number is worse than none", got, strconv.IntSize)
		}
	}
}

// TestIntConversionInRange makes sure the guards did not break the ordinary
// cases, including the exact boundaries.
func TestIntConversionInRange(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
	}{
		{"zero", 0.0, 0},
		{"truncates towards zero", 3.99, 3},
		{"truncates towards zero, negative", -3.99, -3},
		{"largest int64", int64(math.MaxInt64), math.MaxInt64},
		{"smallest int64", int64(math.MinInt64), math.MinInt64},
		{"float at the lower int64 boundary", float64(math.MinInt64), math.MinInt64},
		{"string at the upper int64 boundary", "9223372036854775807", math.MaxInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Record{"v": tt.value}

			if got := r.Int64("v"); got != tt.want {
				t.Errorf("Int64() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewRecord(t *testing.T) {
	r := NewRecord()

	if r == nil {
		t.Fatal("NewRecord returned nil")
	}
	if len(r) != 0 {
		t.Errorf("a new record holds %d entries, want 0", len(r))
	}

	// A record from NewRecord must be writable; the zero value of the type is
	// a nil map and would panic here.
	r.Set("k", 1)
	if got := r.Int("k"); got != 1 {
		t.Errorf("Int(k) = %d, want 1", got)
	}
}

func TestSet(t *testing.T) {
	r := NewRecord()

	r.Set("k", "first")
	if got := r.String("k"); got != "first" {
		t.Errorf("String(k) = %q, want %q", got, "first")
	}

	r.Set("k", "second") // overwrites
	if got := r.String("k"); got != "second" {
		t.Errorf("String(k) = %q, want %q", got, "second")
	}

	r.Set("nil", nil)
	if !r.Exists("nil") {
		t.Error("a key set to nil does not exist")
	}
	if v, ok := r.Value("nil"); !ok || v != nil {
		t.Errorf("Value(nil) = (%v, %v), want (<nil>, true)", v, ok)
	}
}

func TestCopy(t *testing.T) {
	original := Record{"a": 1, "b": "two"}

	clone := original.Copy()

	if len(clone) != len(original) {
		t.Fatalf("the copy holds %d entries, want %d", len(clone), len(original))
	}
	for k, v := range original {
		if clone[k] != v {
			t.Errorf("copy[%q] = %v, want %v", k, clone[k], v)
		}
	}

	// The copy must be independent of the original.
	clone.Set("a", 99)
	if got := original.Int("a"); got != 1 {
		t.Errorf("writing to the copy changed the original: a = %d, want 1", got)
	}

	original.Set("c", 3)
	if clone.Exists("c") {
		t.Error("writing to the original changed the copy")
	}
}

// TestCopyIsShallow records the documented limitation: values are copied as
// they are, so a map or slice stays shared between original and copy.
func TestCopyIsShallow(t *testing.T) {
	shared := []int{1, 2, 3}
	original := Record{"slice": shared}

	clone := original.Copy()
	shared[0] = 99

	v, _ := clone.Value("slice")
	if got := v.([]int)[0]; got != 99 {
		t.Errorf("clone sees %d, want 99: Copy is documented as shallow", got)
	}
}

// TestUnsupportedTypesYieldZero documents the conversion boundary: only bool,
// int, int64, float64 and string are understood. Anything else - and that
// includes the other integer widths a database driver may hand out - reads as
// the zero value rather than as an error.
func TestUnsupportedTypesYieldZero(t *testing.T) {
	for _, value := range []any{int32(7), uint(7), uint64(7), float32(7), []byte("7"), nil} {
		r := Record{"v": value}

		if got := r.Int("v"); got != 0 {
			t.Errorf("Int() = %d for %T, want 0", got, value)
		}
		if got := r.String("v"); got != "" {
			t.Errorf("String() = %q for %T, want an empty string", got, value)
		}
		if got := r.Bool("v"); got {
			t.Errorf("Bool() = true for %T, want false", value)
		}
	}
}

// TestBoolIsNonZero pins down the rule for numbers: a value is true when it is
// non-zero, following the usual convention. NaN is the one exception - "not a
// number" is not a truth value - and number-like strings follow the same rule
// as numbers, so that "2" and 2 cannot disagree.
func TestBoolIsNonZero(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},

		{"one", 1, true},
		{"zero", 0, false},
		{"two", 2, true},
		{"negative", -1, true},
		{"large", 2134, true},

		{"int64 one", int64(1), true},
		{"int64 two", int64(2), true},
		{"int64 zero", int64(0), false},

		{"float one", 1.0, true},
		{"float two", 2.0, true},
		{"float fraction", 0.1, true},
		{"float zero", 0.0, false},
		{"negative zero", math.Copysign(0, -1), false},
		{"infinity", math.Inf(1), true},
		{"negative infinity", math.Inf(-1), true},
		{"not a number", math.NaN(), false},

		{"named true", "true", true},
		{"named yes", "yes", true},
		{"named on", "on", true},
		{"named uppercase", "TRUE", true},
		{"named false", "false", false},
		{"named no", "no", false},
		{"named off", "off", false},

		{"string one", "1", true},
		{"string zero", "0", false},
		{"string two", "2", true},
		{"string negative", "-1", true},
		{"string fraction", "0.5", true},
		{"string with sign", "+4306644447701", true},
		{"string not a number", "NaN", false},
		{"string non numeric", "a", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Record{"v": tt.value}

			if got := r.Bool("v"); got != tt.want {
				t.Errorf("Bool() = %v for %#v, want %v", got, tt.value, tt.want)
			}
		})
	}
}
