package presets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeRaw(t *testing.T, dir, id, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetBuiltin(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, id := range []string{"", "defaults"} {
		p, err := s.Get(id)
		if err != nil {
			t.Fatalf("Get(%q): %v", id, err)
		}
		if p.ID != "defaults" || p.Name != "Defaults" || !p.Builtin || len(p.Overrides) != 0 {
			t.Fatalf("Get(%q) = %+v, want synthetic Defaults", id, p)
		}
	}
}

func TestGetUnsafeID(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Get("../etc"); err == nil {
		t.Fatal("Get(unsafe) should error")
	}
}

func TestGetMissing(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Get("nope")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Get(missing) err = %v, want os.ErrNotExist", err)
	}
}

func TestGetReadsFile(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "kindle", "name: Kindle\noverrides:\n  document:\n    toc_type: inline\n")
	p, err := NewStore(dir).Get("kindle")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "kindle" || p.Name != "Kindle" || p.Builtin {
		t.Fatalf("got %+v", p)
	}
	doc, _ := p.Overrides["document"].(map[string]any)
	if doc["toc_type"] != "inline" {
		t.Fatalf("overrides not parsed: %+v", p.Overrides)
	}
}

func TestListBuiltinFirstAndSorted(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "beta", "name: Beta\noverrides: {}\n")
	writeRaw(t, dir, "alpha", "name: Alpha\noverrides: {}\n")
	// _meta.yaml must be ignored, not treated as a preset:
	os.WriteFile(filepath.Join(dir, "_meta.yaml"), []byte("default: alpha\n"), 0o644)

	list, err := NewStore(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range list {
		names = append(names, p.Name)
	}
	want := []string{"Defaults", "Alpha", "Beta"}
	if len(names) != len(want) {
		t.Fatalf("List names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("List names = %v, want %v", names, want)
		}
	}
	if !list[0].Builtin {
		t.Fatal("first entry must be the Builtin")
	}
}

func TestListEmptyDir(t *testing.T) {
	list, err := NewStore(filepath.Join(t.TempDir(), "does-not-exist")).List()
	if err != nil {
		t.Fatalf("List on missing dir: %v", err)
	}
	if len(list) != 1 || !list[0].Builtin {
		t.Fatalf("List on missing dir = %+v, want [Defaults]", list)
	}
}

func TestCreatePersistsAndRoundTrips(t *testing.T) {
	s := NewStore(t.TempDir())
	p, err := s.Create("Kindle")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "kindle" || p.Name != "Kindle" || p.UpdatedAt.IsZero() {
		t.Fatalf("Create = %+v", p)
	}
	got, err := s.Get("kindle")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Name != "Kindle" || len(got.Overrides) != 0 {
		t.Fatalf("reloaded = %+v", got)
	}
}

func TestCreateUniqueIDs(t *testing.T) {
	s := NewStore(t.TempDir())
	a, _ := s.Create("Kindle")
	b, _ := s.Create("Kindle")
	if a.ID == b.ID {
		t.Fatalf("duplicate names produced same id %q", a.ID)
	}
	if b.ID != "kindle-2" {
		t.Fatalf("second id = %q, want kindle-2", b.ID)
	}
}

func TestSaveRejectsBuiltin(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Save(&Preset{ID: "defaults", Name: "x", Builtin: true}); err == nil {
		t.Fatal("Save(builtin) should error")
	}
	if err := s.Save(&Preset{ID: "defaults", Name: "x"}); err == nil {
		t.Fatal("Save(id=defaults) should error")
	}
}

func TestSaveSparseRoundTripAndStampsTime(t *testing.T) {
	s := NewStore(t.TempDir())
	p, _ := s.Create("Compact")
	p.Overrides = map[string]any{
		"document": map[string]any{
			"images": map[string]any{"optimize": true, "jpeg_quality_level": 70},
		},
	}
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChangedCount() != 2 {
		t.Fatalf("round-tripped ChangedCount = %d, want 2", got.ChangedCount())
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("Save should stamp UpdatedAt")
	}
	img := got.Overrides["document"].(map[string]any)["images"].(map[string]any)
	if img["jpeg_quality_level"] != 70 || img["optimize"] != true {
		t.Fatalf("overrides not preserved: %+v", got.Overrides)
	}
}

func TestSaveRejectsMeta(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Save(&Preset{ID: "_meta", Name: "x"}); err == nil {
		t.Fatal("Save(id=_meta) should error")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "_meta.yaml")); !os.IsNotExist(err) {
		t.Fatalf("Save(id=_meta) must not create _meta.yaml, stat err = %v", err)
	}
}

func TestDefaultUnset(t *testing.T) {
	if id := NewStore(t.TempDir()).DefaultID(); id != "" {
		t.Fatalf("DefaultID unset = %q, want \"\"", id)
	}
}

func TestSetDefaultRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	p, _ := s.Create("Kindle")
	if err := s.SetDefault(p.ID); err != nil {
		t.Fatal(err)
	}
	if id := s.DefaultID(); id != p.ID {
		t.Fatalf("DefaultID = %q, want %q", id, p.ID)
	}
}

func TestSetDefaultBuiltinClears(t *testing.T) {
	s := NewStore(t.TempDir())
	p, _ := s.Create("Kindle")
	s.SetDefault(p.ID)
	if err := s.SetDefault("defaults"); err != nil {
		t.Fatal(err)
	}
	if id := s.DefaultID(); id != "" {
		t.Fatalf("DefaultID after builtin = %q, want \"\"", id)
	}
}

func TestSetDefaultUnknown(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.SetDefault("ghost"); err == nil {
		t.Fatal("SetDefault(unknown) should error")
	}
}
