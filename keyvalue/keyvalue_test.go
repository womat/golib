package keyvalue

import (
	"testing"
)

const int64Zero = int64(0)
const float64Zero = float64(0.0)

var myTestRecord = Record{
	"":            "",
	"emptyString": "",
	"stringa":     "a",
	"string1":     "1",
	"string0":     "0",
	"stringPhone": "+4306644447701",
	"StringHuge":  "1234567890123456.0",
	"StringSmall": "0.00000012345678",
	"int2134":     2134,
	"float3.14":   3.14,
	"boolTrue":    true,
	"boolFalse":   false,
	"int1":        1,
	"int-1":       -1,
	"int0":        0,
	"float0.0":    0.0,
	"float1.0":    1.0,
	"float-1.0":   -1.0,
	"float0.1":    0.1,
	"floatPhone":  4306644447701.0,
	"floatHuge":   1234567890123456.0,
	"floatSmall":  0.00000012345678,
}

func Test_Bool(t *testing.T) {
	expectedStrict := map[string]interface{}{
		"":            false,
		"emptyString": false,
		"stringa":     false,
		"string1":     false,
		"string0":     false,
		"stringPhone": false,
		"int2134":     false,
		"float3.14":   false,
		"boolTrue":    true,
		"boolFalse":   false,
		"int1":        false,
		"int-1":       false,
		"int0":        false,
		"float0.0":    false,
		"float1.0":    false,
		"float-1.0":   false,
		"float0.1":    false,
		"floatPhone":  false,
	}
	expectedConverted := map[string]interface{}{
		"":            false,
		"emptyString": false,
		"stringa":     false,
		"string1":     true,
		"string0":     false,
		"stringPhone": false,
		"int2134":     false,
		"float3.14":   false,
		"boolTrue":    true,
		"boolFalse":   false,
		"int1":        true,
		"int-1":       false,
		"int0":        false,
		"float0.0":    false,
		"float1.0":    true,
		"float-1.0":   false,
		"float0.1":    false,
		"floatPhone":  false,
	}

	for k, v := range expectedStrict {
		x := myTestRecord.Bool(k)
		if x != v {
			t.Errorf("unexpected input = %v, output = %v", k, x)
		}
	}

	for k, v := range expectedConverted {
		x := myTestRecord.Bool(k, true)
		if x != v {
			t.Errorf("unexpected (converted) input = %v, output = %v", k, x)
		}
	}

}

func Test_String(t *testing.T) {
	expectedStrict := map[string]interface{}{
		"":            "",
		"emptyString": "",
		"stringa":     "a",
		"string1":     "1",
		"string0":     "0",
		"stringPhone": "+4306644447701",
		"int2134":     "",
		"float3.14":   "",
		"boolTrue":    "",
		"boolFalse":   "",
		"int1":        "",
		"int-1":       "",
		"int0":        "",
		"float0.0":    "",
		"float1.0":    "",
		"float-1.0":   "",
		"float0.1":    "",
		"floatPhone":  "",
		"floatHuge":   "",
	}

	expectedConverted := map[string]interface{}{
		"":            "",
		"emptyString": "",
		"stringa":     "a",
		"string1":     "1",
		"string0":     "0",
		"stringPhone": "+4306644447701",
		"int2134":     "2134",
		"float3.14":   "3.14",
		"boolTrue":    "true",
		"boolFalse":   "false",
		"int1":        "1",
		"int-1":       "-1",
		"int0":        "0",
		"float0.0":    "0",
		"float1.0":    "1",
		"float-1.0":   "-1",
		"float0.1":    "0.1",
		"floatPhone":  "4306644447701",
		"floatHuge":   "1234567890123456",
		"floatSmall":  "0.00000012345678",
	}

	for k, v := range expectedStrict {
		x := myTestRecord.String(k)
		if x != v {
			t.Errorf("unexpected input = %v, output = %q", k, x)
		}
	}

	for k, v := range expectedConverted {
		x := myTestRecord.String(k, true)
		if x != v {
			t.Errorf("unexpected (converted) input = %v, output = %q", k, x)
		}
	}

	x := myTestRecord.String("key does not exist", true)
	if x != "" {
		t.Errorf("expect an empty string on nil")
	}
}

func Test_Int(t *testing.T) {
	expectedStrict := map[string]interface{}{
		"":            0,
		"emptyString": 0,
		"stringa":     0,
		"string1":     0,
		"string0":     0,
		"stringPhone": 0,
		"int2134":     2134,
		"float3.14":   0,
		"boolTrue":    0,
		"boolFalse":   0,
		"int1":        1,
		"int-1":       -1,
		"int0":        0,
		"float0.0":    0,
		"float1.0":    0,
		"float-1.0":   0,
		"float0.1":    0,
		"floatPhone":  0,
	}

	expectedConverted := map[string]interface{}{
		"":            0,
		"emptyString": 0,
		"stringa":     0,
		"string1":     1,
		"string0":     0,
		"stringPhone": 4306644447701,
		"int2134":     2134,
		"float3.14":   3,
		"boolTrue":    1,
		"boolFalse":   0,
		"int1":        1,
		"int-1":       -1,
		"int0":        0,
		"float0.0":    0,
		"float1.0":    1,
		"float-1.0":   -1,
		"float0.1":    0,
		"floatPhone":  4306644447701,
	}

	for k, v := range expectedStrict {
		x := myTestRecord.Int(k)
		if x != v {
			t.Errorf("unexpected input = %v, output = %v", k, x)
		}
	}

	for k, v := range expectedConverted {
		x := myTestRecord.Int(k, true)
		if x != v {
			t.Errorf("unexpected (converted) input = %v, output = %v", k, x)
		}
	}
}

func Test_Int64(t *testing.T) {
	expectedStrict := map[string]interface{}{
		"":            int64Zero,
		"emptyString": int64Zero,
		"stringa":     int64Zero,
		"string1":     int64Zero,
		"string0":     int64Zero,
		"stringPhone": int64Zero,
		"int2134":     int64(2134),
		"float3.14":   int64Zero,
		"boolTrue":    int64Zero,
		"boolFalse":   int64Zero,
		"int1":        int64(1),
		"int-1":       int64(-1),
		"int0":        int64Zero,
		"float0.0":    int64Zero,
		"float1.0":    int64Zero,
		"float-1.0":   int64Zero,
		"float0.1":    int64Zero,
		"floatPhone":  int64Zero,
	}

	expectedConverted := map[string]interface{}{
		"":            int64Zero,
		"emptyString": int64Zero,
		"stringa":     int64Zero,
		"string1":     int64(1),
		"string0":     int64Zero,
		"stringPhone": int64(4306644447701),
		"int2134":     int64(2134),
		"float3.14":   int64(3),
		"boolTrue":    int64(1),
		"boolFalse":   int64Zero,
		"int1":        int64(1),
		"int-1":       int64(-1),
		"int0":        int64Zero,
		"float0.0":    int64Zero,
		"float1.0":    int64(1),
		"float-1.0":   int64(-1),
		"float0.1":    int64Zero,
		"floatPhone":  int64(4306644447701),
	}

	for k, v := range expectedStrict {
		x := myTestRecord.Int64(k)
		if x != v {
			t.Errorf("unexpected input = %v, output = %v", k, x)
		}
	}

	for k, v := range expectedConverted {
		x := myTestRecord.Int64(k, true)
		if x != v {
			t.Errorf("unexpected (converted) input = %v, output = %v", k, x)
		}
	}
}

func Test_Float(t *testing.T) {
	expectedStrict := map[string]interface{}{
		"":            float64Zero,
		"emptyString": float64Zero,
		"stringa":     float64Zero,
		"string1":     float64Zero,
		"string0":     float64Zero,
		"stringPhone": float64Zero,
		"StringHuge":  float64Zero,
		"StringSmall": float64Zero,
		"int2134":     float64Zero,
		"float3.14":   3.14,
		"boolTrue":    float64Zero,
		"boolFalse":   float64Zero,
		"int1":        float64Zero,
		"int-1":       float64Zero,
		"int0":        float64Zero,
		"float0.0":    float64Zero,
		"float1.0":    1.0,
		"float-1.0":   -1.0,
		"float0.1":    0.1,
		"floatPhone":  4306644447701.0,
	}

	expectedConverted := map[string]interface{}{
		"":            float64Zero,
		"emptyString": float64Zero,
		"stringa":     float64Zero,
		"string1":     1.0,
		"string0":     float64Zero,
		"stringPhone": 4306644447701.0,
		"StringHuge":  1234567890123456.0,
		"StringSmall": 0.00000012345678,
		"int2134":     2134.0,
		"float3.14":   3.14,
		"boolTrue":    1.0,
		"boolFalse":   float64Zero,
		"int1":        1.0,
		"int-1":       -1.0,
		"int0":        float64Zero,
		"float0.0":    float64Zero,
		"float1.0":    1.0,
		"float-1.0":   -1.0,
		"float0.1":    0.1,
		"floatPhone":  4306644447701.0,
	}

	for k, v := range expectedStrict {
		x := myTestRecord.Float64(k)
		if x != v {
			t.Errorf("unexpected input = %v, output = %v", k, x)
		}
	}

	for k, v := range expectedConverted {
		x := myTestRecord.Float64(k, true)
		if x != v {
			t.Errorf("unexpected (converted) input = %v, output = %v", k, x)
		}
	}
}

func Test_Value(t *testing.T) {
	for _, key := range []string{"", "emptyString", "stringa"} {
		if _, ok := myTestRecord.Value(key).(string); !ok {
			t.Errorf("unexpected string format for key %q", key)
		}
	}

	for _, key := range []string{"boolTrue", "boolTrue"} {
		if _, ok := myTestRecord.Value(key).(bool); !ok {
			t.Errorf("unexpected bool format for key %q", key)
		}
	}

	for _, key := range []string{"x", ",", "-2"} {
		ret := myTestRecord.Value(key)
		if ret != nil {
			t.Errorf("unexpected for key %q", key)
		}
	}
}

func Test_Exists(t *testing.T) {
	for _, key := range []string{"", "emptyString", "stringa"} {
		if ok := myTestRecord.Exists(key); !ok {
			t.Errorf("unexpected for key %s", key)
		}
	}
	for _, key := range []string{"x", "YemptyString", "Zstringa"} {
		if ok := myTestRecord.Exists(key); ok {
			t.Errorf("unexpected for key %s", key)
		}
	}
}

func TestRecord_GetSortedKeys(t *testing.T) {
	testRecord := Record{
		"9":   1,
		"a":   1,
		"123": 1,
		"+":   1,
	}

	expectedRecord := []string{"+", "123", "9", "a"}
	keys := testRecord.GetSortedKeys()

	if l := len(keys); l != len(expectedRecord) {
		t.Errorf("unexpected len of result: got: %q, expected: %q", l, len(expectedRecord))
	}

	for n, key := range keys {
		if e := expectedRecord[n]; key != e {
			t.Errorf("key got: %q, expected: %q", key, e)

		}
	}

	if l := len(Record{}.GetSortedKeys()); l != 0 {
		t.Errorf("unexpected len of result: got: %q, expected: %q", l, 0)

	}
}
