package main

import (
	"os"
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
