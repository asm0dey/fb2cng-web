package main

import (
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
