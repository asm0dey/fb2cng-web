package convert

import (
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestBuildConfigEmpty(t *testing.T) {
	out, err := BuildConfig("", FormOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil config when nothing set, got %q", out)
	}
}

func TestBuildConfigFormOnly(t *testing.T) {
	out, err := BuildConfig("", FormOptions{
		TocType:        ptr("flat"),
		ImagesOptimize: ptr(false),
		JpegQuality:    ptr(80),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"version: 1", "toc_type: flat", "optimize: false", "jpeg_quality_level: 80"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestBuildConfigFormWinsOverRaw(t *testing.T) {
	raw := "version: 1\ndocument:\n  toc_type: normal\n  insert_soft_hyphen: false\n"
	out, err := BuildConfig(raw, FormOptions{TocType: ptr("old_kindle")})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "toc_type: old_kindle") {
		t.Errorf("form value should win, got:\n%s", s)
	}
	if !strings.Contains(s, "insert_soft_hyphen: false") {
		t.Errorf("unrelated raw key should be preserved, got:\n%s", s)
	}
}

func TestBuildConfigRawOnlyEnsuresVersion(t *testing.T) {
	out, err := BuildConfig("document:\n  toc_type: flat\n", FormOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version: 1") {
		t.Errorf("version must be ensured, got:\n%s", out)
	}
}

func TestBuildConfigFootnotesMode(t *testing.T) {
	out, err := BuildConfig("", FormOptions{FootnotesMode: ptr("float")})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"version: 1", "mode: float"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestBuildConfigInvalidYAML(t *testing.T) {
	if _, err := BuildConfig("\tnot: [valid", FormOptions{}); err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestDeepMerge(t *testing.T) {
	dst := map[string]any{
		"version": 1,
		"document": map[string]any{
			"insert_soft_hyphen": true,
			"footnotes":          map[string]any{"mode": "floatRenumbered"},
		},
	}
	src := map[string]any{
		"document": map[string]any{
			"insert_soft_hyphen": false,  // scalar override
			"toc_type":           "flat", // new key
		},
	}
	deepMerge(dst, src)

	doc := dst["document"].(map[string]any)
	if doc["insert_soft_hyphen"] != false {
		t.Errorf("src should override scalar, got %v", doc["insert_soft_hyphen"])
	}
	if doc["toc_type"] != "flat" {
		t.Errorf("src key should be added, got %v", doc["toc_type"])
	}
	fn := doc["footnotes"].(map[string]any)
	if fn["mode"] != "floatRenumbered" {
		t.Errorf("untouched nested key should be preserved, got %v", fn["mode"])
	}
}
