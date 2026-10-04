package claudecheck

import "strings"

func (r *runner) hasCheck(id string) bool {
	for _, check := range r.report.Checks {
		if check.ID == id {
			return true
		}
	}
	return false
}

func (r *runner) establishBaseline() bool {
	body := r.body("Reply with exactly PONG.")
	body["system"] = "Respond with exactly PONG and no other text."
	base, response, ok := r.probe("basic", Request{Body: body})
	r.upstreamModel = response.Model
	r.baselineResult("basic", ok, response)
	probe := "basic"
	if !ok && r.ctx.Err() == nil && baselineRecoverable(response.ErrorCode) {
		// Consume the planned stream test once, with the same system/messages.
		// It provides an independent path through a proxy that may buffer or
		// mishandle non-streaming responses, without duplicating paid requests.
		probe = "stream"
		body["stream"] = true
		base, response, ok = r.probe(probe, Request{Body: body})
		if response.Model != "" {
			r.upstreamModel = response.Model
		}
		r.baselineResult(probe, ok, response)
	}
	if !ok {
		r.report.StopReason = response.ErrorCode
		if r.report.StopReason == "" || r.report.StopReason == "invalid_response" {
			r.report.StopReason = "baseline_unavailable"
		}
		return false
	}
	r.report.BaselineProbe = probe
	r.modelEcho(base, response)
	r.check("source", "inconclusive", "fingerprint_only", map[string]any{"baseline_probe": probe, "message_id": base.ID, "response_model": base.Model, "upstream_model": response.Model, "bedrock_hint": strings.HasPrefix(base.ID, "msg_bdrk_") || strings.Contains(base.Model, "anthropic.claude") || response.Header.Get("x-amzn-requestid") != ""})
	r.count(body, base.Usage)
	r.coldCache(base.Usage)
	r.check("system", status(base.text() == "PONG"), "observed", map[string]any{"baseline_probe": probe, "matched": base.text() == "PONG"})
	return true
}
