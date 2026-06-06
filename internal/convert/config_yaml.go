package convert

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// FormOptions are the common knobs the web form exposes. A nil pointer means
// "not set by the user" and is left to fbc's default / the raw YAML value.
type FormOptions struct {
	TocType          *string // document.toc_type
	ImagesOptimize   *bool   // document.images.optimize
	JpegQuality      *int    // document.images.jpeg_quality_level
	FootnotesMode    *string // document.footnotes.mode
	InsertSoftHyphen *bool   // document.insert_soft_hyphen
}

func (o FormOptions) empty() bool {
	return o.TocType == nil && o.ImagesOptimize == nil && o.JpegQuality == nil &&
		o.FootnotesMode == nil && o.InsertSoftHyphen == nil
}

// BuildConfig merges form options on top of the user's raw YAML and returns the
// effective config to pass to fbc via -c. Form values win on conflict. Returns
// (nil, nil) when neither raw YAML nor any form option is provided, so the
// caller sends no -c and fbc uses its embedded defaults.
func BuildConfig(rawYAML string, o FormOptions) ([]byte, error) {
	if strings.TrimSpace(rawYAML) == "" && o.empty() {
		return nil, nil
	}

	root := map[string]any{}
	if rawYAML != "" {
		if err := yaml.Unmarshal([]byte(rawYAML), &root); err != nil {
			return nil, fmt.Errorf("parse config YAML: %w", err)
		}
	}
	if _, ok := root["version"]; !ok {
		root["version"] = 1
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
