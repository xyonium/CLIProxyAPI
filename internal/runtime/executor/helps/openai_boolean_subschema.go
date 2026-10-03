package helps

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
)

// NormalizeOpenAIBooleanSubschemas rewrites JSON Schema boolean subschemas in
// OpenAI chat-completions tool parameter declarations so strict OpenAPI 3.0
// upstream validators (Volcengine Ark code 11133, codebuddyCN 400001) accept
// the request. The claude -> openai translator path was fixed upstream in
// 4b4a4448 (issue #6230), but requests that already arrive in OpenAI format
// (/v1/chat/completions from clients such as OpenWebUI harnesses) pass through
// the openai -> openai translator, which forwards the body verbatim — so MCP
// tools declaring boolean subschemas like {"items": true} still reach the
// upstream untouched and get rejected.
//
// Semantics mirror upstream 4b4a4448: boolean `true` (accept anything) becomes
// an empty object schema `{}`; boolean `false` is preserved; boolean
// `additionalProperties` values are preserved because OpenAI structured
// outputs require them.
func NormalizeOpenAIBooleanSubschemas(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	updatedTools, changed := normalizeOpenAIToolBooleanList(gjson.GetBytes(body, "tools"))
	if !changed {
		return body
	}
	out, errSet := sjson.SetRawBytes(body, "tools", updatedTools)
	if errSet != nil {
		return body
	}
	return out
}

// normalizeOpenAIToolBooleanList batches changed tools so unchanged declarations are
// copied verbatim, preserving array formatting. Mirrors normalizeCodexToolList.
func normalizeOpenAIToolBooleanList(tools gjson.Result) ([]byte, bool) {
	if !tools.IsArray() {
		return nil, false
	}
	var out []byte
	offset := 0
	tools.ForEach(func(_, tool gjson.Result) bool {
		params := tool.Get("function.parameters")
		if !params.Exists() {
			return true
		}
		normalized, changed := normalizeOpenAIBooleanSchemaJSON([]byte(params.Raw))
		if !changed {
			return true
		}
		updatedTool, errSet := sjson.SetRawBytes([]byte(tool.Raw), "function.parameters", normalized)
		if errSet != nil {
			return true
		}
		if out == nil {
			out = make([]byte, 0, len(tools.Raw))
		}
		start := tool.Index - tools.Index
		out = append(out, tools.Raw[offset:start]...)
		out = append(out, updatedTool...)
		offset = start + len(tool.Raw)
		return true
	})
	if out == nil {
		return nil, false
	}
	return append(out, tools.Raw[offset:]...), true
}

// normalizeOpenAIBooleanSchemaJSON normalizes boolean subschemas inside a single
// tool parameter schema, returning the original bytes when nothing changed. It
// decodes with UseNumber so numeric literals (e.g. large defaults) survive the
// round trip without float precision loss.
func normalizeOpenAIBooleanSchemaJSON(raw []byte) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, false
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil || root == nil {
		return raw, false
	}
	// Reject trailing garbage so partial JSON never reaches the upstream mutated.
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return raw, false
	}
	if !normalizeOpenAIBooleanSchemaValue(root) {
		return raw, false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return raw, false
	}
	return bytes.TrimSpace(buf.Bytes()), true
}

// normalizeOpenAIBooleanSchemaValue walks schema-bearing keyword locations and
// rewrites boolean subschemas in place. Only keys under known JSON Schema
// keyword positions are visited, so booleans inside user data (descriptions,
// defaults, enums) are never touched. Returns true when any node changed.
func normalizeOpenAIBooleanSchemaValue(v any) bool {
	changed := false
	switch schema := v.(type) {
	case map[string]any:
		for _, mapKey := range util.SchemaMapKeywords {
			subMap, ok := schema[mapKey].(map[string]any)
			if !ok {
				continue
			}
			for subKey, subSchema := range subMap {
				if boolSub, ok := subSchema.(bool); ok {
					if normalized, ok := normalizeOpenAIBooleanLeaf(mapKey, boolSub); ok {
						subMap[subKey] = normalized
						changed = true
					}
					continue
				}
				if normalizeOpenAIBooleanSchemaValue(subSchema) {
					changed = true
				}
			}
		}
		for _, valKey := range util.SchemaValueKeywords {
			val, exists := schema[valKey]
			if !exists {
				continue
			}
			switch sub := val.(type) {
			case bool:
				if normalized, ok := normalizeOpenAIBooleanLeaf(valKey, sub); ok {
					schema[valKey] = normalized
					changed = true
				}
			case map[string]any:
				if normalizeOpenAIBooleanSchemaValue(sub) {
					changed = true
				}
			case []any:
				for i, item := range sub {
					if boolItem, ok := item.(bool); ok {
						if normalized, ok := normalizeOpenAIBooleanLeaf(valKey, boolItem); ok {
							sub[i] = normalized
							changed = true
						}
						continue
					}
					if normalizeOpenAIBooleanSchemaValue(item) {
						changed = true
					}
				}
			}
		}
	case []any:
		for _, item := range schema {
			if normalizeOpenAIBooleanSchemaValue(item) {
				changed = true
			}
		}
	}
	return changed
}

// normalizeOpenAIBooleanLeaf converts a boolean subschema to its normalized form.
// `true` becomes `{}`; `false` and any `additionalProperties` boolean are left
// untouched (second return value false) to preserve OpenAI structured-output
// constraints and reject-all semantics.
func normalizeOpenAIBooleanLeaf(key string, value bool) (any, bool) {
	if key == "additionalProperties" {
		return nil, false
	}
	if value {
		return map[string]any{}, true
	}
	return nil, false
}
