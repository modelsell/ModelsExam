package openaicheck

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"sort"
	"strings"
	"sync"

	"model-check/common"
)

const (
	toolResultOrder = `{"order_id":"ORD-731","status":"shipped","total":42,"currency":"USD"}`
	promptInventory = "Find inventory for SKU-204 at warehouse north, excluding reserved stock. Use the appropriate tool with include_reserved set to false. The answer is not supplied in this prompt."
	promptOrder     = "Look up order ORD-731 with the provided tool. After receiving its result, return exactly one JSON object with the fields order_id, status, total, currency, using only values in the tool result."
	promptExtract   = "Extract this fictional order into exactly one JSON object with exactly the keys order_id (string), paid (boolean), quantity (integer), total (number). Order ORD-246 contains three identical items costing 12.50 each. It has been paid in full. Compute total and do not add any other fields."
)

type assertion struct {
	Name   string
	Passed bool
}

func failed(assertions []assertion) []string {
	var names []string
	for _, a := range assertions {
		if !a.Passed {
			names = append(names, a.Name)
		}
	}
	return names
}

func fn(name, description string, properties object, required ...string) object {
	return object{"type": "function", "function": object{"name": name, "description": description, "parameters": object{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}}
}

func toolInventory() object {
	return fn("lookup_inventory", "Return available stock for a SKU at a warehouse.", object{"sku": object{"type": "string"}, "warehouse": object{"type": "string"}, "include_reserved": object{"type": "boolean"}}, "sku", "warehouse", "include_reserved")
}

func toolWeather() object {
	return fn("get_weather", "Return a weather observation for a city.", object{"city": object{"type": "string"}}, "city")
}

func toolOrder() object {
	return fn("lookup_order", "Return the status and total of a fictional order.", object{"order_id": object{"type": "string"}}, "order_id")
}

// flatTool converts a Chat Completions tool into the Responses API shape,
// where name/description/parameters sit beside "type".
func flatTool(tool object) object {
	inner := obj(tool, "function")
	return object{"type": "function", "name": inner["name"], "description": inner["description"], "parameters": inner["parameters"]}
}

func schemaOrder() object {
	return object{"type": "object", "properties": object{"order_id": object{"type": "string"}, "paid": object{"type": "boolean"}, "quantity": object{"type": "integer"}, "total": object{"type": "number"}}, "required": []string{"order_id", "paid", "quantity", "total"}, "additionalProperties": false}
}

func expectedExtract() map[string]any {
	return map[string]any{"order_id": "ORD-246", "paid": true, "quantity": float64(3), "total": 37.5}
}

func expectedOrder() map[string]any {
	return map[string]any{"order_id": "ORD-731", "status": "shipped", "total": float64(42), "currency": "USD"}
}

// parseJSONObject parses the whole text as one JSON object. When allowFence is
// set a surrounding Markdown code fence is tolerated and reported.
func parseJSONObject(text string, allowFence bool) (value map[string]any, fenced bool, ok bool) {
	text = strings.TrimSpace(text)
	if common.Unmarshal([]byte(text), &value) == nil && value != nil {
		return value, false, true
	}
	if !allowFence || !strings.HasPrefix(text, "```") {
		return nil, false, false
	}
	inner := strings.TrimPrefix(text, "```")
	if i := strings.Index(inner, "\n"); i >= 0 {
		inner = inner[i+1:]
	}
	inner = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(inner), "```"))
	value = nil
	if common.Unmarshal([]byte(inner), &value) == nil && value != nil {
		return value, true, true
	}
	return nil, false, false
}

// gradeJSON compares typed values; key order and whitespace do not matter, while
// missing, extra and fabricated fields do.
func gradeJSON(text string, expected map[string]any, allowFence bool) ([]assertion, bool) {
	actual, fenced, valid := parseJSONObject(text, allowFence)
	exact := valid && len(actual) == len(expected)
	assertions := []assertion{{"json_object", valid}}
	names := make([]string, 0, len(expected))
	for key := range expected {
		names = append(names, key)
		if _, present := actual[key]; !present {
			exact = false
		}
	}
	sort.Strings(names)
	assertions = append(assertions, assertion{"exact_fields", exact})
	for _, key := range names {
		value, present := actual[key]
		assertions = append(assertions, assertion{"field_" + key, valid && present && reflect.DeepEqual(value, expected[key])})
	}
	return assertions, fenced
}

// gradeToolCall checks one assembled tool call against the expected function
// name and arguments, and the finish reason that should accompany it.
func gradeToolCall(v view, finishExpected, name string, input map[string]any) []assertion {
	var call toolCall
	if len(v.Tools) == 1 {
		call = v.Tools[0]
	}
	var args map[string]any
	_ = common.UnmarshalJsonStr(call.Arguments, &args)
	assertions := []assertion{
		{"one_tool_call", len(v.Tools) == 1},
		{"call_id", call.ID != ""},
		{"tool_name", call.Name == name},
		{"tool_arguments_json", args != nil},
		{"finish_reason", v.Finish == finishExpected},
	}
	if input != nil {
		assertions = append(assertions, assertion{"tool_arguments", reflect.DeepEqual(args, input)})
	}
	return assertions
}

var (
	imageOnce sync.Once
	imageData string
)

// redImageDataURL returns a small solid red PNG as a data URL. It is generated
// locally so the probe never depends on an external image host.
func redImageDataURL() string {
	imageOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		var buffer bytes.Buffer
		_ = png.Encode(&buffer, img)
		imageData = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
	})
	return imageData
}

// toolMessageCalls rebuilds the assistant tool_calls message from parsed calls,
// dropping provider-specific extras such as annotations that strict servers may
// refuse to accept as input.
func toolMessageCalls(calls []toolCall) object {
	items := make([]any, 0, len(calls))
	for _, c := range calls {
		items = append(items, object{"id": c.ID, "type": "function", "function": object{"name": c.Name, "arguments": c.Arguments}})
	}
	return object{"role": "assistant", "content": nil, "tool_calls": items}
}
