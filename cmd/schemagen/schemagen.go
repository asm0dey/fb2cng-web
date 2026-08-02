package main

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"

	"fb2cng-web/internal/schema"
	"gopkg.in/yaml.v3"
)

type kv struct {
	Key string
	Val any
}

// flattenNode walks a YAML node in document order, emitting one kv per scalar
// leaf with a dotted key path. Nested mappings recurse; sequences/other kinds
// are treated as leaf scalars (decoded as-is).
func flattenNode(prefix string, n *yaml.Node, out *[]kv) {
	if n.Kind == yaml.DocumentNode {
		for _, c := range n.Content {
			flattenNode(prefix, c, out)
		}
		return
	}
	if n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		name := n.Content[i].Value
		val := n.Content[i+1]
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if val.Kind == yaml.MappingNode {
			flattenNode(key, val, out)
			continue
		}
		var v any
		_ = val.Decode(&v)
		*out = append(*out, kv{Key: key, Val: v})
	}
}

// inferKind maps a Go scalar to a schema.Kind. Enums are never inferred (they
// require annotation); anything non-bool/non-int is a string.
func inferKind(v any) schema.Kind {
	switch v.(type) {
	case bool:
		return schema.KindBool
	case int, int64:
		return schema.KindInt
	default:
		return schema.KindString
	}
}

// buildOptions flattens the dump and builds ordered Option descriptors. Group is
// the first path segment; Label is the last.
func buildOptions(root *yaml.Node) []schema.Option {
	var kvs []kv
	flattenNode("", root, &kvs)
	opts := make([]schema.Option, 0, len(kvs))
	for _, p := range kvs {
		seg := strings.Split(p.Key, ".")
		opts = append(opts, schema.Option{
			Key:     p.Key,
			Group:   seg[0],
			Label:   seg[len(seg)-1],
			Kind:    inferKind(p.Val),
			Default: p.Val,
		})
	}
	return opts
}

// descRe matches a markdown line like:  - `dotted.key` — description
// or:  `dotted.key`: description   (separator may be em dash, hyphen, or colon).
var descRe = regexp.MustCompile("^\\s*[-*]?\\s*`([A-Za-z0-9_.]+)`\\s*[-—:]+\\s*(.+?)\\s*$")

// parseDescriptions best-effort parses key->description pairs from config.md.
func parseDescriptions(md string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(md, "\n") {
		if m := descRe.FindStringSubmatch(line); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out
}

// applyDescriptions fills Description on options that have a documented key.
func applyDescriptions(opts []schema.Option, desc map[string]string) {
	for i := range opts {
		if d, ok := desc[opts[i].Key]; ok {
			opts[i].Description = d
		}
	}
}

type driftReport struct {
	Added   []string
	Removed []string
	Retyped []string
}

// computeDrift reports keys added/removed and keys whose Kind changed.
func computeDrift(old, next []schema.Option) driftReport {
	oldByKey := map[string]schema.Option{}
	for _, o := range old {
		oldByKey[o.Key] = o
	}
	nextByKey := map[string]schema.Option{}
	for _, o := range next {
		nextByKey[o.Key] = o
	}
	var r driftReport
	for _, o := range next {
		prev, ok := oldByKey[o.Key]
		if !ok {
			r.Added = append(r.Added, o.Key)
			continue
		}
		if prev.Kind != o.Kind {
			r.Retyped = append(r.Retyped, o.Key)
		}
	}
	for _, o := range old {
		if _, ok := nextByKey[o.Key]; !ok {
			r.Removed = append(r.Removed, o.Key)
		}
	}
	sort.Strings(r.Added)
	sort.Strings(r.Removed)
	sort.Strings(r.Retyped)
	return r
}

// writeOptions writes opts as an indented JSON array.
func writeOptions(path string, opts []schema.Option) error {
	data, err := json.MarshalIndent(opts, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// loadExisting reads a prior options.json for the drift comparison; a missing or
// unparsable file yields nil (treated as an empty prior schema).
func loadExisting(path string) []schema.Option {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var opts []schema.Option
	if json.Unmarshal(data, &opts) != nil {
		return nil
	}
	return opts
}
