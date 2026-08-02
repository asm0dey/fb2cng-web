// Package schema loads the checked-in option schema (options.json) that drives
// the preset editor: which options exist, their type, group, label, default,
// and (best-effort) description.
package schema

import (
	"fmt"
	"strconv"
)

// Kind is the value type of a config option.
type Kind string

const (
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
	KindString Kind = "string"
	KindEnum   Kind = "enum"
)

// Option is one config key.
type Option struct {
	Key         string   `json:"key"`   // dotted path, e.g. "document.images.jpeg_quality_level"
	Group       string   `json:"group"` // first path segment, e.g. "document"
	Label       string   `json:"label"` // last path segment
	Kind        Kind     `json:"kind"`
	Default     any      `json:"default"`
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

// IsDefault reports whether val equals the option's default value. It normalizes
// across the numeric types JSON (float64) and form parsing (int) produce, so
// int(75) and float64(75) compare equal.
func (o Option) IsDefault(val any) bool {
	return normalize(o.Default) == normalize(val)
}

func normalize(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}
