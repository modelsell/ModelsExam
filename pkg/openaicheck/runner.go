package openaicheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"
)

type runner struct {
	ctx       context.Context
	call      Transport
	observe   func(Event)
	opts      Options
	report    Report
	plan      map[string]PlanItem
	reasoning bool
	requests  int
}

type result struct {
	sample Sample
	resp   Response
	v      view
	doc    object
	issues []string
	ok     bool // HTTP 200, no transport error, and the response contract holds
}

// newID returns a random UUID (v4) so saved reports share the history routes
// and id validation used by the Claude checks.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// IsReasoningModel reports OpenAI reasoning families, which reject sampling
// parameters and spend part of the token budget on hidden reasoning.
func IsReasoningModel(model string) bool {
	name := strings.ToLower(model)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if strings.Contains(name, "chat") {
		return false
	}
	for _, prefix := range []string{"o1", "o3", "o4", "gpt-5"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Run executes the selected suite. Requests are never retried.
func Run(ctx context.Context, options Options, transport Transport) Report {
	return RunWithObserver(ctx, options, transport, nil)
}

func RunWithObserver(ctx context.Context, options Options, transport Transport, observe func(Event)) Report {
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()
	start := time.Now()
	options = options.normalized()
	r := &runner{ctx: ctx, call: transport, observe: observe, opts: options, plan: map[string]PlanItem{}, reasoning: IsReasoningModel(options.Model)}
	r.report = Report{Version: ReportVersion, ID: newID(), Provider: "openai", Model: options.Model, StartedAt: start.UTC().Format(time.RFC3339), Checks: []Check{}, Samples: []Sample{}, Summary: map[string]int{}}
	r.report.Options = &options
	r.report.Plan = Plan(options)
	for _, item := range r.report.Plan {
		r.plan[item.ID] = item
	}
	r.report.Limits = &RunLimits{ProbeTimeoutSeconds: int(ProbeTimeout / time.Second), RunTimeoutSeconds: int(RunTimeout / time.Second), MaxRequests: MaxRequests(options)}
	r.emit(Event{Type: "start", Report: &r.report})

	r.modelsList()
	if r.chatBasic() {
		r.chatSuite()
		r.responsesSuite()
		r.errorShape()
	} else if r.report.StopReason == "" {
		// A rejected chat request (for example a Responses-only model, or a
		// token-limit parameter the server does not know) must not hide the
		// other APIs. Availability failures stop the run in chatBasic.
		r.responsesSuite()
		r.errorShape()
	}
	r.usageFields()
	r.modelConsistency()
	r.performance()

	for _, item := range r.report.Plan {
		if r.hasCheck(item.ID) {
			continue
		}
		code := "not_run"
		switch {
		case !item.Selected:
			code = "not_requested"
		case r.ctx.Err() != nil:
			code = "cancelled"
		case r.report.StopReason != "":
			code = "run_stopped"
		case strings.HasPrefix(item.ID, "chat_") && item.ID != "chat_basic":
			code = "baseline_failed"
		}
		r.check(item.ID, "skipped", code, nil)
	}
	r.report.Cancelled = ctx.Err() != nil
	if ctx.Err() == context.DeadlineExceeded {
		r.report.StopReason = "run_timeout"
	} else if ctx.Err() != nil {
		r.report.StopReason = "cancelled"
	}
	if r.report.StopReason != "" && r.report.StopProbe == "" && len(r.report.Samples) > 0 {
		r.report.StopProbe = r.report.Samples[len(r.report.Samples)-1].Probe
	}
	r.report.DurationMS = time.Since(start).Milliseconds()
	r.report.RequestsRun = r.requests
	Finalize(&r.report)
	r.emit(Event{Type: "done", Report: &r.report})
	return r.report
}

// Finalize recomputes the summary and score from the recorded checks.
func Finalize(report *Report) {
	report.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, c := range report.Checks {
		report.Summary[c.Status]++
	}
	report.Score = Score(*report)
}

// Score is the share of passed assertions among decided assertions. Observations,
// inconclusive and skipped checks never move it, and it is nil when nothing was
// decided. It rates API compatibility, not model authenticity.
func Score(report Report) *int {
	pass, fail := 0, 0
	for _, c := range report.Checks {
		if c.Kind != "assertion" {
			continue
		}
		switch c.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		}
	}
	if pass+fail == 0 {
		return nil
	}
	score := (pass*100 + (pass+fail)/2) / (pass + fail)
	return &score
}

func (r *runner) emit(e Event) {
	if r.observe != nil {
		r.observe(e)
	}
}

func (r *runner) selected(id string) bool { return r.plan[id].Selected }

func (r *runner) hasCheck(id string) bool {
	for _, c := range r.report.Checks {
		if c.ID == id {
			return true
		}
	}
	return false
}

// live reports whether a selected probe may still run.
func (r *runner) live(id string) bool {
	return r.selected(id) && r.ctx.Err() == nil && r.report.StopReason == ""
}

func (r *runner) check(id, state, code string, evidence map[string]any) {
	if r.hasCheck(id) {
		return
	}
	item := r.plan[id]
	kind := item.Kind
	if kind == "" {
		kind = "assertion"
	}
	if kind == "observation" && state == "fail" {
		state = "inconclusive"
	}
	if r.ctx.Err() != nil && state == "fail" {
		state, code = "skipped", "cancelled"
	}
	check := Check{ID: id, Stage: item.Stage, Kind: kind, Status: state, Code: code, Evidence: evidence}
	r.report.Checks = append(r.report.Checks, check)
	r.emit(Event{Type: "check", Check: &check})
}

// judge turns a probe result into a check. A transport or access problem is
// availability evidence and stays inconclusive; a refused or malformed response
// is a compatibility failure.
func (r *runner) judge(id string, res *result, pass bool, okCode, failCode string, evidence map[string]any) {
	if evidence == nil {
		evidence = map[string]any{}
	}
	if !res.ok {
		code := res.sample.ErrorCode
		if code == "" {
			code = "invalid_response"
		}
		evidence["http_status"] = res.sample.Status
		if len(res.issues) > 0 {
			evidence["issues"] = res.issues
		}
		if res.sample.Error != "" {
			evidence["upstream_error"] = res.sample.Error
		}
		if IsAvailabilityCode(code) {
			r.check(id, "inconclusive", code, evidence)
			return
		}
		r.check(id, "fail", code, evidence)
		return
	}
	if pass {
		r.check(id, "pass", okCode, evidence)
	} else {
		r.check(id, "fail", failCode, evidence)
	}
}

var headerNames = []string{"x-request-id", "openai-processing-ms", "openai-version", "openai-model", "retry-after", "x-ratelimit-remaining-requests", "x-ratelimit-remaining-tokens", "server", "via", "cf-ray", "x-new-api-version"}

func (r *runner) probe(id, kind string, req Request) *result {
	res := &result{}
	if r.ctx.Err() != nil {
		res.sample = Sample{Probe: id, Kind: kind, ErrorCode: "cancelled"}
		return res
	}
	ctx, cancel := context.WithTimeout(r.ctx, ProbeTimeout)
	defer cancel()
	r.emit(Event{Type: "probe_start", Probe: id})
	r.requests++
	started := time.Now()
	resp, err := r.call(ctx, req)
	res.resp = resp
	sample := Sample{Probe: id, Kind: kind, Method: req.Method, Path: req.Path, Status: resp.Status, DurationMS: time.Since(started).Milliseconds(), FirstEventMS: resp.FirstEventMS, Stream: req.Stream, ErrorCode: resp.ErrorCode, Headers: map[string]string{}}
	for _, name := range headerNames {
		if value := resp.Header.Get(name); value != "" {
			sample.Headers[name] = truncate(value, 512)
		}
	}
	if sample.ErrorCode == "" && kind != KindError {
		sample.ErrorCode = FailureCode(resp.Status, err)
	}
	if err != nil {
		sample.Error = truncate(err.Error(), 400)
	} else if resp.Status != http.StatusOK && kind != KindError {
		sample.Error = upstreamError(resp.Body)
	}
	httpOK := err == nil && resp.Status == http.StatusOK
	if httpOK {
		switch kind {
		case KindModels:
			listed, count, issues := parseModels(resp.Body, r.report.Model)
			res.issues = issues
			res.doc = object{"listed": listed, "count": float64(count)}
		case KindChat:
			if req.Stream {
				res.v, res.issues = parseChatStream(resp.Events, resp.Done)
			} else {
				res.v, res.issues = parseChat(resp.Body)
				res.doc, _ = parseObject(resp.Body)
			}
		case KindResponses:
			if req.Stream {
				res.v, res.issues = parseResponsesStream(resp.Events)
			} else {
				res.v, res.issues = parseResponses(resp.Body)
				res.doc, _ = parseObject(resp.Body)
			}
		}
		res.issues = dedupe(res.issues)
	}
	res.ok = httpOK && len(res.issues) == 0
	if kind != KindError {
		valid := res.ok
		sample.Valid = &valid
		sample.ValidationErrors = res.issues
		if httpOK && !res.ok && sample.ErrorCode == "" {
			sample.ErrorCode = "invalid_response"
		}
	}
	sample.ResponseModel, sample.ResponseID, sample.FinishReason, sample.Fingerprint = res.v.Model, res.v.ID, res.v.Finish, res.v.Fingerprint
	sample.Usage, sample.UsageIssues = res.v.Usage, res.v.UsageIssues
	res.sample = sample
	r.report.Samples = append(r.report.Samples, sample)
	r.emit(Event{Type: "sample", Sample: &sample})
	return res
}

// chatBasic establishes the baseline. Credential, quota, network and upstream
// failures stop the run, because every later probe would only repeat them.
func (r *runner) chatBasic() bool {
	if !r.selected("chat_basic") || r.ctx.Err() != nil {
		return false
	}
	res := r.probe("chat_basic", KindChat, r.chatRequest(r.chatBody(r.budget(256), user("Reply with exactly PONG."))))
	evidence := map[string]any{"finish_reason": res.v.Finish, "text_length": len(res.v.Text)}
	if !res.ok {
		if msg := res.sample.Error; strings.Contains(msg, "max_completion_tokens") || strings.Contains(msg, "max_tokens") {
			evidence["hint"] = "the endpoint rejected the token-limit parameter; retry with limit_param set to the other name"
		}
	}
	r.judge("chat_basic", res, res.v.Finish == "stop" && res.v.Text != "", "envelope_valid", "unexpected_finish_or_empty", evidence)
	if !res.ok && IsAvailabilityCode(res.sample.ErrorCode) {
		r.report.StopReason, r.report.StopProbe = "baseline_unavailable", "chat_basic"
		return false
	}
	// A 200 with a broken contract still deserves the rest of the suite: those
	// are exactly the findings a user needs. Only a refused baseline skips it.
	return res.sample.Status == http.StatusOK && res.sample.ErrorCode != "cancelled"
}

func (r *runner) chatRequest(body map[string]any) Request {
	stream, _ := body["stream"].(bool)
	return Request{Method: http.MethodPost, Path: "/v1/chat/completions", Body: body, Stream: stream}
}

func (r *runner) responsesRequest(body map[string]any) Request {
	stream, _ := body["stream"].(bool)
	return Request{Method: http.MethodPost, Path: "/v1/responses", Body: body, Stream: stream}
}

func (r *runner) chatBody(limit int, messages ...any) map[string]any {
	body := map[string]any{"model": r.report.Model, "messages": messages}
	if limit > 0 {
		body[r.opts.LimitParam] = limit
	}
	return body
}

// budget leaves room for hidden reasoning tokens on reasoning models.
func (r *runner) budget(n int) int {
	if r.reasoning && n < 2048 {
		return 2048
	}
	return n
}

func user(text string) object      { return object{"role": "user", "content": text} }
func system(text string) object    { return object{"role": "system", "content": text} }
func assistant(text string) object { return object{"role": "assistant", "content": text} }

func (r *runner) modelsList() {
	if !r.live("models_list") {
		return
	}
	res := r.probe("models_list", KindModels, Request{Method: http.MethodGet, Path: "/v1/models"})
	listed, _ := res.doc["listed"].(bool)
	count, _ := res.doc["count"].(float64)
	evidence := map[string]any{"models_returned": int(count), "target_listed": listed}
	// Relays often hide or alias models, so absence is not a failure; the check
	// is an observation and a "fail" is recorded as inconclusive.
	r.judge("models_list", res, listed, "model_listed", "model_not_listed", evidence)
}

func (r *runner) errorShape() {
	if !r.live("error_shape") {
		return
	}
	// A missing required field must be rejected as a client error. This costs
	// no tokens on a conforming endpoint.
	res := r.probe("error_shape", KindError, Request{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{"model": r.report.Model}})
	status := res.resp.Status
	code := FailureCode(status, nil)
	if status == http.StatusOK {
		r.check("error_shape", "fail", "accepted_invalid_request", map[string]any{"http_status": status})
		return
	}
	if IsAvailabilityCode(code) || status == 0 {
		if status == 0 {
			code = "network_error"
		}
		r.check("error_shape", "inconclusive", code, map[string]any{"http_status": status})
		return
	}
	message, issues := parseErrorShape(status, res.resp.Body)
	evidence := map[string]any{"http_status": status, "message": truncate(message, 160)}
	if len(issues) > 0 {
		evidence["issues"] = issues
		r.check("error_shape", "fail", "error_shape_mismatch", evidence)
		return
	}
	r.check("error_shape", "pass", "openai_error_shape", evidence)
}

func (r *runner) usageFields() {
	considered, missing, bad := 0, 0, 0
	var problems []string
	for _, s := range r.report.Samples {
		if (s.Kind != KindChat && s.Kind != KindResponses) || s.Status != http.StatusOK {
			continue
		}
		considered++
		if s.Usage.Total == nil || s.Usage.Input == nil || s.Usage.Output == nil {
			missing++
			continue
		}
		issues := append(append([]string{}, s.UsageIssues...), usageIssues(s.ValidationErrors)...)
		if len(issues) > 0 || *s.Usage.Input <= 0 {
			bad++
			problems = append(problems, s.Probe)
		}
	}
	evidence := map[string]any{"responses_inspected": considered, "missing_usage": missing, "inconsistent_usage": bad}
	switch {
	case considered == 0:
		r.check("usage_fields", "inconclusive", "no_successful_samples", evidence)
	case missing > 0:
		r.check("usage_fields", "fail", "usage_missing", evidence)
	case bad > 0:
		evidence["probes"] = problems
		r.check("usage_fields", "fail", "usage_inconsistent", evidence)
	default:
		r.check("usage_fields", "pass", "usage_consistent", evidence)
	}
}

func usageIssues(all []string) []string {
	var out []string
	for _, issue := range all {
		if strings.Contains(issue, "usage") || strings.HasPrefix(issue, "missing_prompt") || strings.HasPrefix(issue, "missing_completion") || strings.HasPrefix(issue, "missing_input") || strings.HasPrefix(issue, "missing_output") || strings.HasPrefix(issue, "missing_total") || strings.HasPrefix(issue, "negative_") {
			out = append(out, issue)
		}
	}
	return out
}

func normalizeModel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// modelMatches accepts the requested name, or the same model with a dated or
// "latest" snapshot suffix. "gpt-4o" must not match "gpt-4o-mini-...".
func modelMatches(requested, observed string) bool {
	requested, observed = normalizeModel(requested), normalizeModel(observed)
	if requested == observed {
		return true
	}
	suffix := func(base, full string) bool {
		if !strings.HasPrefix(full, base+"-") {
			return false
		}
		rest := full[len(base)+1:]
		return rest == "latest" || (rest != "" && rest[0] >= '0' && rest[0] <= '9')
	}
	return suffix(requested, observed) || suffix(observed, requested)
}

func (r *runner) modelConsistency() {
	distinct := map[string]bool{}
	prints := map[string]bool{}
	mismatch := false
	for _, s := range r.report.Samples {
		if s.ResponseModel != "" {
			distinct[s.ResponseModel] = true
			mismatch = mismatch || !modelMatches(r.report.Model, s.ResponseModel)
		}
		if s.Fingerprint != "" {
			prints[s.Fingerprint] = true
		}
	}
	observed := keys(distinct)
	evidence := map[string]any{"requested": r.report.Model, "observed": observed, "system_fingerprints": keys(prints), "identity_verified": false}
	switch {
	case len(observed) == 0:
		r.check("model_consistency", "inconclusive", "no_model_reported", evidence)
	case mismatch:
		r.check("model_consistency", "inconclusive", "model_mismatch", evidence)
	case len(distinct) > 1:
		// Snapshot aliases can differ in suffix only; report it without judging.
		r.check("model_consistency", "inconclusive", "model_changed_between_requests", evidence)
	default:
		r.check("model_consistency", "pass", "model_echo_matches", evidence)
	}
}

func (r *runner) performance() {
	var durations, firsts []int64
	for _, s := range r.report.Samples {
		if (s.Kind != KindChat && s.Kind != KindResponses) || s.Status != http.StatusOK {
			continue
		}
		durations = append(durations, s.DurationMS)
		if s.Stream && s.FirstEventMS != nil {
			firsts = append(firsts, *s.FirstEventMS)
		}
	}
	if len(durations) == 0 {
		r.check("performance", "inconclusive", "no_successful_samples", nil)
		return
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	evidence := map[string]any{"requests": len(durations), "median_ms": durations[len(durations)/2], "max_ms": durations[len(durations)-1]}
	if len(firsts) > 0 {
		sort.Slice(firsts, func(i, j int) bool { return firsts[i] < firsts[j] })
		evidence["stream_first_event_median_ms"] = firsts[len(firsts)/2]
	}
	r.check("performance", "pass", "latency_observed", evidence)
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
