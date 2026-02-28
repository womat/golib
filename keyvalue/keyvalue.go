// Package keyvalue provides a generic key-value record type with type-safe accessors.
// Values can be retrieved as bool, int, int64, float64, or string with automatic type conversion.
// For raw access without conversion, use the Value method.
package keyvalue

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Record is a generic key-value map.
type Record map[string]any

// NewRecord creates a new empty Record.
func NewRecord() Record {
	return make(Record)
}

// Exists reports whether key exists in the record.
func (r Record) Exists(key string) bool {
	_, ok := r[key]
	return ok
}

// Value returns the raw value for key, or nil if not found.
// Use this for special cases where type conversion is not desired.
func (r Record) Value(key string) (any, bool) {
	v, ok := r[key]
	return v, ok
}

// Set sets the value for key.
func (r Record) Set(key string, value any) {
	r[key] = value
}

// Bool returns the bool value for key.
// Converts from string, int, int64, and float64 if necessary.
// Returns false if not found or not convertible.
func (r Record) Bool(key string) bool {
	switch i := r[key].(type) {
	case bool:
		return i
	case int:
		return i == 1
	case int64:
		return i == 1
	case float64:
		return i == 1.0
	case string:
		switch strings.ToLower(i) {
		case "true", "yes", "1", "on":
			return true
		case "false", "no", "0", "off":
			return false
		}
		v, err := strconv.ParseFloat(i, 64)
		return err == nil && v == 1
	}

	return false
}

// Float64 returns the float64 value for key.
// Converts from string, int, int64, and bool if necessary.
// Returns 0.0 if not found or not convertible.
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
func (r Record) Int(key string) int {
	switch i := r[key].(type) {
	case float64:
		return int(i)
	case int64:
		return int(i)
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
// Returns 0 if not found or not convertible.
func (r Record) Int64(key string) int64 {
	switch i := r[key].(type) {
	case float64:
		return int64(i)
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
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// Copy returns a shallow copy of the record.
// Note: pointer values (maps, slices, structs) are not deep copied.
func (r Record) Copy() Record {
	record := make(Record, len(r))
	for k, v := range r {
		record[k] = v
	}
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
