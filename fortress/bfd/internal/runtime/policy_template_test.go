package runtime

import (
	"encoding/json"
	"testing"
)

func para(text string) any {
	return map[string]any{"type": "paragraph", "attrs": map[string]any{"textAlign": "left"}, "content": []any{map[string]any{"type": "text", "text": text}}}
}

func texts(t *testing.T, doc jsonNode) []string {
	t.Helper()

	var out []string
	for _, c := range doc["content"].([]any) {
		out = append(out, extractText(c.(jsonNode)))
	}

	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// Cases mirror Comp's process-policy-template.test.ts.
func TestRenderPolicyTemplate(t *testing.T) {
	vars := map[string]string{"COMPANY": "Acme Inc", "EMPLOYEES": "50", "DATA": "PII"}
	soc2 := map[string]bool{"soc2": true}
	hipaa := map[string]bool{"soc2": true, "hipaa": true}

	cases := []struct {
		name  string
		nodes []any
		flags map[string]bool
		want  []string
	}{
		{"placeholders", []any{para("{{COMPANY}} has {{EMPLOYEES}} employees handling {{DATA}}")}, nil, []string{"Acme Inc has 50 employees handling PII"}},
		{"product name", []any{para("Record names in the Comp AI platform.")}, nil, []string{"Record names in the Black Fortress platform."}},
		{"unknown placeholder", []any{para("Contact {{SUPPORT}}")}, nil, []string{"Contact [to be defined: support]"}},
		{"inline true", []any{para("Before {{#if soc2}}SOC 2 content{{/if}} after")}, soc2, []string{"Before SOC 2 content after"}},
		{"inline false", []any{para("Before {{#if soc2}}SOC 2 content{{/if}} after")}, nil, []string{"Before  after"}},
		{"multiple inline", []any{para("{{#if soc2}}SOC2{{/if}} and {{#if hipaa}}HIPAA{{/if}}")}, soc2, []string{"SOC2 and "}},
		{"block true", []any{para("{{#if soc2}}"), para("SOC 2 specific content"), para("{{/if}}"), para("Always visible")}, soc2, []string{"SOC 2 specific content", "Always visible"}},
		{"block false", []any{para("{{#if soc2}}"), para("SOC 2 specific content"), para("{{/if}}"), para("Always visible")}, nil, []string{"Always visible"}},
		{"unknown flag", []any{para("{{#if unknown}}"), para("Hidden"), para("{{/if}}")}, soc2, nil},
		{"open marker with text", []any{para("{{#if soc2}} SOC 2 intro text"), para("More content"), para("{{/if}}")}, soc2, []string{" SOC 2 intro text", "More content"}},
		{"close marker with text", []any{para("{{#if soc2}}"), para("Content here"), para("End of section {{/if}}")}, soc2, []string{"Content here", "End of section "}},
		{"mixed false", []any{para("{{#if soc2}} intro"), para("body"), para("end {{/if}}")}, nil, nil},
		{"nested true/true", []any{para("{{#if soc2}}"), para("SOC 2 content"), para("{{#if hipaa}}"), para("SOC 2 + HIPAA content"), para("{{/if}}"), para("{{/if}}")}, hipaa, []string{"SOC 2 content", "SOC 2 + HIPAA content"}},
		{"nested true/false", []any{para("{{#if soc2}}"), para("SOC 2 only"), para("{{#if hipaa}}"), para("HIPAA"), para("{{/if}}"), para("Still SOC 2"), para("{{/if}}")}, soc2, []string{"SOC 2 only", "Still SOC 2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Round-trip through JSON so nodes have the types encoding/json produces.
			raw, _ := json.Marshal(map[string]any{"type": "doc", "content": tc.nodes})
			var content any
			_ = json.Unmarshal(raw, &content)

			doc := renderPolicy(content, vars, tc.flags)
			if got := texts(t, doc); !equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderPolicyPrunesAttrs(t *testing.T) {
	raw := `{"type":"doc","content":[{"type":"heading","attrs":{"level":2,"textAlign":"left"},"content":[{"type":"text","text":"Scope"}]},{"type":"paragraph","attrs":{"textAlign":"left"},"content":[{"type":"text","text":"x"}]}]}`

	var content any
	_ = json.Unmarshal([]byte(raw), &content)

	doc := renderPolicy(content, nil, nil)
	nodes := doc["content"].([]any)

	if attrs := nodes[0].(jsonNode)["attrs"].(map[string]any); len(attrs) != 1 || attrs["level"] != float64(2) {
		t.Fatalf("heading attrs = %v", attrs)
	}

	if _, ok := nodes[1].(jsonNode)["attrs"]; ok {
		t.Fatal("paragraph textAlign not pruned")
	}
}

func TestFrameworkTemplateFlags(t *testing.T) {
	flags := frameworkTemplateFlags([]string{"SOC 2", "ISO 27001 (2022)", "GDPR"})
	if !flags["soc2"] || !flags["iso27001"] || !flags["gdpr"] || flags["hipaa"] {
		t.Fatalf("flags = %v", flags)
	}
}
