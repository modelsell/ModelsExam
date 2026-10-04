package claudecheck

import (
	"strconv"
	"strings"

	"model-check/common"
)

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (r *runner) coldCache(usage Usage) {
	state := "inconclusive"
	if usage.CacheRead != nil && usage.CacheWrite != nil && *usage.CacheRead == 0 && *usage.CacheWrite == 0 {
		state = "pass"
	}
	r.check("cold_cache", state, "cold_cache_observation", map[string]any{"baseline_probe": r.report.BaselineProbe, "cache_read": usage.CacheRead, "cache_write": usage.CacheWrite})
}

// officialErrorType maps an HTTP status to the error.type documented at
// https://platform.claude.com/docs/en/api/errors. The API may use
// invalid_request_error for other 4xx statuses, so any unlisted 4xx accepts it.
func officialErrorType(status int, kind string) bool {
	known := map[int]string{
		400: "invalid_request_error", 401: "authentication_error", 402: "billing_error",
		403: "permission_error", 404: "not_found_error", 409: "conflict_error",
		413: "request_too_large", 429: "rate_limit_error", 500: "api_error",
		504: "timeout_error", 529: "overloaded_error",
	}
	if want, ok := known[status]; ok {
		return kind == want
	}
	if status >= 400 && status < 500 {
		return kind == "invalid_request_error"
	}
	return status >= 500 && kind == "api_error"
}

// errorShape checks the documented envelope: a top-level "type":"error", an
// "error" object with string "type" and "message", a top-level "request_id",
// and an error.type that agrees with the HTTP status. The check passes only
// when all of that holds; a gateway may legitimately normalize errors, so
// anything else stays inconclusive and never counts as a failure.
func (r *runner) errorShape(response Response) {
	var body map[string]any
	shape := "unknown"
	evidence := map[string]any{"http_status": response.Status}
	official := false
	if common.Unmarshal(response.Body, &body) == nil {
		if e, ok := body["error"].(map[string]any); ok {
			kind, _ := e["type"].(string)
			message, _ := e["message"].(string)
			requestID, _ := body["request_id"].(string)
			if body["type"] == "error" && kind != "" && message != "" {
				shape = "anthropic"
				matches := officialErrorType(response.Status, kind)
				evidence["error_type"] = kind
				evidence["type_matches_status"] = matches
				evidence["request_id_in_body"] = requestID != ""
				official = matches && requestID != ""
			} else {
				shape = "proxy_error"
			}
		} else if _, ok := body["message"].(string); ok {
			shape = "aws_or_proxy"
		}
	}
	if response.Header != nil {
		evidence["request_id_header"] = response.Header.Get("request-id") != ""
	}
	if r.report.Transport == "bedrock_runtime" && response.Status >= 400 {
		shape = "aws_sdk"
		official = false
	}
	evidence["shape"] = shape
	state := "inconclusive"
	if official {
		state = "pass"
	}
	// The configured protocol can legitimately normalize an error envelope.
	r.check("error_shape", state, "error_envelope", evidence)
}

func (r *runner) usageAudit() {
	inspected, missing, invalid, compared, mismatches := 0, 0, 0, 0, 0
	for _, sample := range r.report.Samples {
		if IsCountProbe(sample.Probe) || sample.Status != 200 {
			continue
		}
		inspected++
		u := sample.Usage
		if u.Input == nil || u.Output == nil {
			missing++
		}
		for _, n := range []*int64{u.Input, u.Output, u.CacheWrite, u.CacheRead} {
			if n != nil && *n < 0 {
				invalid++
			}
		}
		// Cached requests may use different header normalization. Compare only
		// observed uncached samples, never silently equate missing cache to zero.
		for name, value := range map[string]*int64{"x-amzn-bedrock-output-token-count": u.Output, "x-amzn-bedrock-input-token-count": u.Input} {
			if strings.Contains(name, "input") && (u.CacheWrite == nil || u.CacheRead == nil || *u.CacheWrite != 0 || *u.CacheRead != 0) {
				continue
			}
			if raw, exists := sample.Headers[name]; exists && value != nil {
				n, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || n < 0 {
					continue
				}
				compared++
				if n != *value {
					mismatches++
				}
			}
		}
	}
	state := "pass"
	if inspected == 0 || missing > 0 {
		state = "inconclusive"
	}
	if invalid > 0 {
		state = "fail"
	}
	r.check("usage_fields", state, "usage_observation", map[string]any{"samples": inspected, "missing": missing, "invalid": invalid})
	state = "inconclusive"
	if compared > 0 {
		state = status(mismatches == 0)
	}
	r.check("aws_usage", state, "aws_header_comparison", map[string]any{"compared_fields": compared, "mismatches": mismatches})
}

func (r *runner) repeatability() {
	body := r.body("Reply with exactly PONG.")
	body["stream"] = true
	ids, models, inputs := map[string]bool{}, map[string]bool{}, map[int64]bool{}
	attempts, success, matched := 0, 0, 0
	for _, id := range []string{"repeat_1", "repeat_2", "repeat_3"} {
		if r.ctx.Err() != nil {
			break
		}
		attempts++
		m, _, ok := r.probe(id, Request{Body: body})
		if !ok {
			continue
		}
		success++
		matched += boolInt(m.text() == "PONG")
		ids[m.ID], models[m.Model] = true, true
		if n, yes := totalInput(m.Usage); yes {
			inputs[n] = true
		}
	}
	state := "inconclusive"
	if attempts == 3 && success == 3 && matched == 3 && len(ids) == 3 && len(models) == 1 && len(inputs) == 1 {
		state = "pass"
	}
	if attempts > success {
		state = "fail"
	}
	r.check("repeatability", state, "repeat_sample", map[string]any{"attempts": attempts, "success": success, "matched": matched, "unique_ids": len(ids), "model_variants": len(models), "input_variants": len(inputs), "population_claim": false})
}

func (r *runner) thinkingStream() {
	if r.ctx.Err() != nil {
		return
	}
	body := r.body("Calculate 137 * 149 and reply with the result.")
	body["thinking"], body["stream"], body["max_tokens"] = r.thinkingMode(), true, 2048
	m, response, ok := r.probe("thinking_stream", Request{Body: body})
	signed := false
	for _, block := range m.Content {
		if signature, yes := block["signature"].(string); block["type"] == "thinking" && yes && signature != "" {
			signed = true
		}
		if data, yes := block["data"].(string); block["type"] == "redacted_thinking" && yes && data != "" {
			signed = true
		}
	}
	state := "inconclusive"
	if ok && signed {
		state = "pass"
	}
	if response.Status == 200 && !ok {
		state = "fail"
	}
	r.check("thinking_stream", state, "thinking_stream_observation", map[string]any{"lifecycle_valid": ok, "signed_block": signed, "http_status": response.Status})
}
