package claudecheck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"model-check/common"
	"github.com/google/uuid"
)

type runner struct {
	ctx           context.Context
	call          Transport
	report        Report
	upstreamModel string
	observe       func(Event)
}

// Run performs up to 36 focused requests, or 45 for the legacy suite.
// A transient baseline problem can use
// the already planned stream probe to recover. Requests are never retried.
func Run(ctx context.Context, options Options, transport Transport) Report {
	return RunWithObserver(ctx, options, transport, nil)
}

func RunWithObserver(ctx context.Context, options Options, transport Transport, observe func(Event)) Report {
	if options.Suite == "focused" {
		return runFocused(ctx, options.normalized(), transport, observe)
	}
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()
	start := time.Now()
	options = options.normalized()
	r := &runner{ctx: ctx, call: transport, observe: observe, report: Report{Version: 8, ID: uuid.NewString(), Model: options.Model, StartedAt: start.UTC().Format(time.RFC3339), Checks: []Check{}, Samples: []Sample{}, Summary: map[string]int{}}}
	r.report.Limits = &RunLimits{ProbeTimeoutSeconds: int(ProbeTimeout / time.Second), RunTimeoutSeconds: int(RunTimeout / time.Second), MaxRequests: MaxRequests(options)}
	r.report.Options = &options
	r.report.Plan = Plan(options)
	if options.PromptAudit || options.Cache {
		r.report.TokenAudit = &TokenAuditReport{Version: 1, Prompt: []TokenComparison{}, Cache: []TokenComparison{}}
		if options.PromptAudit {
			for _, fixture := range promptAuditFixtures(options.Model) {
				r.report.TokenAudit.Prompt = append(r.report.TokenAudit.Prompt, TokenComparison{ID: fixture.id, Code: "pending"})
			}
		}
		if options.Cache {
			for _, id := range []string{"cache_read_1", "cache_read_2"} {
				r.report.TokenAudit.Cache = append(r.report.TokenAudit.Cache, TokenComparison{ID: id, Code: "pending"})
			}
		}
	}
	r.emit(Event{Type: "start", Report: &r.report})
	if r.establishBaseline() {
		r.behavior()
		if options.Vision {
			r.vision()
		}
		if options.PDF {
			r.document()
		}
		if options.Thinking {
			r.thinking()
			r.thinkingStream()
		} else {
			r.check("thinking", "skipped", "not_requested", nil)
			r.check("signature_replay", "skipped", "not_requested", nil)
		}
		if options.Cache {
			r.cache()
		} else {
			r.check("cache", "skipped", "not_requested", nil)
		}
		if options.Repeat {
			r.repeatability()
		}
		if options.StreamComparison {
			r.streamComparison()
		}
		if options.PromptAudit {
			r.promptTokenAudit()
		}
		r.usageAudit()
		if options.Benchmark {
			r.benchmark()
		}
		if options.Performance {
			r.performance()
		}
		if options.Bedrock {
			r.bedrock()
		}
	}
	r.modelConsistency()
	// Resolve the exact persisted plan, including disabled capabilities and
	// dependencies. Newer clients must not invent results for older reports.
	for _, item := range r.report.Plan {
		found := false
		for _, check := range r.report.Checks {
			found = found || check.ID == item.ID
		}
		if found || item.ID == "billing" || item.ID == "prompt_integrity" {
			continue
		}
		code := "baseline_failed"
		if r.report.StopReason != "" {
			code = "run_stopped"
		}
		if !item.Selected {
			code = "not_requested"
		} else if r.ctx.Err() != nil {
			code = "cancelled"
		}
		r.check(item.ID, "skipped", code, nil)
	}

	// An upstream count endpoint is not an independent billing authority and
	// low token counts cannot prove the absence of injected system instructions.
	r.check("billing", "inconclusive", "reported_usage_only", nil)
	if !r.hasCheck("prompt_integrity") {
		r.check("prompt_integrity", "inconclusive", "no_trusted_baseline", nil)
	}
	r.report.Cancelled = ctx.Err() != nil
	if ctx.Err() == context.DeadlineExceeded {
		r.report.StopReason = "run_timeout"
	} else if ctx.Err() != nil {
		r.report.StopReason = "cancelled"
	}
	if r.report.StopReason != "" && len(r.report.Samples) > 0 {
		r.report.StopProbe = r.report.Samples[len(r.report.Samples)-1].Probe
	}
	if r.report.TokenAudit != nil {
		for _, items := range [][]TokenComparison{r.report.TokenAudit.Prompt, r.report.TokenAudit.Cache} {
			for i := range items {
				if items[i].Code == "pending" {
					items[i].Code = "not_collected"
				}
			}
		}
	}
	r.report.DurationMS = time.Since(start).Milliseconds()
	r.report.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, c := range r.report.Checks {
		r.report.Summary[c.Status]++
	}
	r.emit(Event{Type: "done", Report: &r.report})
	return r.report
}

func (r *runner) emit(event Event) {
	if r.observe != nil {
		r.observe(event)
	}
}

func (r *runner) body(prompt string) map[string]any {
	return map[string]any{"model": r.report.Model, "max_tokens": 128, "messages": []any{map[string]any{"role": "user", "content": prompt}}}
}

func (r *runner) check(id, state, code string, evidence map[string]any) {
	// A transport/access problem is availability evidence, not a failed model
	// assertion. Only apply this to the check's own request, never an aggregate.
	if state == "fail" {
		for i := len(r.report.Samples) - 1; i >= 0; i-- {
			sample := r.report.Samples[i]
			if sample.Probe != id {
				continue
			}
			if sample.StopReason == "refusal" && sample.Valid != nil && *sample.Valid {
				state, code = "inconclusive", "request_refused"
			} else if sample.ErrorCode != "" && sample.ErrorCode != "invalid_response" {
				state, code = "inconclusive", sample.ErrorCode
			}
			break
		}
	}
	if r.ctx.Err() != nil && state == "fail" {
		state, code = "skipped", "cancelled"
	}
	check := Check{ID: id, Status: state, Code: code, Evidence: evidence}
	r.report.Checks = append(r.report.Checks, check)
	r.emit(Event{Type: "check", Check: &check})
}

func status(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func (r *runner) probe(id string, request Request) (message, Response, bool) {
	if r.ctx.Err() != nil {
		return message{}, Response{}, false
	}
	ctx, cancel := context.WithTimeout(r.ctx, ProbeTimeout)
	defer cancel()
	r.emit(Event{Type: "probe_start", Probe: id})
	start := time.Now()
	response, err := r.call(ctx, request)
	if response.Diagnostic == nil {
		response.Diagnostic = DiagnoseBedrock(response, err)
	}
	if response.Diagnostic != nil {
		if code := response.Diagnostic.failureCode(); code != "" {
			response.ErrorCode = code
		}
	}
	if response.ErrorCode == "" {
		response.ErrorCode = FailureCode(response.Status, err)
	}
	sample := Sample{Diagnostic: response.Diagnostic, RequestProfile: response.RequestProfile, ErrorCode: response.ErrorCode, EffectiveMaxTokens: response.EffectiveMaxTokens, Probe: id, Status: response.Status, DurationMS: time.Since(start).Milliseconds(), FirstEventMS: response.FirstEventMS, Stream: response.Stream, UpstreamModel: response.Model, Headers: map[string]string{}}
	for _, name := range []string{"x-amzn-errortype", "retry-after", "x-amzn-requestid", "request-id", "x-request-id", "x-amzn-bedrock-input-token-count", "x-amzn-bedrock-output-token-count", "server", "via", "cf-ray", "x-new-api-version", "anthropic-ratelimit-requests-remaining", "anthropic-ratelimit-input-tokens-remaining", "anthropic-ratelimit-input-tokens-reset"} {
		if value := response.Header.Get(name); value != "" {
			if len(value) > 1024 {
				value = value[:1024]
			}
			sample.Headers[name] = value
		}
	}
	for _, name := range []string{"x-amzn-requestid", "request-id", "x-request-id"} {
		if id := sample.Headers[name]; id != "" {
			sample.RequestID = id
			break
		}
	}
	if n, ok := request.Body["max_tokens"].(int); ok {
		number := int64(n)
		sample.RequestedMaxTokens = &number
	}
	var m message
	valid := true
	if stream, _ := request.Body["stream"].(bool); stream && !request.Count {
		m, valid = parseStream(response.Events)
	} else if !request.Count {
		if len(response.Body) == 0 && response.ErrorCode != "" {
			valid = false
		} else if common.Unmarshal(response.Body, &m) != nil {
			sample.ValidationErrors = []string{"invalid_message_json"}
		} else {
			blocks := len(m.Content)
			sample.ContentBlocks = &blocks
			sample.ValidationErrors = m.validationErrors(id == "max_tokens" || id == "zero_output")
		}
		valid = valid && len(sample.ValidationErrors) == 0
	}
	sample.ResponseModel, sample.MessageID, sample.StopReason, sample.Usage = m.Model, m.ID, m.StopReason, m.Usage
	if err != nil {
		sample.Error = err.Error()
	} // Transport returns only sanitized errors.
	validResponse := err == nil && response.Diagnostic == nil && response.Status == http.StatusOK && valid
	sample.Valid = &validResponse
	if !validResponse && response.ErrorCode == "" {
		response.ErrorCode, sample.ErrorCode = "invalid_response", "invalid_response"
	}
	// A refusal can have a valid wire envelope without establishing the probe's
	// capability. Preserve protocol validity, but stop semantic dependants from
	// treating refusal as a successful tool replay or usable baseline.
	if validResponse && m.StopReason == "refusal" {
		response.ErrorCode, sample.ErrorCode = "request_refused", "request_refused"
	}
	r.report.Samples = append(r.report.Samples, sample)
	r.emit(Event{Type: "sample", Sample: &sample})
	return m, response, validResponse && m.StopReason != "refusal"
}

func (r *runner) count(body map[string]any, usage Usage) {
	_, response, ok := r.probe("token_count", Request{Body: body, Count: true})
	var count struct {
		Input *int64 `json:"input_tokens"`
	}
	if !ok || common.Unmarshal(response.Body, &count) != nil || count.Input == nil || *count.Input < 0 {
		r.check("token_count", "inconclusive", "count_unavailable", map[string]any{"http_status": response.Status})
		return
	}
	actual, ok := totalInput(usage)
	if !ok {
		r.check("token_count", "inconclusive", "missing_usage", nil)
		return
	}
	state, code := "pass", "same_endpoint_count"
	if actual != *count.Input {
		state, code = "inconclusive", "count_difference"
	}
	r.check("token_count", state, code, map[string]any{"count_tokens": *count.Input, "input_plus_cache": actual, "difference": actual - *count.Input})
}

func (r *runner) behavior() {
	body := r.body("Reply with exactly PONG.")
	if !r.hasCheck("stream") {
		body["stream"] = true
		_, response, ok := r.probe("stream", Request{Body: body})
		r.baselineResult("stream", ok, response)
	}

	r.structuredTool()

	r.tokenLimit("max_tokens", 1, "Write a long numbered list of integers starting with 1, continuing to 1000.")

	body = r.body("Output exactly: ALPHA CHECK_STOP OMEGA")
	body["stop_sequences"] = []string{"CHECK_STOP"}
	m, _, ok := r.probe("stop_sequence", Request{Body: body})
	r.check("stop_sequence", status(ok && m.StopReason == "stop_sequence" && !strings.Contains(m.text(), "CHECK_STOP")), "observed", nil)

	needle := uuid.NewString()
	body = r.body("")
	body["messages"] = []any{map[string]any{"role": "user", "content": "Remember this code: " + needle}, map[string]any{"role": "assistant", "content": "I will remember the code."}, map[string]any{"role": "user", "content": "Return only the exact code I gave you."}}
	m, _, ok = r.probe("multi_turn", Request{Body: body})
	r.check("multi_turn", status(ok && m.text() == needle), "observed", nil)

	r.tokenLimit("zero_output", 0, "warmup")
}

func probeTools() []any {
	return []any{map[string]any{"name": "record_probe", "description": "Record the provided test value.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []string{"value"}}}}
}

func (r *runner) thinking() {
	body := r.body("Calculate 137 * 149. Think through the calculation, then call record_probe with the result as a string.")
	body["max_tokens"] = 2048
	body["tools"] = probeTools()
	body["thinking"] = r.thinkingMode()
	m, resp, ok := r.probe("thinking", Request{Body: body})
	signed := false
	var results []any
	for _, block := range m.Content {
		if block["type"] == "thinking" {
			signature, _ := block["signature"].(string)
			signed = signed || signature != ""
		}
		if block["type"] == "redacted_thinking" {
			data, _ := block["data"].(string)
			signed = signed || data != ""
		}
		if block["type"] == "tool_use" {
			if id, yes := block["id"].(string); yes && id != "" {
				results = append(results, map[string]any{"type": "tool_result", "tool_use_id": id, "content": "Recorded successfully."})
			}
		}
	}
	if !ok || !signed {
		r.check("thinking", "inconclusive", "thinking_unavailable", map[string]any{"http_status": resp.Status})
		r.check("signature_replay", "skipped", "dependency_unavailable", nil)
		return
	}
	r.check("thinking", "pass", "signature_present", nil)
	if len(results) == 0 || m.StopReason != "tool_use" {
		r.check("signature_replay", "inconclusive", "dependency_unavailable", nil)
		return
	}
	// Preserve every content block, including opaque signatures, verbatim in
	// meaning. Never decode or mutate the signature, nor claim local validation.
	body["messages"] = append(body["messages"].([]any), map[string]any{"role": "assistant", "content": m.Content}, map[string]any{"role": "user", "content": results})
	m, _, ok = r.probe("signature_replay", Request{Body: body})
	r.check("signature_replay", status(ok && m.valid()), "replay_only", nil)
}

func (r *runner) cache() {
	if r.report.TokenAudit != nil && r.report.TokenAudit.Version >= 4 {
		r.repeatedCache()
		return
	}
	var prefix strings.Builder
	fmt.Fprintf(&prefix, "Synthetic cache test %s. Read this reference and answer the question below.\n", uuid.NewString())
	for i := 0; i < 384; i++ {
		fmt.Fprintf(&prefix, "Record %d: amber birch cedar delta elm frost green harbor iris jade kite lemon maple north oak pine.\n", i)
	}
	body := r.body("Return only the first word in Record 73.")
	body["system"] = []any{map[string]any{"type": "text", "text": prefix.String(), "cache_control": map[string]any{"type": "ephemeral"}}}
	var usages []Usage
	var first Sample
	defer func() { r.check("cache_token_audit", "inconclusive", "cache_token_audit_observed", nil) }()
	for i, id := range []string{"cache_write", "cache_read_1", "cache_read_2"} {
		if r.ctx.Err() != nil {
			break
		}
		m, _, ok := r.probe(id, Request{Body: body})
		if i == 0 {
			first = r.report.Samples[len(r.report.Samples)-1]
		} else {
			r.compareCacheTokens(i-1, first, r.report.Samples[len(r.report.Samples)-1])
		}
		if !ok || !m.valid() {
			r.check("cache", "inconclusive", "cache_unconfirmed", nil)
			return
		}
		usages = append(usages, m.Usage)
	}
	if len(usages) != 3 {
		return
	}
	var read, write, total int64
	for _, u := range usages {
		if u.CacheRead == nil || u.CacheWrite == nil {
			r.check("cache", "inconclusive", "missing_usage", nil)
			return
		}
		n, ok := totalInput(u)
		if !ok {
			r.check("cache", "fail", "unexpected_usage", nil)
			return
		}
		total += n
		read += *u.CacheRead
		write += *u.CacheWrite
	}
	rate := float64(0)
	if total > 0 {
		rate = float64(read) / float64(total)
	}
	state, code := "inconclusive", "cache_unconfirmed"
	if *usages[0].CacheWrite > 0 && (*usages[1].CacheRead > 0 || *usages[2].CacheRead > 0) {
		state, code = "pass", "cache_observed"
	}
	r.check("cache", state, code, map[string]any{"cache_write_tokens": write, "cache_read_tokens": read, "input_plus_cache": total, "read_token_ratio": rate, "ttl": "default", "requests": len(usages), "warm_requests": 2, "warm_hits": boolInt(*usages[1].CacheRead > 0) + boolInt(*usages[2].CacheRead > 0), "first_write_tokens": *usages[0].CacheWrite, "repeated_write_tokens": *usages[1].CacheWrite + *usages[2].CacheWrite})
}

func (r *runner) thinkingMode() map[string]any {
	modelName := r.upstreamModel
	if modelName == "" {
		modelName = r.report.Model
	}
	name := strings.ReplaceAll(strings.ToLower(modelName), ".", "-")
	adaptive := false
	for _, s := range []string{"opus-4-6", "opus-4-7", "opus-4-8", "sonnet-4-6", "opus-5", "sonnet-5", "fable", "mythos"} {
		adaptive = adaptive || strings.Contains(name, s)
	}
	if adaptive {
		return map[string]any{"type": "adaptive"}
	} else {
		return map[string]any{"type": "enabled", "budget_tokens": 1024}
	}
}
