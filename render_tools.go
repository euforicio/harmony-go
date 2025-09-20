package harmony

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// renderDeveloperContent renders developer instructions and the tools section directly into tokens.
func (e *Encoding) renderDeveloperContent(dev DeveloperContent, out *[]uint32) {
	body := e.acquireBuilder()
	// Pre-size builder to reduce growth churn
	if sz := estimateDeveloperContentSize(&dev); sz > 0 {
		if sz > 1<<18 {
			sz = 1 << 18
		}
		body.Grow(sz*2 + 128)
	}
	if dev.Instructions != nil && *dev.Instructions != "" {
		body.WriteString("# Instructions\n\n")
		body.WriteString(*dev.Instructions)
	}
	if len(dev.Tools) > 0 {
		if body.Len() > 0 {
			body.WriteString("\n\n")
		}
		e.writeToolsSection(body, dev.Tools)
	}
	e.renderText(body.String(), out)
	e.releaseBuilder(body)
}

// writeToolsSection renders tool namespaces and their tools in a TypeScript-like
// schema description used by Harmony prompts.
func (e *Encoding) writeToolsSection(body *strings.Builder, tools map[string]ToolNamespaceConfig) {
	if len(tools) == 0 {
		return
	}

	names := make([]string, 0, len(tools))
	for n := range tools {
		names = append(names, n)
	}
	sort.Strings(names)

	body.WriteString("# Tools")
	for _, nsName := range names {
		body.WriteString("\n\n")
		ns := tools[nsName]
		buf := e.acquireBuffer()
		buf.WriteString("## ")
		buf.WriteString(ns.Name)
		buf.WriteString("\n\n")
		if ns.Description != nil && *ns.Description != "" {
			if len(ns.Tools) > 0 {
				// write each line as comment without allocating a []string
				writeCommentLines(buf, *ns.Description)
			} else {
				buf.WriteString(*ns.Description)
				buf.WriteString("\n\n")
			}
		}
		if len(ns.Tools) > 0 {
			buf.WriteString("namespace ")
			buf.WriteString(ns.Name)
			buf.WriteString(" {\n\n")
			for idx := range ns.Tools {
				tool := &ns.Tools[idx]
				writeCommentLines(buf, tool.Description)
				if len(tool.Parameters) == 0 {
					fmt.Fprintf(buf, "type %s = () => any;\n\n", tool.Name)
				} else {
					schema, ordered, err := tool.parsedParameters()
					if err != nil || schema == nil {
						buf.WriteString("type ")
						buf.WriteString(tool.Name)
						buf.WriteString(" = (_: any) => any;\n\n")
					} else {
						rootDesc := ""
						if m, ok := schema.(map[string]any); ok {
							if d, ok := m["description"].(string); ok && d != "" {
								rootDesc = d
							}
						}
						buf.WriteString("type ")
						buf.WriteString(tool.Name)
						buf.WriteString(" = (_:")
						if rootDesc != "" {
							fmt.Fprintf(buf, " // %s\n{", rootDesc)
						} else {
							fmt.Fprintf(buf, " {")
						}
						e.renderSchemaObjectWithOrder(buf, schema, "\n", ordered)
						buf.WriteString("\n}) => any;\n\n")
					}
				}
				// spacing handled by previous WriteString; no extra work
			}
			buf.WriteString("} // namespace ")
			buf.WriteString(ns.Name)
		}
		body.WriteString(strings.TrimRight(buf.String(), "\n"))
		e.releaseBuffer(buf)
	}
}

// writeToolsSectionStream was removed (unused) to satisfy linters.

func (t *ToolDescription) parsedParameters() (any, []string, error) {
	if t == nil || len(t.Parameters) == 0 {
		return nil, nil, nil
	}
	if t.parsed == nil {
		t.parsed = &toolParsedCache{}
	}
	t.parsed.once.Do(func() {
		var v any
		if err := json.Unmarshal(t.Parameters, &v); err != nil {
			t.parsed.err = err
			return
		}
		t.parsed.value = v
		t.parsed.orderedKeys = orderedPropertyKeys(t.Parameters)
	})
	return t.parsed.value, t.parsed.orderedKeys, t.parsed.err
}

// writeCommentLines writes text as comment lines (prefix "// ") efficiently
// without allocating a slice of lines.
func writeCommentLines(buf *bytes.Buffer, text string) {
	start := 0
	for start <= len(text) {
		i := strings.IndexByte(text[start:], '\n')
		if i < 0 {
			// last line (may be empty)
			line := text[start:]
			fmt.Fprintf(buf, "// %s\n", line)
			break
		}
		line := text[start : start+i]
		fmt.Fprintf(buf, "// %s\n", line)
		start += i + 1
	}
}

// toolParsedCache holds memoized parsing state for ToolDescription.Parameters.
// It is reachable only through a pointer from ToolDescription so that copying
// ToolDescription values does not copy synchronization primitives.
type toolParsedCache struct {
	once        sync.Once
	value       any
	err         error
	orderedKeys []string
}

// renderSchemaObject expects a JSON object schema with optional properties/required/oneOf
// renderSchemaObject wrapper removed (unused) to satisfy linters

// renderSchemaObjectWithOrder renders a JSON Schema object and, when provided,
// uses the given key order for the immediate properties object.
func (e *Encoding) renderSchemaObjectWithOrder(buf *bytes.Buffer, schema any, indent string, orderedKeys []string) {
	m, _ := schema.(map[string]any)
	// Render properties
	props, _ := m["properties"].(map[string]any)
	var requiredSet map[string]struct{}
	if reqArr, ok := m["required"].([]any); ok {
		requiredSet = make(map[string]struct{}, len(reqArr))
		for _, r := range reqArr {
			if s, ok := r.(string); ok {
				requiredSet[s] = struct{}{}
			}
		}
	} else {
		requiredSet = map[string]struct{}{}
	}
	// property order: respect provided order if present, otherwise sort by name
	var keys []string
	if len(orderedKeys) > 0 {
		keys = append(keys, orderedKeys...)
		// include any missing keys (defensive)
		inSet := make(map[string]struct{}, len(keys))
		for _, k := range keys {
			inSet[k] = struct{}{}
		}
		for k := range props {
			if _, ok := inSet[k]; !ok {
				keys = append(keys, k)
			}
		}
	} else {
		keys = make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	for _, key := range keys {
		val := props[key]
		// Property-level comments
		// Title
		if title, ok := getString(val, "title"); ok && title != "" {
			fmt.Fprintf(buf, "%s// %s", indent, title)
			fmt.Fprintf(buf, "%s//", indent)
		}
		// Description and examples
		if desc, ok := getString(val, "description"); ok && desc != "" {
			for _, line := range strings.Split(desc, "\n") {
				fmt.Fprintf(buf, "%s// %s", indent, line)
			}
		}
		if exsv, ok := mget(val, "examples"); ok {
			if exs, ok := exsv.([]any); ok && len(exs) > 0 {
				fmt.Fprintf(buf, "%s// Examples:", indent)
				for _, ex := range exs {
					fmt.Fprintf(buf, "%s// - %s", indent, stringifyLiteral(ex))
				}
			}
		}

		// If oneOf
		if ov, ok := mget(val, "oneOf"); ok {
			if oneOf, ok2 := ov.([]any); ok2 && len(oneOf) > 0 {
				// Property-level default comment (above variants)
				if def, ok := mget(val, "default"); ok {
					fmt.Fprintf(buf, "%s// default: %s", indent, defaultCommentLiteral(val, def))
				}
				// Property name line ending with ':'
				fmt.Fprintf(buf, "%s%s", indent, key)
				if _, ok := requiredSet[key]; !ok {
					fmt.Fprint(buf, "?")
				}
				fmt.Fprint(buf, ":")

				propDesc, _ := getString(val, "description")
				for i, variant := range oneOf {
					fmt.Fprintf(buf, "%s | %s", indent, e.schemaToTS(variant, indent+"   "))
					// inline comments for variant description/default if present
					var trailing []string
					if d, ok := getString(variant, "description"); ok && d != "" {
						// avoid duplicating property-level description on first variant
						if !(i == 0 && propDesc != "" && d == propDesc) {
							trailing = append(trailing, d)
						}
					}
					if def, ok := mget(variant, "default"); ok {
						trailing = append(trailing, "default: "+defaultCommentLiteral(variant, def))
					}
					if len(trailing) > 0 {
						fmt.Fprintf(buf, " // %s", strings.Join(trailing, " "))
					}
					_ = i
				}
				fmt.Fprintf(buf, "%s,", indent)
				continue
			}
		}

		// Property line (normal path)
		fmt.Fprintf(buf, "%s%s", indent, key)
		if _, ok := requiredSet[key]; !ok {
			fmt.Fprint(buf, "?")
		}
		fmt.Fprint(buf, ": ")

		// Nullable
		nullable := false
		if nbv, ok := mget(val, "nullable"); ok {
			if nb, ok2 := nbv.(bool); ok2 {
				nullable = nb
			}
		}

		// Normal type
		ts := e.schemaToTS(val, indent+"    ")
		if nullable && !strings.Contains(ts, "null") {
			ts += " | null"
		}
		fmt.Fprint(buf, ts)
		// Default inline comment if present
		if def, ok := mget(val, "default"); ok {
			fmt.Fprintf(buf, ", // default: %s", defaultCommentLiteral(val, def))
		} else {
			fmt.Fprint(buf, ",")
		}
	}
}

func (e *Encoding) schemaToTS(schema any, indent string) string {
	// Handle map schema
	if m, ok := schema.(map[string]any); ok {
		// type as string or array
		if t, ok := m["type"].(string); ok {
			switch t {
			case "object":
				buf := e.acquireBuffer()
				buf.WriteString("{")
				e.renderSchemaObjectWithOrder(buf, m, indent, nil)
				buf.WriteString("\n")
				buf.WriteString(indent[:len(indent)-1]) // approximate outdent for closing brace
				buf.WriteString("}")
				return e.bufferStringAndRelease(buf)
			case "string":
				// enum
				if arr, ok := m["enum"].([]any); ok && len(arr) > 0 {
					vals := make([]string, 0, len(arr))
					for _, v := range arr {
						vals = append(vals, fmt.Sprintf("\"%s\"", fmt.Sprint(v)))
					}
					return strings.Join(vals, " | ")
				}
				return "string"
			case "number", "integer":
				return "number"
			case "boolean":
				return "boolean"
			case "array":
				if items, ok := m["items"]; ok {
					return e.schemaToTS(items, indent) + "[]"
				}
				return "Array<any>"
			}
		}
		if arr, ok := m["type"].([]any); ok && len(arr) > 0 {
			vals := make([]string, 0, len(arr))
			for _, v := range arr {
				vs := fmt.Sprint(v)
				switch vs {
				case "integer":
					vs = "number"
				}
				vals = append(vals, vs)
			}
			return strings.Join(vals, " | ")
		}
		if oneOf, ok := m["oneOf"].([]any); ok && len(oneOf) > 0 {
			types := make([]string, 0, len(oneOf))
			for _, v := range oneOf {
				types = append(types, e.schemaToTS(v, indent))
			}
			return strings.Join(types, " | ")
		}
		return "any"
	}
	return "any"
}

// ----- Utilities -----
func getString(v any, key string) (string, bool) {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m[key].(string); ok {
			return s, true
		}
	}
	return "", false
}

func mget(v any, key string) (any, bool) {
	if m, ok := v.(map[string]any); ok {
		val, ok := m[key]
		return val, ok
	}
	return nil, false
}

func stringifyLiteral(v any) string {
	switch t := v.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(t)
	}
}

// stringifyDefault formats default values for comments without extra quotes
// around strings to match canonical output.
// stringifyDefault removed (unused) to satisfy linters

func isEnum(schema any) bool {
	if m, ok := schema.(map[string]any); ok {
		if arr, ok := m["enum"].([]any); ok {
			return len(arr) > 0
		}
	}
	return false
}

func defaultCommentLiteral(schema any, def any) string {
	switch t := def.(type) {
	case string:
		if isEnum(schema) {
			return t
		}
		return stringifyLiteral(t)
	default:
		return stringifyLiteral(def)
	}
}

// orderedPropertyKeys extracts the key order from the "properties" object in a schema JSON blob.
func orderedPropertyKeys(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// read opening '{'
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	for dec.More() {
		// read key
		tk, err := dec.Token()
		if err != nil {
			return nil
		}
		key, _ := tk.(string)
		if key == "properties" {
			// next token should be '{'
			t2, err := dec.Token()
			if err != nil {
				return nil
			}
			if d2, ok := t2.(json.Delim); !ok || d2 != '{' {
				return nil
			}
			var out []string
			for dec.More() {
				tk2, err := dec.Token()
				if err != nil {
					return out
				}
				k2, _ := tk2.(string)
				if k2 == "" {
					return out
				}
				out = append(out, k2)
				// skip value
				var skip any
				if err := dec.Decode(&skip); err != nil {
					return out
				}
			}
			// consume closing '}'
			_, _ = dec.Token()
			return out
		}
		// skip non-properties value
		var skip any
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	return nil
}
