package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"fb2cng-web/internal/schema"
	"gopkg.in/yaml.v3"
)

func loadDump(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		t.Fatal(err)
	}
	return &n
}

func TestBuildOptionsOrderAndKinds(t *testing.T) {
	opts := buildOptions(loadDump(t, "testdata/dump.yaml"))
	want := []struct {
		key, group, label string
		kind              schema.Kind
	}{
		{"version", "version", "version", schema.KindInt},
		{"document.toc_type", "document", "toc_type", schema.KindString},
		{"document.insert_soft_hyphen", "document", "insert_soft_hyphen", schema.KindBool},
		{"document.images.optimize", "document", "optimize", schema.KindBool},
		{"document.images.jpeg_quality_level", "document", "jpeg_quality_level", schema.KindInt},
		{"document.footnotes.mode", "document", "mode", schema.KindString},
		{"document.output_name_template", "document", "output_name_template", schema.KindString},
	}
	if len(opts) != len(want) {
		t.Fatalf("got %d opts, want %d: %+v", len(opts), len(want), opts)
	}
	for i, w := range want {
		o := opts[i]
		if o.Key != w.key || o.Group != w.group || o.Label != w.label || o.Kind != w.kind {
			t.Fatalf("opt[%d]=%+v want key=%s group=%s label=%s kind=%s",
				i, o, w.key, w.group, w.label, w.kind)
		}
	}
}

func TestParseAndApplyDescriptions(t *testing.T) {
	md, err := os.ReadFile("testdata/config.md")
	if err != nil {
		t.Fatal(err)
	}
	desc := parseDescriptions(string(md))
	if desc["document.images.jpeg_quality_level"] == "" {
		t.Fatalf("jpeg description not parsed: %v", desc)
	}
	if desc["document.toc_type"] == "" {
		t.Fatalf("toc_type description (colon form) not parsed: %v", desc)
	}
	opts := buildOptions(loadDump(t, "testdata/dump.yaml"))
	applyDescriptions(opts, desc)
	var got string
	for _, o := range opts {
		if o.Key == "document.toc_type" {
			got = o.Description
		}
	}
	if got == "" {
		t.Fatal("description not applied to document.toc_type")
	}
}

func TestComputeDrift(t *testing.T) {
	old := []schema.Option{
		{Key: "gone", Kind: schema.KindString},
		{Key: "document.toc_type", Kind: schema.KindInt}, // was int
	}
	next := []schema.Option{
		{Key: "document.toc_type", Kind: schema.KindString}, // now string -> retyped
		{Key: "fresh", Kind: schema.KindBool},               // added
	}
	d := computeDrift(old, next)
	if len(d.Added) != 1 || d.Added[0] != "fresh" {
		t.Fatalf("added: %v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "gone" {
		t.Fatalf("removed: %v", d.Removed)
	}
	if len(d.Retyped) != 1 || d.Retyped[0] != "document.toc_type" {
		t.Fatalf("retyped: %v", d.Retyped)
	}
}

func TestWriteAndReloadRoundTrip(t *testing.T) {
	opts := buildOptions(loadDump(t, "testdata/dump.yaml"))
	out := filepath.Join(t.TempDir(), "options.json")
	if err := writeOptions(out, opts); err != nil {
		t.Fatal(err)
	}
	// schema.Load reads the embedded file, so reload the written array directly to
	// confirm writeOptions emits valid, re-parseable JSON.
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded []schema.Option
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("re-parse written options.json: %v", err)
	}
	if len(reloaded) != len(opts) {
		t.Fatalf("round-trip count %d vs %d", len(reloaded), len(opts))
	}
	s := &schema.Schema{Options: reloaded}
	o, ok := s.Get("document.images.jpeg_quality_level")
	if !ok || o.Kind != schema.KindInt {
		t.Fatalf("reloaded jpeg option: %+v ok=%v", o, ok)
	}
}
