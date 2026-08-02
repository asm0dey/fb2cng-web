package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/schema"
)

type rowVM struct {
	Key         string
	Label       string
	Kind        string // "bool","int","string","enum"
	Enum        []string
	StrValue    string
	DefaultStr  string
	Description string
	Checked     bool // for bool controls
	Changed     bool
	Known       bool // present in the schema
	IsTemplate  bool // output_name_template -> textarea + preview
}

type groupVM struct {
	Name    string
	Total   int
	Changed int
	Rows    []rowVM
}

type effectiveVM struct {
	YAML          string
	Error         string
	Valid         bool
	Invalid       bool
	OverrideCount int
}

type editorVM struct {
	layout          // Plan 1's header struct: Tab, ContentName, User — so base can render chrome + nav
	Preset          *presets.Preset
	IsDefaultPreset bool
	TotalOptions    int
	ChangedCount    int
	Groups          []groupVM
	Effective       effectiveVM
}

// normalizeStr renders a value the way the editor displays it, matching
// schema.Option.IsDefault's normalization of numeric types.
func normalizeStr(v any) string {
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

// flattenOverrides flattens a nested preset override map into dotted keys.
func flattenOverrides(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flattenOverrides(key, sub, out)
			continue
		}
		out[key] = v
	}
}

// buildEditorVM builds the grid: every schema option in schema order (grouped by
// Group), plus any override keys not covered by the schema as synthetic rows.
func (s *Server) buildEditorVM(p *presets.Preset, flat map[string]any) editorVM {
	order := []string{}
	byName := map[string]*groupVM{}
	ensure := func(name string) *groupVM {
		g, ok := byName[name]
		if !ok {
			g = &groupVM{Name: name}
			byName[name] = g
			order = append(order, name)
		}
		return g
	}

	used := map[string]bool{}
	for _, gname := range s.schema.Groups() {
		g := ensure(gname)
		for _, opt := range s.schema.InGroup(gname) {
			val, present := flat[opt.Key]
			if !present {
				val = opt.Default
			}
			used[opt.Key] = true
			g.Rows = append(g.Rows, s.rowFor(opt, val, present))
		}
	}

	// Synthetic rows for override keys the schema does not describe.
	var extra []string
	for k := range flat {
		if !used[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		g := ensure(strings.Split(k, ".")[0])
		g.Rows = append(g.Rows, rowVM{
			Key:      k,
			Label:    k,
			Kind:     string(schema.KindString),
			StrValue: normalizeStr(flat[k]),
			Changed:  true,
			Known:    false,
		})
	}

	vm := editorVM{Preset: p, TotalOptions: len(s.schema.Options)}
	for _, name := range order {
		g := byName[name]
		g.Total = len(g.Rows)
		for _, row := range g.Rows {
			if row.Changed {
				g.Changed++
				vm.ChangedCount++
			}
		}
		vm.Groups = append(vm.Groups, *g)
	}
	return vm
}

func (s *Server) rowFor(opt schema.Option, val any, present bool) rowVM {
	r := rowVM{
		Key:         opt.Key,
		Label:       opt.Label,
		Kind:        string(opt.Kind),
		Enum:        opt.Enum,
		StrValue:    normalizeStr(val),
		DefaultStr:  normalizeStr(opt.Default),
		Description: opt.Description,
		Known:       true,
		IsTemplate:  strings.Contains(opt.Key, "output_name_template"),
	}
	if opt.Kind == schema.KindBool {
		r.Checked = r.StrValue == "true"
	}
	r.Changed = present && !opt.IsDefault(val)
	return r
}

func (s *Server) handlePresetEditor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.presets.Get(id)
	if err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	flat := map[string]any{}
	flattenOverrides("", p.Overrides, flat)

	vm := s.buildEditorVM(p, flat)
	vm.layout = layout{Tab: "settings", ContentName: "editor", User: r.Header.Get("Remote-User")}
	vm.IsDefaultPreset = s.presets.DefaultID() == id ||
		(s.presets.DefaultID() == "" && id == "defaults")

	// Render the "editor" content template through Plan 1's base layout.
	s.render(w, vm)
}
