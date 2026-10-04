package runtime

import (
	"regexp"
	"strings"
)

// Go port of Comp's deterministic policy template processor
// (apps/api/src/trigger/policies/process-policy-template.ts). Comp's policy
// templates are TipTap JSON with {{PLACEHOLDER}} values and {{#if flag}}
// … {{/if}} blocks keyed on the organization's frameworks; blocks may sit
// inside one text node or span sibling nodes.

type jsonNode = map[string]any

var (
	placeholderRe = regexp.MustCompile(`\{\{(\w+)\}\}`)
	inlineIfRe    = regexp.MustCompile(`(?s)\{\{#if\s+(\w+)\}\}(.*?)\{\{/if\}\}`)
	openIfRe      = regexp.MustCompile(`\{\{#if\s+(\w+)\}\}`)
	onlyOpenRe    = regexp.MustCompile(`^\s*\{\{#if\s+\w+\}\}\s*$`)
	onlyCloseRe   = regexp.MustCompile(`^\s*\{\{/if\}\}\s*$`)
	closeMarkerRe = regexp.MustCompile(`\{\{/if\}\}`)
	// Comp's templates name its own product as the system of record; here
	// that system is Black Fortress.
	productNameRe  = regexp.MustCompile(`\bComp AI\b`)
	frameworkFlags = []struct {
		flag  string
		match func(name string) bool
	}{
		{"soc2", func(n string) bool { return regexp.MustCompile(`soc\s*2`).MatchString(n) || strings.Contains(n, "soc") }},
		{"hipaa", func(n string) bool { return strings.Contains(n, "hipaa") }},
		{"pipeda", func(n string) bool { return strings.Contains(n, "pipeda") }},
		{"gdpr", func(n string) bool { return strings.Contains(n, "gdpr") }},
		{"iso27001", func(n string) bool { return regexp.MustCompile(`iso\s*27001`).MatchString(n) }},
		{"pci", func(n string) bool { return strings.Contains(n, "pci") }},
		{"nist", func(n string) bool { return strings.Contains(n, "nist") }},
		{"ccpa", func(n string) bool { return strings.Contains(n, "ccpa") }},
		{"nen7510", func(n string) bool { return regexp.MustCompile(`nen\s*7510`).MatchString(n) }},
	}
)

// frameworkTemplateFlags turns the organization's framework names into the
// {{#if}} flags Comp's templates use.
func frameworkTemplateFlags(names []string) map[string]bool {
	flags := map[string]bool{}

	for _, f := range frameworkFlags {
		for _, n := range names {
			if f.match(strings.ToLower(n)) {
				flags[f.flag] = true
			}
		}
	}

	return flags
}

func extractText(n jsonNode) string {
	if t, ok := n["text"].(string); ok {
		return t
	}

	children, _ := n["content"].([]any)

	var b strings.Builder

	for _, c := range children {
		if cn, ok := c.(jsonNode); ok {
			b.WriteString(extractText(cn))
		}
	}

	return b.String()
}

func stripMarker(n jsonNode, marker *regexp.Regexp) jsonNode {
	out := jsonNode{}
	for k, v := range n {
		out[k] = v
	}

	if t, ok := n["text"].(string); ok {
		out["text"] = marker.ReplaceAllString(t, "")
		return out
	}

	if children, ok := n["content"].([]any); ok {
		next := make([]any, 0, len(children))

		for _, c := range children {
			if cn, ok := c.(jsonNode); ok {
				next = append(next, stripMarker(cn, marker))
			} else {
				next = append(next, c)
			}
		}

		out["content"] = next
	}

	return out
}

type templateContext struct {
	vars    map[string]string
	flags   map[string]bool
	missing func(key string) string
}

func (tc templateContext) processText(n jsonNode) jsonNode {
	text, _ := n["text"].(string)

	text = inlineIfRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := inlineIfRe.FindStringSubmatch(m)
		if tc.flags[parts[1]] {
			return parts[2]
		}

		return ""
	})
	text = placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		key := placeholderRe.FindStringSubmatch(m)[1]
		if v, ok := tc.vars[key]; ok && v != "" {
			return v
		}

		return tc.missing(key)
	})

	text = productNameRe.ReplaceAllString(text, "Black Fortress")

	if text == "" {
		return nil
	}

	out := jsonNode{}
	for k, v := range n {
		out[k] = v
	}

	out["text"] = text

	return out
}

func (tc templateContext) processNode(n jsonNode) jsonNode {
	if n["type"] == "text" {
		return tc.processText(n)
	}

	children, ok := n["content"].([]any)
	if !ok {
		return n
	}

	processed := tc.processContent(children)
	if len(processed) == 0 && n["type"] != "doc" {
		return nil
	}

	out := jsonNode{}
	for k, v := range n {
		out[k] = v
	}

	out["content"] = processed

	return out
}

// processContent mirrors Comp's processContentArray, including how
// conditional blocks spanning sibling nodes nest.
func (tc templateContext) processContent(nodes []any) []any {
	result := []any{}
	skipDepth := 0

	keep := func(n jsonNode) {
		if p := tc.processNode(n); p != nil {
			result = append(result, p)
		}
	}

	for _, raw := range nodes {
		node, ok := raw.(jsonNode)
		if !ok {
			continue
		}

		text := extractText(node)
		open := openIfRe.FindStringSubmatch(text)
		closes := strings.Contains(text, "{{/if}}")
		onlyMarker := onlyOpenRe.MatchString(text) || onlyCloseRe.MatchString(text)

		switch {
		case open != nil && closes:
			if skipDepth == 0 {
				keep(node)
			}
		case open != nil:
			switch {
			case skipDepth > 0:
				skipDepth++
			case !tc.flags[open[1]]:
				skipDepth++
			case !onlyMarker:
				keep(stripMarker(node, openIfRe))
			}
		case closes:
			switch {
			case skipDepth > 0:
				skipDepth--
			case !onlyMarker:
				keep(stripMarker(node, closeMarkerRe))
			}
		case skipDepth == 0:
			keep(node)
		}
	}

	return result
}

// allowedAttrs lists the node attributes Probo's document schema keeps;
// TipTap extras such as textAlign are dropped.
var allowedAttrs = map[string][]string{
	"heading":     {"level"},
	"orderedList": {"start"},
}

func pruneAttrs(n jsonNode) {
	if attrs, ok := n["attrs"].(map[string]any); ok {
		keep := map[string]any{}

		for _, k := range allowedAttrs[n["type"].(string)] {
			if v, ok := attrs[k]; ok {
				keep[k] = v
			}
		}

		if len(keep) == 0 {
			delete(n, "attrs")
		} else {
			n["attrs"] = keep
		}
	}

	if children, ok := n["content"].([]any); ok {
		for _, c := range children {
			if cn, ok := c.(jsonNode); ok {
				pruneAttrs(cn)
			}
		}
	}
}

// renderPolicy returns a Probo-ready ProseMirror doc for one Comp policy
// template.
func renderPolicy(content any, vars map[string]string, flags map[string]bool) jsonNode {
	tc := templateContext{
		vars:  vars,
		flags: flags,
		missing: func(key string) string {
			return "[to be defined: " + strings.ToLower(key) + "]"
		},
	}

	var nodes []any

	switch c := content.(type) {
	case jsonNode:
		nodes, _ = c["content"].([]any)
	case []any:
		nodes = c
	}

	doc := jsonNode{"type": "doc", "content": tc.processContent(nodes)}
	pruneAttrs(doc)

	return doc
}
