package geminicheck

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
	promptInventory = "Find inventory for SKU-204 at warehouse north, excluding reserved stock. Use the appropriate tool with include_reserved set to false. The answer is not supplied in this prompt."
	promptOrder     = "Look up order ORD-731 with the provided tool. After receiving its result, return exactly one JSON object with the fields order_id, status, total, currency, using only values in the tool result."
	promptExtract   = "Extract this fictional order into exactly one JSON object with exactly the keys order_id (string), paid (boolean), quantity (integer), total (number). Order ORD-246 contains three identical items costing 12.50 each. It has been paid in full. Compute total and do not add any other fields."
)

// Schema types use the proto enum names (STRING, OBJECT, ...) of the Gemini
// Schema object, the spelling every official reference example accepts.
func schema(kind string) object { return object{"type": kind} }

func declaration(name, description string, properties object, required ...string) object {
	return object{"name": name, "description": description, "parameters": object{"type": "OBJECT", "properties": properties, "required": required}}
}

func declInventory() object {
	return declaration("lookup_inventory", "Return available stock for a SKU at a warehouse.", object{"sku": schema("STRING"), "warehouse": schema("STRING"), "include_reserved": schema("BOOLEAN")}, "sku", "warehouse", "include_reserved")
}

func declWeather() object {
	return declaration("get_weather", "Return a weather observation for a city.", object{"city": schema("STRING")}, "city")
}

func declOrder() object {
	return declaration("lookup_order", "Return the status and total of a fictional order.", object{"order_id": schema("STRING")}, "order_id")
}

func inventoryArgs() map[string]any {
	return map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false}
}

func toolResultOrder() object {
	return object{"order_id": "ORD-731", "status": "shipped", "total": 42, "currency": "USD"}
}

func schemaOrder() object {
	return object{
		"type":             "OBJECT",
		"properties":       object{"order_id": schema("STRING"), "paid": schema("BOOLEAN"), "quantity": schema("INTEGER"), "total": schema("NUMBER")},
		"required":         []string{"order_id", "paid", "quantity", "total"},
		"propertyOrdering": []string{"order_id", "paid", "quantity", "total"},
	}
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

var (
	imageOnce sync.Once
	imageData string
)

// redImageBase64 returns a small solid red PNG, generated locally so the probe
// never depends on an external image host.
func redImageBase64() string {
	imageOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		var buffer bytes.Buffer
		_ = png.Encode(&buffer, img)
		imageData = base64.StdEncoding.EncodeToString(buffer.Bytes())
	})
	return imageData
}
