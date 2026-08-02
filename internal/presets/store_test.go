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
