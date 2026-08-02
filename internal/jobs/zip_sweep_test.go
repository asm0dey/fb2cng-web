package jobs

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteZipFlatWithCollisionSuffix(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	id, _ := s.Create("defaults", "epub3")
	// Two inputs each produce a "book.epub" — the zip must not collide.
	for _, in := range []string{"a.fb2", "b.fb2"} {
		dir := s.OutDir(id, in)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "book.epub"), []byte("DATA"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := s.WriteZip(id, &buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		if names[f.Name] {
			t.Fatalf("duplicate zip entry %q", f.Name)
		}
		names[f.Name] = true
	}
	if len(zr.File) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(zr.File), names)
	}
	if !names["book.epub"] || (!names["book-1.epub"]) {
		t.Fatalf("expected book.epub + book-1.epub, got %v", names)
	}
}

func TestSweepRemovesOldJobs(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	oldID, _ := s.Create("defaults", "epub3")
	newID, _ := s.Create("defaults", "epub3")

	// Age the old job's dir past the TTL.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(s.Dir, oldID), past, past); err != nil {
		t.Fatal(err)
	}

	removed := s.Sweep(time.Now())
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, oldID)); !os.IsNotExist(err) {
		t.Fatalf("old job should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, newID)); err != nil {
		t.Fatalf("new job should survive: %v", err)
	}
}
