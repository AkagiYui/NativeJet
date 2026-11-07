// Package config is the only configuration composition implementation.
// Maps merge recursively, sequences replace, and an explicit null deletes.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"nativejet/internal/model"
)

const MaxBytes = 4 << 20

type Layer struct{ Name, Content string }

func Parse(text string) (map[string]any, map[string]int, error) {
	if len(text) > MaxBytes {
		return nil, nil, errors.New("配置超过 4 MB")
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, map[string]int{}, nil
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, nil, errors.New("仅支持一个 YAML 文档")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("配置根节点必须是 YAML 对象")
	}
	var out map[string]any
	if err := doc.Decode(&out); err != nil {
		return nil, nil, err
	}
	lines := map[string]int{}
	var walk func(*yaml.Node, string, int) error
	seen := map[*yaml.Node]bool{}
	count := 0
	walk = func(n *yaml.Node, path string, depth int) error {
		count++
		if depth > 64 || count > 100000 {
			return errors.New("YAML 层级或节点数量超过限制")
		}
		if seen[n] {
			return errors.New("不支持循环 YAML 锚点")
		}
		seen[n] = true
		defer delete(seen, n)
		if n.Kind == yaml.AliasNode {
			return walk(n.Alias, path, depth+1)
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if k.Tag == "!!merge" {
					if err := walk(v, path, depth+1); err != nil {
						return err
					}
					continue
				}
				if k.Tag != "!!str" {
					return errors.New("配置键必须为字符串")
				}
				p := k.Value
				if path != "" {
					p = path + "." + p
				}
				lines[p] = v.Line
				if err := walk(v, p, depth+1); err != nil {
					return err
				}
			}
		} else if n.Kind == yaml.SequenceNode {
			for _, v := range n.Content {
				if err := walk(v, path, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(doc.Content[0], "", 0); err != nil {
		return nil, nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, lines, nil
}

func Compile(layers ...Layer) (model.Preview, map[string]any, error) {
	result := map[string]any{}
	traces := map[string][]model.Trace{}
	var record func(string, any, string, map[string]int)
	record = func(path string, value any, layer string, lines map[string]int) {
		traces[path] = append(traces[path], model.Trace{Layer: layer, Value: value, Line: lines[path], Deleted: value == nil})
		if m, ok := value.(map[string]any); ok {
			for k, v := range m {
				p := k
				if path != "" {
					p = path + "." + k
				}
				record(p, v, layer, lines)
			}
		}
	}
	var merge func(map[string]any, map[string]any, string, string, map[string]int)
	merge = func(dst, src map[string]any, prefix, layer string, lines map[string]int) {
		for k, v := range src {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			if v == nil {
				record(path, nil, layer, lines)
				delete(dst, k)
				continue
			}
			if m, ok := v.(map[string]any); ok {
				traces[path] = append(traces[path], model.Trace{Layer: layer, Value: v, Line: lines[path]})
				current, ok := dst[k].(map[string]any)
				if !ok {
					current = map[string]any{}
				}
				merge(current, m, path, layer, lines)
				dst[k] = current
			} else {
				record(path, v, layer, lines)
				dst[k] = v
			}
		}
	}
	for _, l := range layers {
		m, lines, err := Parse(l.Content)
		if err != nil {
			return model.Preview{}, nil, fmt.Errorf("%s: %w", l.Name, err)
		}
		merge(result, m, "", l.Name, lines)
	}
	out, err := yaml.Marshal(result)
	if err != nil {
		return model.Preview{}, nil, err
	}
	sum := sha256.Sum256(out)
	preview := model.Preview{YAML: string(out), Hash: hex.EncodeToString(sum[:])}
	var flatten func(map[string]any, string)
	flatten = func(m map[string]any, prefix string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := m[k]
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			if child, ok := v.(map[string]any); ok && len(child) > 0 {
				flatten(child, path)
				continue
			}
			t := traces[path]
			source := ""
			if len(t) > 0 {
				source = t[len(t)-1].Layer
			}
			preview.Fields = append(preview.Fields, model.Field{Path: path, Value: v, Source: source, Trace: t})
		}
	}
	flatten(result, "")
	return preview, result, nil
}

func BoolAt(m map[string]any, path string) bool { v, _ := At(m, path); b, _ := v.(bool); return b }
func At(m map[string]any, path string) (any, bool) {
	var value any = m
	for _, p := range strings.Split(path, ".") {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = obj[p]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

// Redact creates a presentation-only view. The actual launch file stays intact.
func Redact(text string) string {
	m, _, err := Parse(text)
	if err != nil {
		return "[配置无法解析，未展示内容]"
	}
	out, _ := yaml.Marshal(RedactValue("", m))
	return string(out)
}
func RedactValue(path string, value any) any {
	parts := strings.Split(path, ".")
	key := strings.ToLower(parts[len(parts)-1])
	if key == "secret" || strings.Contains(key, "password") || strings.Contains(key, "private-key") || key == "uuid" || strings.Contains(key, "token") || key == "url" || key == "authentication" || key == "users" {
		return "••••••••"
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = RedactValue(k, x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = RedactValue("", x)
		}
		return out
	default:
		return value
	}
}
