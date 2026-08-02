package convert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAcceptsGoodConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfg, []byte("version: 1\ndocument:\n  toc_type: normal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(fakeBin(t)).Validate(context.Background(), cfg); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfg, []byte("__invalid: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(fakeBin(t)).Validate(context.Background(), cfg); err == nil {
		t.Fatal("invalid config accepted")
	}
}
