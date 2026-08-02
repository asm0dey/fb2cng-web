package jobs

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateBuildsTreeAndInitialStatus(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	id, err := s.Create("defaults", "epub3")
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 16 {
		t.Fatalf("id should be 16 hex chars, got %q", id)
	}
	for _, sub := range []string{"in", "out"} {
		if fi, err := os.Stat(filepath.Join(s.Dir, id, sub)); err != nil || !fi.IsDir() {
			t.Fatalf("missing dir %s: %v", sub, err)
		}
	}
	st, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != id || st.Preset != "defaults" || st.Format != "epub3" || st.Done {
		t.Fatalf("unexpected initial status: %+v", st)
	}
}

func TestPathBuilders(t *testing.T) {
	s := NewStore("/base", time.Hour)
	if got := s.InputPath("abc", "book.fb2"); got != filepath.Join("/base", "abc", "in", "book.fb2") {
		t.Fatalf("InputPath: %s", got)
	}
	if got := s.OutDir("abc", "book.fb2"); got != filepath.Join("/base", "abc", "out", "book.fb2") {
		t.Fatalf("OutDir: %s", got)
	}
	if got := s.LogPath("abc", "book.fb2"); got != filepath.Join("/base", "abc", "book.fb2.log") {
		t.Fatalf("LogPath: %s", got)
	}
}

func TestSaveLoadRoundtripAtomic(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	id, _ := s.Create("defaults", "epub3")
	st, _ := s.Load(id)
	st.Files = []FileResult{{
		Input:   "book.fb2",
		State:   StateDone,
		Outputs: []string{"book.epub"},
		Sizes:   map[string]int64{"book.epub": 42},
		Millis:  7,
	}}
	st.Done = true
	if err := s.Save(id, st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, id, "status.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file should be renamed away, err=%v", err)
	}
	got, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Done || len(got.Files) != 1 || got.Files[0].Sizes["book.epub"] != 42 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestOutputFileRejectsTraversal(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	for _, bad := range []string{"../secret", "a/b", `a\b`, "..", ""} {
		if _, err := s.OutputFile("job", "book.fb2", bad); err == nil {
			t.Errorf("base %q should be rejected", bad)
		}
		if _, err := s.OutputFile("job", bad, "book.epub"); err == nil {
			t.Errorf("input %q should be rejected", bad)
		}
	}
	got, err := s.OutputFile("job", "book.fb2", "book.epub")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("job", "out", "book.fb2", "book.epub")) {
		t.Fatalf("unexpected safe path: %s", got)
	}
}

// TestBadJobIDsRejectedEverywhere is the trust-boundary regression test: the
// job id flows unchecked into Store.root() = filepath.Join(s.Dir, id), so it
// must be validated exactly like a filename (see safeName) everywhere it is
// accepted as untrusted input.
func TestBadJobIDsRejectedEverywhere(t *testing.T) {
	s := NewStore(t.TempDir(), time.Hour)
	// Seed one legitimate job so a traversal id could plausibly reach real
	// sibling content if the checks were missing.
	id, err := s.Create("defaults", "epub3")
	if err != nil {
		t.Fatal(err)
	}
	_ = id

	bad := []string{"", "../x", "a/b", `a\b`, ".."}
	for _, badID := range bad {
		if ValidID(badID) {
			t.Errorf("ValidID(%q) = true, want false", badID)
		}
		if _, err := s.Load(badID); err == nil {
			t.Errorf("Load(%q) should error", badID)
		}
		if _, err := s.OutputFile(badID, "book.fb2", "book.epub"); err == nil {
			t.Errorf("OutputFile(id=%q, ...) should error", badID)
		}
		if err := s.WriteZip(badID, io.Discard); err == nil {
			t.Errorf("WriteZip(%q) should error", badID)
		}
	}

	if !ValidID(id) {
		t.Errorf("ValidID(%q) = false, want true for a real job id", id)
	}
}
