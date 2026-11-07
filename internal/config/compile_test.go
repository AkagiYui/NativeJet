package config

import (
	"strings"
	"testing"
)

func TestCompositionAndProvenance(t *testing.T) {
	p, m, err := Compile(
		Layer{Name: "原文", Content: "mixed-port: 7890\ntun:\n  enable: true\n  stack: mixed\nrules: [MATCH,DIRECT]\nfuture:\n  enabled: true\n"},
		Layer{Name: "全局", Content: "mixed-port: 7897\ntun:\n  enable: false\nrules: ['MATCH,REJECT']\n"},
		Layer{Name: "本地覆盖", Content: "future:\n  enabled: null\n  opaque: preserved\n"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if BoolAt(m, "tun.enable") {
		t.Fatal("explicit false was lost")
	}
	if v, _ := At(m, "tun.stack"); v != "mixed" {
		t.Fatalf("inherited stack %v", v)
	}
	if _, exists := At(m, "future.enabled"); exists {
		t.Fatal("null did not delete")
	}
	if v, _ := At(m, "future.opaque"); v != "preserved" {
		t.Fatal("unknown future field lost")
	}
	rules := m["rules"].([]any)
	if len(rules) != 1 || rules[0] != "MATCH,REJECT" {
		t.Fatal("sequence was merged instead of replaced")
	}
	for _, f := range p.Fields {
		if f.Path == "tun.enable" {
			if len(f.Trace) != 2 || f.Source != "全局" || f.Trace[0].Line != 3 {
				t.Fatalf("trace %#v", f)
			}
		}
	}
	second, _, err := Compile(Layer{Name: "结果", Content: p.YAML})
	if err != nil || second.Hash != p.Hash {
		t.Fatal("configuration generation is not deterministic")
	}
}
func TestYAMLAnchorsAndInvalidDocuments(t *testing.T) {
	m, _, err := Parse("defaults: &base\n  enable: true\ntun:\n  <<: *base\n  stack: mixed\n")
	if err != nil || !BoolAt(m, "tun.enable") {
		t.Fatalf("anchor: %v", err)
	}
	for _, text := range []string{"key: 1\nkey: 2\n", "x: 1\n---\nx: 2\n", "- not-a-map\n", "x: [unterminated", "x: &loop\n  x: *loop\n"} {
		if _, _, err := Parse(text); err == nil {
			t.Fatalf("accepted invalid YAML: %q", text)
		}
	}
}
func TestObjectReplacementRetainsTrace(t *testing.T) {
	p, _, err := Compile(Layer{Name: "原文", Content: "custom:\n  nested: 1\n"}, Layer{Name: "覆盖", Content: "custom: literal\n"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Fields) != 1 || len(p.Fields[0].Trace) != 2 {
		t.Fatalf("missing replaced object: %#v", p.Fields)
	}
}
func TestRedaction(t *testing.T) {
	result := Redact("secret: abcdef\nproxies:\n- name: n\n  password: qwerty\n  uuid: sensitive\n")
	for _, secret := range []string{"abcdef", "qwerty", "sensitive"} {
		if strings.Contains(result, secret) {
			t.Fatal("secret leaked")
		}
	}
}
