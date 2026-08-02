package jobs

import (
	"archive/zip"
	"bytes"
	"fmt"
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

// TestWriteZipPropagatesCloseError is the regression test for bean q2t8:
// zip.Writer.Close() flushes the central directory to w, and its error must
// not be silently dropped by an unnamed defer. eocdFailWriter buffers every
// write except the one carrying the end-of-central-directory signature (the
// last bytes zip.Writer's internal bufio.Writer physically flushes to w,
// inside Close()), which it fails — simulating a write failure during the
// final flush.
func TestWriteZipPropagatesCloseError(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	id, _ := s.Create("defaults", "epub3")
	dir := s.OutDir(id, "a.fb2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "book.epub"), []byte("DATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	fw := &eocdFailWriter{}
	if err := s.WriteZip(id, fw); err == nil {
		t.Fatal("expected WriteZip to propagate zip.Writer.Close()'s error, got nil")
	}
}

// eocdFailWriter fails the one Write call whose payload contains the zip
// end-of-central-directory record signature (PK\x05\x06), and buffers
// everything else. Because archive/zip wraps a bufio.Writer around the
// destination and only calls Flush() at the very end of Close(), all of a
// small test archive's bytes (local headers, file data, central directory,
// EOCD) land in that single final Write call — so failing on the EOCD
// signature deterministically simulates a failure during Close()'s flush.
type eocdFailWriter struct {
	buf bytes.Buffer
}

func (w *eocdFailWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte{0x50, 0x4B, 0x05, 0x06}) {
		return 0, fmt.Errorf("simulated failure writing end-of-central-directory")
	}
	return w.buf.Write(p)
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
