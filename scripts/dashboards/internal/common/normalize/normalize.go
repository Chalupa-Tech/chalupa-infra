// Package normalize emits canonical dashboard JSON for the CI gate.
//
// Go's encoding/json sorts struct fields by source-code order; we need
// alphabetical key order so the committed JSON is stable across SDK
// rebuilds. We also strip volatile keys (version, iteration, gnetId,
// dashboard-root id) that Grafana auto-fills server-side and that
// would otherwise produce spurious diffs.
//
// The pipeline:
//  1. SDK MarshalJSON -> raw bytes (struct-order keys)
//  2. json.Unmarshal into map[string]any (loses key order)
//  3. strip volatile keys, sort tags
//  4. re-marshal via a recursive sorted-key encoder
package normalize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// volatileRootKeys are stripped from the dashboard root before re-encoding.
var volatileRootKeys = map[string]struct{}{
	"version":   {},
	"iteration": {},
	"id":        {},
	"gnetId":    {},
}

// Render takes the SDK's raw JSON output and returns the canonical,
// committable form: alphabetical keys, volatile fields stripped, tags
// sorted, two-space indent, trailing newline.
func Render(raw []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("normalize: parse SDK output: %w", err)
	}
	for k := range volatileRootKeys {
		delete(root, k)
	}
	if tags, ok := root["tags"].([]any); ok {
		strs := make([]string, 0, len(tags))
		for _, t := range tags {
			if s, ok := t.(string); ok {
				strs = append(strs, s)
			}
		}
		sort.Strings(strs)
		out := make([]any, len(strs))
		for i, s := range strs {
			out[i] = s
		}
		root["tags"] = out
	}
	var buf bytes.Buffer
	if err := encode(&buf, root, ""); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// encode is a recursive JSON encoder that emits map keys in
// alphabetical order. Slices preserve element order; primitives use
// the stdlib encoder for correctness.
func encode(buf *bytes.Buffer, v any, indent string) error {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		next := indent + "  "
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			buf.WriteString(next)
			kj, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kj)
			buf.WriteString(": ")
			if err := encode(buf, x[k], next); err != nil {
				return err
			}
			if i < len(keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent)
		buf.WriteByte('}')
	case []any:
		if len(x) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		next := indent + "  "
		for i, item := range x {
			buf.WriteString(next)
			if err := encode(buf, item, next); err != nil {
				return err
			}
			if i < len(x)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent)
		buf.WriteByte(']')
	default:
		// Primitives: stdlib handles strings, numbers, bools, nil correctly.
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}
