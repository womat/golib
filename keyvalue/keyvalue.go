package keyvalue

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Record defines the generic map[string]interface{}
type Record map[string]interface{}

func NewRecord() Record {
	return make(Record)
}

// Exists checks the existence of the key
func (r Record) Exists(key string) bool {
	_, ok := r[key]
	return ok
}

// Value returns the value for a specific key
// or nil if it does not exist in the keyvalue record
func (r Record) Value(key string) interface{} {
	val, ok := r[key]
	if ok {
		return val
	}
	return nil
}

// Bool returns a boolean value for a specific key
// or false when not found or not convertable
func (r Record) Bool(key string, convert ...bool) bool {
	if len(convert) == 0 || !convert[0] {
		if val, ok := r[key].(bool); ok {
			return val
		}
		return false
	}
	switch i := r[key].(type) {
	case int64, int:
		return i == 1
	case float64:
		return i == 1.0
	case string:
		switch i {
		case "true":
			return true
		case "false":
			return false
		}
		v, err := strconv.ParseFloat(i, 64)
		return err == nil && v == 1
	case bool:
		return i
	}

	return false
}

// Float64 returns a float64 value for a specific key
// or 0.0 when not found or not convertable
func (r Record) Float64(key string, convert ...bool) float64 {
	if len(convert) == 0 || !convert[0] {
		if value, ok := r[key].(float64); ok {
			return value
		}
		return 0.0
	}
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

// Int returns an int value for a specific key
// or 0 when not found or not convertable
func (r Record) Int(key string, convert ...bool) int {
	if len(convert) == 0 || !convert[0] {
		if val, ok := r[key].(int); ok {
			return val
		}
		return 0
	}
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

// Int64 returns an int64 value for a specific key
// or 0 when not found or not convertable
func (r Record) Int64(key string, convert ...bool) int64 {
	if len(convert) == 0 || !convert[0] {
		if val, ok := r[key].(int64); ok {
			return val
		}
		if val, ok := r[key].(int); ok {
			return int64(val)
		}
		return 0
	}
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

// String returns a string value for a specific key
// or "" when not found
func (r Record) String(key string, convert ...bool) string {
	if len(convert) == 0 || !convert[0] {
		if str, ok := r[key].(string); ok {
			return str
		}
		return ""
	}

	switch v := r[key].(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%v", v)
	case int64:
		return fmt.Sprintf("%v", v)
	case float64:
		s := strconv.FormatFloat(v, 'f', 15, 64)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		return s
	}
	return ""
}

// Copy makes a copy of the record
// RESTRICTIONS: if the value is a pointer (maps or structs) only the reference is copied, not the content!
func (r Record) Copy() Record {
	// a map is a pointer  so maps must be copied

	record := make(Record)
	for k, v := range r {
		record[k] = v
	}

	return record
}

// GetSortedKeys returns the sorted keys as a slice
func (r Record) GetSortedKeys() []string {
	keys := make([]string, len(r))
	var n int
	for k := range r {
		keys[n] = k
		n++
	}

	sort.Strings(keys)
	return keys
}
