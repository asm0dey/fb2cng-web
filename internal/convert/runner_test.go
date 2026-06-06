package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeBin(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fake-fbc.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fake fbc missing: %v", err)
	}
	return abs
}

func TestDumpDefaults(t *testing.T) {
	out, err := New(fakeBin(t)).DumpDefaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version: 1") {
		t.Fatalf("unexpected defaults: %s", out)
	}
}

func TestConvertSingleOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "book.fb2")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")

	outs, err := New(fakeBin(t)).Convert(context.Background(), in, "epub3", "", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || filepath.Base(outs[0]) != "book.epub" {
		t.Fatalf("unexpected outputs: %v", outs)
	}
}

func TestConvertMultiOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "multi.fb2")
	os.WriteFile(in, []byte("x"), 0o644)
	outs, err := New(fakeBin(t)).Convert(context.Background(), in, "epub3", "", filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 {
		t.Fatalf("expected 2 outputs, got %v", outs)
	}
}

func TestConvertError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "corrupt.fb2")
	os.WriteFile(in, []byte("x"), 0o644)
	_, err := New(fakeBin(t)).Convert(context.Background(), in, "epub3", "", filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Fatalf("expected fbc error with stderr, got %v", err)
	}
}
