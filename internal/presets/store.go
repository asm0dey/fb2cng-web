package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Store owns PRESETS_DIR. One file per preset: <id>.yaml. Default marker in _meta.yaml.
type Store struct {
	Dir string
}

// NewStore returns a Store rooted at dir. The directory is created lazily on write.
func NewStore(dir string) *Store { return &Store{Dir: dir} }

func (s *Store) path(id string) string { return filepath.Join(s.Dir, id+".yaml") }
func (s *Store) metaPath() string      { return filepath.Join(s.Dir, "_meta.yaml") }

// meta is the sidecar holding the single default marker.
type meta struct {
	Default string `yaml:"default"`
}

// builtin returns a fresh synthetic read-only "Defaults" preset.
func builtin() *Preset {
	return &Preset{ID: BuiltinID, Name: "Defaults", Overrides: map[string]any{}, Builtin: true}
}

// Get returns a preset by id. "" or "defaults" returns the synthetic Builtin.
func (s *Store) Get(id string) (*Preset, error) {
	if id == "" || id == BuiltinID {
		return builtin(), nil
	}
	if !validID(id) {
		return nil, fmt.Errorf("invalid preset id %q", id)
	}
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var p Preset
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse preset %q: %w", id, err)
	}
	p.ID = id
	p.Builtin = false
	if p.Overrides == nil {
		p.Overrides = map[string]any{}
	}
	return &p, nil
}

// List returns all presets, the synthetic Builtin first, then user presets sorted by name.
func (s *Store) List() ([]*Preset, error) {
	out := []*Preset{builtin()}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	var users []*Preset
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "_meta.yaml" || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		id := strings.TrimSuffix(name, ".yaml")
		if !validID(id) {
			continue
		}
		p, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		users = append(users, p)
	}
	sort.Slice(users, func(i, j int) bool {
		return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
	})
	return append(out, users...), nil
}

// Create makes a new empty preset named name and persists it.
func (s *Store) Create(name string) (*Preset, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "New preset"
	}
	id, err := s.freeID(slugify(name))
	if err != nil {
		return nil, err
	}
	p := &Preset{ID: id, Name: name, Overrides: map[string]any{}}
	if err := s.write(p); err != nil {
		return nil, err
	}
	return p, nil
}

// freeID returns base, or base-2, base-3, … until an unused id is found.
func (s *Store) freeID(base string) (string, error) {
	if !validID(base) {
		base = "preset"
	}
	candidate := base
	for i := 2; ; i++ {
		_, err := os.Stat(s.path(candidate))
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

// Save persists p, rejecting the Builtin, and stamps UpdatedAt.
func (s *Store) Save(p *Preset) error {
	if p.Builtin || p.ID == BuiltinID {
		return fmt.Errorf("cannot save built-in preset")
	}
	if !validID(p.ID) {
		return fmt.Errorf("invalid preset id %q", p.ID)
	}
	if p.Overrides == nil {
		p.Overrides = map[string]any{}
	}
	return s.write(p)
}

// write stamps UpdatedAt and atomically writes the preset file.
func (s *Store) write(p *Preset) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	tmp := s.path(p.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(p.ID))
}

// DefaultID returns the id of the default preset, or "" if unset (UI treats "" as "defaults").
func (s *Store) DefaultID() string {
	b, err := os.ReadFile(s.metaPath())
	if err != nil {
		return ""
	}
	var m meta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return ""
	}
	if m.Default == BuiltinID {
		return ""
	}
	return m.Default
}

// SetDefault marks id as the default preset; the Builtin ("" or "defaults") clears the marker.
func (s *Store) SetDefault(id string) error {
	if id == "" || id == BuiltinID {
		return s.writeMeta(meta{Default: ""})
	}
	if !validID(id) {
		return fmt.Errorf("invalid preset id %q", id)
	}
	if _, err := os.Stat(s.path(id)); err != nil {
		return fmt.Errorf("unknown preset %q", id)
	}
	return s.writeMeta(meta{Default: id})
}

// writeMeta atomically writes the _meta.yaml sidecar.
func (s *Store) writeMeta(m meta) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	tmp := s.metaPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.metaPath())
}

// Delete removes a preset, rejecting the Builtin and the current default.
func (s *Store) Delete(id string) error {
	if id == "" || id == BuiltinID {
		return fmt.Errorf("cannot delete built-in preset")
	}
	if !validID(id) {
		return fmt.Errorf("invalid preset id %q", id)
	}
	if s.DefaultID() == id {
		return fmt.Errorf("cannot delete the default preset")
	}
	return os.Remove(s.path(id))
}

// Duplicate copies id's overrides into a new preset named newName.
func (s *Store) Duplicate(id, newName string) (*Preset, error) {
	src, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	dup, err := s.Create(newName)
	if err != nil {
		return nil, err
	}
	dup.Overrides = deepCopy(src.Overrides)
	if err := s.write(dup); err != nil {
		return nil, err
	}
	return dup, nil
}

// deepCopy clones a nested override map so callers cannot mutate the source.
func deepCopy(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			out[k] = deepCopy(sub)
			continue
		}
		out[k] = v
	}
	return out
}
