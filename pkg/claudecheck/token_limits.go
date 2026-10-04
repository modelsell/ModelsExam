package claudecheck

import "net/http"

// https://platform.claude.com/docs/en/build-with-claude/prompt-caching
// Zero is a valid non-streaming pre-warm request, not an invalid parameter.
func (r *runner) tokenLimit(id string, limit int, prompt string) {
	if r.ctx.Err() != nil {
		return
	}
	body := r.body(prompt)
	body["max_tokens"] = limit
	before := len(r.report.Samples)
	m, response, ok := r.probe(id, Request{Body: body})
	if len(r.report.Samples) == before {
		return
	}
	sample := r.report.Samples[len(r.report.Samples)-1]
	evidence := map[string]any{
		"requested_max_tokens": limit, "effective_max_tokens": response.EffectiveMaxTokens,
		"http_status": response.Status, "output_tokens": m.Usage.Output,
		"stop_reason": m.StopReason, "content_blocks": sample.ContentBlocks,
		"validation_errors": sample.ValidationErrors,
	}
	if id == "zero_output" {
		if response.Status >= 400 {
			r.errorShape(response)
		} else {
			r.check("error_shape", "skipped", "no_error_response", map[string]any{"http_status": response.Status})
		}
	}
	state, code := "inconclusive", "token_limit_unavailable"
	switch {
	case r.ctx.Err() != nil:
		state, code = "skipped", "cancelled"
	case response.EffectiveMaxTokens != nil && *response.EffectiveMaxTokens != int64(limit):
		code = "token_limit_overridden"
	case sample.Error != "":
		// Transport/read errors can occur after receiving HTTP 200 headers.
	case response.Status != http.StatusOK:
		// Unsupported models/routes, rate limits and network errors provide no
		// evidence about whether the model obeyed a token budget.
	case !ok:
		state, code = "fail", "token_limit_invalid_response"
	case *m.Usage.Output > int64(limit):
		state, code = "fail", "token_limit_exceeded"
	case limit == 0 && len(m.Content) > 0:
		state, code = "fail", "zero_output_has_content"
	case m.StopReason == "max_tokens" && *m.Usage.Output == int64(limit):
		state, code = "pass", "token_limit_observed"
		if limit == 0 {
			code = "zero_output_observed"
		}
	default:
		// The API can end naturally before the cap, or refuse the synthetic
		// request. Neither demonstrates broken truncation behavior.
		code = "token_limit_not_reached"
	}
	r.check(id, state, code, evidence)
}
