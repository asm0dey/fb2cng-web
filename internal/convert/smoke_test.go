//go:build smoke

package convert

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Requires the real fbc binary. Set FBC_BIN, e.g.:
//
//	FBC_BIN=$PWD/bin/fbc go test -tags smoke ./internal/convert/ -run TestSmoke
func TestSmokeRealConversion(t *testing.T) {
	bin := os.Getenv("FBC_BIN")
	if bin == "" {
		t.Skip("set FBC_BIN to the real fbc binary to run the smoke test")
	}
	dir := t.TempDir()
	src, err := filepath.Abs("../../testdata/sample.fb2")
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "sample.fb2")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(in, data, 0o644)

	cfg, err := BuildConfig("", FormOptions{TocType: strptr("flat")})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, cfg, 0o644)

	outs, err := New(bin).Convert(context.Background(), in, "epub3", cfgPath, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 {
		t.Fatalf("expected 1 output, got %v", outs)
	}
	// A valid epub is a zip whose first entry is "mimetype".
	zr, err := zip.OpenReader(outs[0])
	if err != nil {
		t.Fatalf("output is not a valid zip/epub: %v", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatalf("epub missing mimetype entry")
	}
	rc, _ := zr.File[0].Open()
	mt, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(mt, []byte("application/epub+zip")) {
		t.Fatalf("unexpected mimetype: %q", mt)
	}
}

func strptr(s string) *string { return &s }
