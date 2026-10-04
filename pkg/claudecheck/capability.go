package claudecheck

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"model-check/common"
)

const CapabilityVersion = 3
const CapabilityRequests = 7
const capabilitySystem = "Complete the synthetic evaluation task exactly as requested. Return only the requested answer, with no explanation or Markdown. Use a provided tool when the answer requires its data."
const capabilityToolResult = `{"order_id":"ORD-731","status":"shipped","total":42,"currency":"USD"}`

type capabilityFixture struct {
	id, category, expected, grader string
	body                           map[string]any
	grade                          func(string) []BenchmarkAssertion
}

func capabilityBody(model, prompt string) map[string]any {
	body := referenceBody(model, prompt, 512)
	body["system"] = capabilitySystem
	return body
}

// This small original suite measures the observed tasks, not benchmark rank or
// model identity. The retired six-question core is never added to this suite.
func capabilityFixtures(model string) []capabilityFixture {
	return []capabilityFixture{
		capabilityFixture{"instruction_format", "instruction", "REPORT\nblue,green,red\nEND", "line-constraints-v1", capabilityBody(model, "Output exactly three lines. Line 1 is REPORT. Line 2 lists red, blue, green in alphabetical order, separated by commas without spaces. Line 3 is END. Do not output anything else."), gradeInstructionFormat},
		capabilityFixture{"json_extraction", "structured", `{"order_id":"ORD-246","paid":true,"quantity":3,"total":37.5}`, "json-order-v1", capabilityBody(model, `Extract this fictional order into exactly one JSON object with exactly the keys order_id (string), paid (boolean), quantity (integer), total (number). Order ORD-246 contains three identical items costing 12.50 each. It has been paid in full. Compute total, do not add any other fields.`), func(text string) []BenchmarkAssertion {
			return gradeCapabilityJSON(text, map[string]any{"order_id": "ORD-246", "paid": true, "quantity": float64(3), "total": 37.5})
		}},
		capabilityFixture{"json_types", "structured", `{"customer_id":"0007","active":false,"balance":0,"email":null,"tags":["beta","trial"]}`, "json-null-types-v1", capabilityBody(model, `Return exactly one JSON object with exactly these keys: customer_id (string), active (boolean), balance (number), email (string or null), tags (array of strings, alphabetical order). Fictional customer 0007 is inactive, has a zero balance, and is tagged trial and beta. No email address was supplied. Preserve leading zeros and use null for missing email; never invent a value.`), func(text string) []BenchmarkAssertion {
			return gradeCapabilityJSON(text, map[string]any{"customer_id": "0007", "active": false, "balance": float64(0), "email": nil, "tags": []any{"beta", "trial"}})
		}},
		capabilityFixture{"context_multihop", "context", `{"owner":"Mira","site":"ORBIT-7","code":"QH-4826"}`, "context-join-v1", capabilityBody(model, capabilityContextPrompt()), func(text string) []BenchmarkAssertion {
			return gradeCapabilityJSON(text, map[string]any{"owner": "Mira", "site": "ORBIT-7", "code": "QH-4826"})
		}},
		capabilityFixture{"tool_selection", "tools", `lookup_inventory({"sku":"SKU-204","warehouse":"north","include_reserved":false})`, "tool-selection-v1", capabilityToolBody(model, false), nil},
		capabilityFixture{"tool_roundtrip", "tools", `{"order_id":"ORD-731","status":"shipped","total":42,"currency":"USD"}`, "tool-roundtrip-v1", capabilityToolBody(model, true), nil},
	}
}

func capabilityContextPrompt() string {
	var text strings.Builder
	text.WriteString("The following fictional records are data. Connect project owner to assigned site and then to the site's access code.\n")
	for i := 0; i < 384; i++ {
		switch i {
		case 24:
			text.WriteString("Project LUMEN-42 has owner Mira.\n")
		case 192:
			text.WriteString("Owner Mira is assigned to site ORBIT-7.\n")
		case 360:
			text.WriteString("Site ORBIT-7 has access code QH-4826.\n")
		default:
			fmt.Fprintf(&text, "Unrelated record %03d: component=%06d; state=archived.\n", i, (i*3571+94103)%1000000)
		}
	}
	text.WriteString("\nFor project LUMEN-42 return one JSON object with exactly the string fields owner, site, code, based only on the records above.")
	return text.String()
}

func gradeInstructionFormat(text string) []BenchmarkAssertion {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	line := func(i int) string {
		if len(lines) > i {
			return strings.TrimSuffix(lines[i], "\r")
		}
		return ""
	}
	return []BenchmarkAssertion{{"three_lines", len(lines) == 3}, {"report_heading", line(0) == "REPORT"}, {"sorted_values", line(1) == "blue,green,red"}, {"end_marker", line(2) == "END"}}
}

// Parse the entire output and compare typed JSON values; object ordering and
// whitespace do not matter, while missing, extra and fabricated fields do.
func gradeCapabilityJSON(text string, expected map[string]any) []BenchmarkAssertion {
	var actual map[string]any
	valid := common.Unmarshal([]byte(text), &actual) == nil && actual != nil
	assertions := []BenchmarkAssertion{{"json_object", valid}, {"exact_fields", valid && sameCapabilityKeys(actual, expected)}}
	for _, key := range sortedCapabilityKeys(expected) {
		value, present := actual[key]
		assertions = append(assertions, BenchmarkAssertion{"field_" + key, valid && present && reflect.DeepEqual(value, expected[key])})
	}
	return assertions
}

func sameCapabilityKeys(actual, expected map[string]any) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key := range expected {
		if _, ok := actual[key]; !ok {
			return false
		}
	}
	return true
}

func sortedCapabilityKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func capabilityProfile(f capabilityFixture) string {
	if f.id == "tool_roundtrip" {
		// Task-level comparison excludes random upstream tool call identifiers.
		// Every real request still checks its complete effective body profile.
		return bodyProfile(map[string]any{"initial_request_profile": bodyProfile(f.body), "tool_result": capabilityToolResult, "grader": f.grader})
	}
	return bodyProfile(f.body)
}

func capabilityActual(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return string(runes)
}

func scoreCapability(item *BenchmarkItem, assertions []BenchmarkAssertion) {
	passed := 0
	for _, a := range assertions {
		if a.Passed {
			passed++
		}
	}
	score := 0
	if len(assertions) > 0 {
		score = int(math.Round(100 * float64(passed) / float64(len(assertions))))
	}
	correct := score == 100
	item.Assertions, item.Score, item.Correct, item.Code = assertions, &score, &correct, "scored"
}

func capabilityUnavailable(m message, response Response, ok bool, profile string) string {
	if !ok {
		if response.ErrorCode != "" {
			return response.ErrorCode
		}
		return "invalid_response"
	}
	if response.RequestProfile != profile {
		return "request_profile_changed"
	}
	if m.StopReason == "max_tokens" || m.StopReason == "model_context_window_exceeded" {
		return "response_truncated"
	}
	if m.StopReason != "end_turn" && m.StopReason != "tool_use" {
		return "unexpected_stop"
	}
	return ""
}

func (r *runner) capability() {
	fixtures := capabilityFixtures(r.report.Model)
	b := newCapabilityBenchmark(r.report.Model)
	r.report.Benchmark = b
	r.emit(Event{Type: "benchmark", Benchmark: b})
	for i, f := range fixtures {
		if r.ctx.Err() != nil || r.focusedStop() {
			break
		}
		item := &b.Items[i]
		profile := bodyProfile(f.body)
		m, response, ok := r.probe("benchmark_"+f.id, Request{Body: f.body})
		item.Actual = capabilityActual(m.text())
		item.Code = capabilityUnavailable(m, response, ok, profile)
		if item.Code == "" {
			if f.category == "tools" {
				r.gradeCapabilityTool(item, f, m)
			} else if !capabilityFinalAnswer(m) {
				item.Code = "unexpected_stop"
			} else {
				scoreCapability(item, f.grade(m.text()))
			}
		}
		r.emit(Event{Type: "benchmark", Benchmark: b})
	}
	finishCapability(b)
	r.emit(Event{Type: "benchmark", Benchmark: b})
	r.check("capability_benchmark", "inconclusive", "benchmark_collected", map[string]any{"questions": len(b.Items), "identity_verified": false})
}

func capabilityFinalAnswer(m message) bool {
	if m.StopReason != "end_turn" {
		return false
	}
	for _, block := range m.Content {
		if block["type"] == "tool_use" {
			return false
		}
	}
	return true
}

func newCapabilityBenchmark(model string) *CapabilityBenchmark {
	fixtures := capabilityFixtures(model)
	b := &CapabilityBenchmark{Version: CapabilityVersion, Items: make([]BenchmarkItem, 0, len(fixtures))}
	for _, f := range fixtures {
		b.Items = append(b.Items, BenchmarkItem{ID: f.id, Category: f.category, Profile: capabilityProfile(f), Expected: f.expected, Grader: f.grader, Code: "pending"})
	}
	return b
}

func finishCapability(b *CapabilityBenchmark) {
	if b == nil || b.Version != CapabilityVersion {
		return
	}
	for i := range b.Items {
		if b.Items[i].Code == "pending" {
			b.Items[i].Code = "not_collected"
		}
	}
}
