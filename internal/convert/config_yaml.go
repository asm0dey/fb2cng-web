package convert

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// FormOptions are the common knobs the web form exposes. A nil pointer means
// "not set by the user"; BuildConfig then falls back to the application default
// (see applicationDefaults), then to the raw YAML, then to fbc's own default.
type FormOptions struct {
	TocType          *string // document.toc_type
	ImagesOptimize   *bool   // document.images.optimize
	JpegQuality      *int    // document.images.jpeg_quality_level
	FootnotesMode    *string // document.footnotes.mode
	InsertSoftHyphen *bool   // document.insert_soft_hyphen
	CoverGenerate    *bool   // document.images.cover.generate
	DropcapsEnable   *bool   // document.dropcaps.enable
}

// applicationDefaults is the web app's baseline config. It sits above fbc's
// embedded defaults and below the user's raw YAML and form fields, so any of
// those can override it. These are the product defaults we always apply.
func applicationDefaults() map[string]any {
	return map[string]any{
		"version": 1,
		"document": map[string]any{
			"footnotes":          map[string]any{"mode": "floatRenumbered"},
			"insert_soft_hyphen": true,
			"dropcaps":           map[string]any{"enable": true},
			"images":             map[string]any{"cover": map[string]any{"generate": true}},
		},
	}
}

// BuildConfig returns the effective fbc config (for -c) by layering, lowest to
// highest precedence: application defaults, the user's raw YAML, then the form
// fields. It always returns a non-nil config so our defaults are applied even
// when the user supplies nothing.
func BuildConfig(rawYAML string, o FormOptions) ([]byte, error) {
	root := applicationDefaults()

	if strings.TrimSpace(rawYAML) != "" {
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(rawYAML), &raw); err != nil {
			return nil, fmt.Errorf("parse config YAML: %w", err)
		}
		deepMerge(root, raw)
	}

	doc := child(root, "document")
	if o.TocType != nil {
		doc["toc_type"] = *o.TocType
	}
	if o.InsertSoftHyphen != nil {
		doc["insert_soft_hyphen"] = *o.InsertSoftHyphen
	}
	if o.ImagesOptimize != nil || o.JpegQuality != nil {
		img := child(doc, "images")
		if o.ImagesOptimize != nil {
			img["optimize"] = *o.ImagesOptimize
		}
		if o.JpegQuality != nil {
			img["jpeg_quality_level"] = *o.JpegQuality
		}
	}
	if o.FootnotesMode != nil {
		child(doc, "footnotes")["mode"] = *o.FootnotesMode
	}
	if o.CoverGenerate != nil {
		child(child(doc, "images"), "cover")["generate"] = *o.CoverGenerate
	}
	if o.DropcapsEnable != nil {
		child(doc, "dropcaps")["enable"] = *o.DropcapsEnable
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal config YAML: %w", err)
	}
	return out, nil
}

// child returns the nested map at key, creating it if absent or not a map.
func child(m map[string]any, key string) map[string]any {
	if existing, ok := m[key].(map[string]any); ok {
		return existing
	}
	c := map[string]any{}
	m[key] = c
	return c
}

// deepMerge recursively merges src into dst. When both sides hold a nested map,
// it merges them key by key; otherwise src's value replaces dst's. dst is
// mutated in place.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = sv
	}
}
