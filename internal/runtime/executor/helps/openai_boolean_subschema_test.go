package helps

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// TestNormalizeOpenAIBooleanSubschemas verifies the openai -> openai gap from
// issue #6230: boolean JSON Schema subschemas in MCP tool declarations (e.g.
// Rancher's {"patch": {"type":"array","items":true}}) must be normalized before
// reaching strict upstream validators (Volcengine Ark 11133, codebuddyCN 400001),
// while `false` and `additionalProperties` booleans stay untouched.
func TestNormalizeOpenAIBooleanSubschemas(t *testing.T) {
	input := []byte(`{
		"model": "kimi-k3-256k",
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "mcp__rancher__executeChange",
					"description": "Applies a JSON patch",
					"parameters": {
						"type": "object",
						"properties": {
							"patch": {"type": "array", "items": true, "description": "RFC 6902 patch"},
							"anything": true,
							"disabled": false,
							"either": {"anyOf": [true, {"type": "string"}]},
							"nested": {
								"type": "object",
								"properties": {"flag": true},
								"additionalProperties": false
							}
						},
						"additionalProperties": false,
						"$defs": {"wildcard": true}
					}
				}
			},
			{
				"type": "function",
				"function": {
					"name": "clean_tool",
					"parameters": {
						"type": "object",
						"properties": {"q": {"type": "string"}},
						"additionalProperties": true
					}
				}
			}
		],
		"messages": [{"role": "user", "content": "hello"}]
	}`)

	out := NormalizeOpenAIBooleanSubschemas(input)
	got := gjson.ParseBytes(out)

	params := got.Get("tools.0.function.parameters")
	if !params.Exists() {
		t.Fatalf("parameters missing: %s", got.Raw)
	}

	// items: true -> {}
	if v := params.Get("properties.patch.items"); !v.Exists() || !v.IsObject() || v.Raw != "{}" {
		t.Fatalf("items boolean subschema should be {}, got %s", v.Raw)
	}
	// bare property true -> {}
	if v := params.Get("properties.anything"); !v.Exists() || !v.IsObject() || v.Raw != "{}" {
		t.Fatalf("boolean property subschema should be {}, got %s", v.Raw)
	}
	// anyOf branch true -> {}
	if v := params.Get("properties.either.anyOf.0"); !v.Exists() || !v.IsObject() || v.Raw != "{}" {
		t.Fatalf("boolean anyOf branch should be {}, got %s", v.Raw)
	}
	// $defs.wildcard true -> {}
	if v := params.Get("$defs.wildcard"); !v.Exists() || !v.IsObject() || v.Raw != "{}" {
		t.Fatalf("$defs boolean subschema should be {}, got %s", v.Raw)
	}
	// nested property true -> {}
	if v := params.Get("properties.nested.properties.flag"); !v.Exists() || !v.IsObject() || v.Raw != "{}" {
		t.Fatalf("nested boolean property should be {}, got %s", v.Raw)
	}
	// false subschema preserved
	if v := params.Get("properties.disabled"); v.Type != gjson.False {
		t.Fatalf("false subschema must be preserved, got %s", v.Raw)
	}
	// additionalProperties false preserved (structured outputs rely on it)
	if v := params.Get("properties.nested.additionalProperties"); v.Type != gjson.False {
		t.Fatalf("additionalProperties:false must be preserved, got %s", v.Raw)
	}
	if v := params.Get("additionalProperties"); v.Type != gjson.False {
		t.Fatalf("root additionalProperties:false must be preserved, got %s", v.Raw)
	}
	// second tool's additionalProperties:true must be preserved
	if v := got.Get("tools.1.function.parameters.additionalProperties"); v.Type != gjson.True {
		t.Fatalf("additionalProperties:true must be preserved, got %s", v.Raw)
	}
	// untouched tool body stays byte-identical
	if !strings.Contains(string(out), `"name":"clean_tool"`) && !strings.Contains(string(out), `"name": "clean_tool"`) {
		t.Fatalf("clean tool declaration lost: %s", got.Get("tools.1").Raw)
	}
	// no raw boolean subschema keywords survive in the first tool
	first := got.Get("tools.0.function.parameters").Raw
	if strings.Contains(first, `"items":true`) || strings.Contains(first, `"items": true`) {
		t.Fatalf("items:true survived normalization: %s", first)
	}
}

// TestNormalizeOpenAIBooleanSubschemas_NoTools ensures non-tool payloads are untouched.
func TestNormalizeOpenAIBooleanSubschemas_NoTools(t *testing.T) {
	input := []byte(`{"model":"kimi-k3-256k","messages":[{"role":"user","content":"hi"}]}`)
	out := NormalizeOpenAIBooleanSubschemas(input)
	if string(out) != string(input) {
		t.Fatalf("payload without tools should be unchanged:\n got: %s\nwant: %s", out, input)
	}
}

// TestNormalizeOpenAIBooleanSubschemas_Empty ensures empty input is returned as-is.
func TestNormalizeOpenAIBooleanSubschemas_Empty(t *testing.T) {
	if out := NormalizeOpenAIBooleanSubschemas(nil); out != nil {
		t.Fatalf("nil input should return nil, got %q", out)
	}
	if out := NormalizeOpenAIBooleanSubschemas([]byte{}); len(out) != 0 {
		t.Fatalf("empty input should return empty, got %q", out)
	}
}

// TestNormalizeOpenAIBooleanSubschemas_LargeNumbers ensures numeric literals
// survive normalization without float precision loss.
func TestNormalizeOpenAIBooleanSubschemas_LargeNumbers(t *testing.T) {
	input := []byte(`{"tools":[{"type":"function","function":{"name":"t","parameters":{"type":"object","properties":{"big":9007199254740993,"flag":true}}}}]}`)
	out := NormalizeOpenAIBooleanSubschemas(input)
	if !strings.Contains(string(out), "9007199254740993") {
		t.Fatalf("large integer corrupted by normalization: %s", out)
	}
}
