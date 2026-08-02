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

type sectionVM struct {
	Name    string // raw key segment; used for data-section and search
	Label   string // prettified header
	Total   int
	Changed int
	Rows    []rowVM
}

type groupVM struct {
	Name     string
	Total    int
	Changed  int
	Flat     bool    // len(Sections) == 1 -> template renders rows directly
	Rows     []rowVM // build-time scratch; partitioned into Sections in finalize
	Sections []sectionVM
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
	gs := newGroupSet()
	used := s.appendSchemaRows(gs, flat)
	appendSyntheticRows(gs, flat, used)
	return gs.finalize(p, len(s.schema.Options))
}

// groupSet accumulates editor rows into named groups, preserving first-seen order.
type groupSet struct {
	order  []string
	byName map[string]*groupVM
}

func newGroupSet() *groupSet { return &groupSet{byName: map[string]*groupVM{}} }

func (gs *groupSet) ensure(name string) *groupVM {
	g, ok := gs.byName[name]
	if !ok {
		g = &groupVM{Name: name}
		gs.byName[name] = g
		gs.order = append(gs.order, name)
	}
	return g
}

// appendSchemaRows adds one row per schema option (in schema/group order) and
// returns the set of keys the schema covered.
func (s *Server) appendSchemaRows(gs *groupSet, flat map[string]any) map[string]bool {
	used := map[string]bool{}
	for _, gname := range s.schema.Groups() {
		g := gs.ensure(gname)
		for _, opt := range s.schema.InGroup(gname) {
			val, present := flat[opt.Key]
			if !present {
				val = opt.Default
			}
			used[opt.Key] = true
			g.Rows = append(g.Rows, s.rowFor(opt, val, present))
		}
	}
	return used
}

// appendSyntheticRows adds rows for override keys the schema does not describe.
func appendSyntheticRows(gs *groupSet, flat map[string]any, used map[string]bool) {
	var extra []string
	for k := range flat {
		if !used[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		g := gs.ensure(strings.Split(k, ".")[0])
		g.Rows = append(g.Rows, rowVM{
			Key:      k,
			Label:    k,
			Kind:     string(schema.KindString),
			StrValue: normalizeStr(flat[k]),
			Changed:  true,
			Known:    false,
		})
	}
}

// sectionName derives a group's sub-section from an option key: the 2nd path
// segment when the key nests (document.images.optimize -> "images"), else
// "general" for bare group options (document.fix_zip -> "general").
func sectionName(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) >= 3 {
		return parts[1]
	}
	return "general"
}

// prettify turns a raw section segment into a header label:
// "text_transformations" -> "Text transformations".
func prettify(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// partition splits a group's rows into sections, preserving first-seen order.
func partition(rows []rowVM) []sectionVM {
	var order []string
	byName := map[string]*sectionVM{}
	for _, r := range rows {
		name := sectionName(r.Key)
		sec, ok := byName[name]
		if !ok {
			sec = &sectionVM{Name: name, Label: prettify(name)}
			byName[name] = sec
			order = append(order, name)
		}
		sec.Rows = append(sec.Rows, r)
	}
	out := make([]sectionVM, 0, len(order))
	for _, n := range order {
		sec := byName[n]
		sec.Total = len(sec.Rows)
		for _, r := range sec.Rows {
			if r.Changed {
				sec.Changed++
			}
		}
		out = append(out, *sec)
	}
	return out
}

// finalize computes per-group and total change counts and returns the VM.
func (gs *groupSet) finalize(p *presets.Preset, totalOptions int) editorVM {
	vm := editorVM{Preset: p, TotalOptions: totalOptions}
	for _, name := range gs.order {
		g := gs.byName[name]
		g.Sections = partition(g.Rows)
		g.Rows = nil
		g.Flat = len(g.Sections) == 1
		for _, sec := range g.Sections {
			g.Total += sec.Total
			g.Changed += sec.Changed
			vm.ChangedCount += sec.Changed
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
		IsTemplate:  strings.HasSuffix(opt.Key, "_template"),
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
	vm.layout = layout{Tab: "settings", ContentName: "editor", User: s.userLabel(r)}
	vm.IsDefaultPreset = s.presets.DefaultID() == id ||
		(s.presets.DefaultID() == "" && id == "defaults")
	vm.Effective = s.computeEffective(r.Context(), flat)

	// Render the "editor" content template through Plan 1's base layout.
	s.render(w, vm)
}

// collectOverrides reads posted field values and returns only the sparse set
// that differs from the schema default (dropping equal-to-default keys). Bool
// controls post a hidden "false" plus (when checked) "true"; the last value wins.
// Unknown keys containing a dot are kept as raw strings (synthetic overrides).
func (s *Server) collectOverrides(r *http.Request) map[string]any {
	flat := map[string]any{}
	for key, vals := range r.PostForm {
		if key == "name" || key == "is_default" || key == "" || len(vals) == 0 {
			continue
		}
		v := vals[len(vals)-1]
		if opt, ok := s.schema.Get(key); ok {
			typed := parseValue(opt.Kind, v)
			if typed == nil {
				continue
			}
			if opt.IsDefault(typed) {
				continue
			}
			flat[key] = typed
			continue
		}
		if !strings.Contains(key, ".") || strings.TrimSpace(v) == "" {
			continue
		}
		flat[key] = v
	}
	return flat
}

func parseValue(k schema.Kind, v string) any {
	switch k {
	case schema.KindBool:
		return v == "true"
	case schema.KindInt:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		return nil
	default:
		return v
	}
}

// unflatten turns dotted keys back into a nested map (the shape presets store).
func unflatten(flat map[string]any) map[string]any {
	root := map[string]any{}
	for key, v := range flat {
		segs := strings.Split(key, ".")
		m := root
		for _, seg := range segs[:len(segs)-1] {
			next, ok := m[seg].(map[string]any)
			if !ok {
				next = map[string]any{}
				m[seg] = next
			}
			m = next
		}
		m[segs[len(segs)-1]] = v
	}
	return root
}

func (s *Server) handlePresetSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	p, err := s.presets.Get(id)
	if err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	if name := strings.TrimSpace(r.PostFormValue("name")); name != "" {
		p.Name = name
	}
	p.Overrides = unflatten(s.collectOverrides(r))
	if err := s.presets.Save(p); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if r.PostFormValue("is_default") == "true" {
		if err := s.presets.SetDefault(id); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
	}
	http.Redirect(w, r, "/settings/preset/"+id, http.StatusSeeOther)
}
