// Package presets manages named sparse-override presets over fbc defaults.
package presets

import (
	"regexp"
	"strings"
	"time"
)

// Preset is a named sparse override map over fbc defaults.
type Preset struct {
	ID        string         `yaml:"-"` // filename stem
	Name      string         `yaml:"name"`
	Overrides map[string]any `yaml:"overrides"` // nested map, same shape BuildConfig merges
	UpdatedAt time.Time      `yaml:"updated_at"`
	Builtin   bool           `yaml:"-"` // true only for synthetic "Defaults"
}

// ChangedCount returns the number of leaf keys in Overrides.
func (p *Preset) ChangedCount() int { return countLeaves(p.Overrides) }

func countLeaves(m map[string]any) int {
	n := 0
	for _, v := range m {
		if sub, ok := v.(map[string]any); ok {
			n += countLeaves(sub)
			continue
		}
		n++
	}
	return n
}

// BuiltinID is the id of the synthetic read-only "Defaults" preset.
const BuiltinID = "defaults"

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validID reports whether id is a safe filename stem.
func validID(id string) bool {
	if id == "" || len(id) > 64 || id == "_meta" {
		return false
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) || strings.Contains(id, "..") {
		return false
	}
	return idRe.MatchString(id)
}

// slugify turns a display name into a safe candidate id.
func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	if s == "" || s == BuiltinID || s == "_meta" {
		s = "preset"
	}
	return s
}
