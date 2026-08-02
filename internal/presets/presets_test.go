package presets

import "testing"

func TestChangedCountLeaves(t *testing.T) {
	p := &Preset{Overrides: map[string]any{
		"document": map[string]any{
			"toc_type": "inline",
			"images":   map[string]any{"optimize": true, "jpeg_quality_level": 82},
		},
		"output": map[string]any{"format": "epub3"},
	}}
	if got := p.ChangedCount(); got != 4 {
		t.Fatalf("ChangedCount = %d, want 4", got)
	}
	if got := (&Preset{Overrides: map[string]any{}}).ChangedCount(); got != 0 {
		t.Fatalf("empty ChangedCount = %d, want 0", got)
	}
	if got := (&Preset{}).ChangedCount(); got != 0 {
		t.Fatalf("nil-overrides ChangedCount = %d, want 0", got)
	}
}

func TestValidID(t *testing.T) {
	good := []string{"kindle", "compact-2", "a_b", "x1"}
	for _, id := range good {
		if !validID(id) {
			t.Errorf("validID(%q) = false, want true", id)
		}
	}
	bad := []string{"", "_meta", "../etc", "a/b", `a\b`, "..", "Kindle", "-lead", "a b"}
	for _, id := range bad {
		if validID(id) {
			t.Errorf("validID(%q) = true, want false", id)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Kindle":          "kindle",
		"My Preset 2":     "my-preset-2",
		"  spaced  out  ": "spaced-out",
		"!!!":             "preset",
		"Defaults":        "preset", // reserved id must not be produced
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
