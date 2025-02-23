package keyvalue

| Function                                                     | Comment                         |
|--------------------------------------------------------------|---------------------------------|
| func NewRecord() Record                                      | does make for a new record      |
| func (r Record) Exists(key string) bool                      | checks the existence of the key |
| func (r Record) Value(key string) interface{}                | returns the value               |
| func (r Record) Bool(key string, convert ...bool) bool       | returns a bool value            |
| func (r Record) Float64(key string, convert ...bool) float64 | returns a float value           |
| func (r Record) Int(key string, convert ...bool) int         | returns an int value            |
| func (r Record) Int64(key string, convert ...bool) int64     | returns an int64 value          |
| func (r Record) String(key string, convert ...bool) string   | returns a string value          |
| func (r Record) Copy() Record                                | copy a record                   |
| func (r Record) GetSortedKeys() []string                     | returns keys sorted             |

