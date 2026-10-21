package openaicheck

import (
	"math"
	"net/http"
	"strconv"
)

const (
	// PromptInjectionRequests is the number of identical minimal requests whose
	// input usage is compared with the allowance. chat_basic is the first one.
	PromptInjectionRequests = 3

	// PromptInputAllowance is a conservative product heuristic, mirroring the
	// Claude check, not an official token count. The minimal request is one
	// short user message (about 13 prompt tokens with OpenAI's chat framing), so
	// the allowance also covers chat templates of other model families behind
	// compatible endpoints. Small additions can be missed.
	PromptInputAllowance int64 = 64

	promptInjectionText = "Reply with exactly PONG."
)

// promptInjectionBody is shared with chat_basic, so the baseline request is
// also the first input sample.
func (r *runner) promptInjectionBody() map[string]any {
	return r.chatBody(r.budget(256), user(promptInjectionText))
}

// sampleInput returns the total input a sample was billed for. Cached tokens
// are a subset of prompt_tokens in the OpenAI contract; a converted upstream
// that reports them separately would otherwise hide a cached hidden prompt.
func sampleInput(s Sample) (int64, bool) {
	if s.Status != http.StatusOK || s.Usage.Input == nil {
		return 0, false
	}
	input := *s.Usage.Input
	if s.Usage.Cached != nil && *s.Usage.Cached > input {
		input += *s.Usage.Cached
	}
	return input, input > 0
}

func promptInjectionScore(input int64) int {
	if input <= PromptInputAllowance {
		return 100
	}
	return min(99, int(math.Round(100*float64(PromptInputAllowance)/float64(input))))
}

// promptInjection compares the input usage of a fixed minimal request with an
// allowance. A relay that prepends a hidden system prompt (or bills for one)
// reports far more input than the user sent. Usage is self-reported by the
// endpoint, so a pass is not proof that nothing was added.
func (r *runner) promptInjection() {
	if !r.live("prompt_injection") {
		return
	}
	probes := []string{"chat_basic"}
	for i := 2; i <= PromptInjectionRequests; i++ {
		res := r.probe("prompt_injection_r"+strconv.Itoa(i), KindChat, r.chatRequest(r.promptInjectionBody()))
		probes = append(probes, res.sample.Probe)
		if IsAvailabilityCode(res.sample.ErrorCode) || r.ctx.Err() != nil {
			break
		}
	}
	var inputs []int64
	var extras []int64
	sum, extraRounds := 0, 0
	for _, s := range r.report.Samples {
		matched := false
		for _, probe := range probes {
			matched = matched || s.Probe == probe
		}
		input, ok := sampleInput(s)
		if !matched || !ok {
			continue
		}
		extra := max(int64(0), input-PromptInputAllowance)
		if extra > 0 {
			extraRounds++
		}
		inputs, extras = append(inputs, input), append(extras, extra)
		sum += promptInjectionScore(input)
	}
	evidence := map[string]any{"allowance_tokens": PromptInputAllowance, "planned_samples": PromptInjectionRequests, "valid_samples": len(inputs), "input_tokens": inputs, "injection_verified": false}
	if len(inputs) == 0 {
		code := "no_usable_usage"
		if last := r.report.Samples[len(r.report.Samples)-1]; last.Probe != "chat_basic" && IsAvailabilityCode(last.ErrorCode) {
			code = last.ErrorCode
		}
		r.check("prompt_injection", "inconclusive", code, evidence)
		return
	}
	// Divide before summing so untrusted counters cannot overflow the average.
	var average, remainder, peak int64
	n := int64(len(extras))
	for _, extra := range extras {
		average += extra / n
		remainder += extra % n
		peak = max(peak, extra)
	}
	average += (remainder + n/2) / n
	score := int(math.Round(float64(sum) / float64(len(inputs))))
	if extraRounds > 0 {
		score = min(99, score)
	}
	evidence["estimated_extra_tokens"], evidence["max_extra_tokens"], evidence["extra_rounds"], evidence["score"] = average, peak, extraRounds, score
	if extraRounds > 0 {
		r.check("prompt_injection", "fail", "extra_input_tokens", evidence)
		return
	}
	r.check("prompt_injection", "pass", "no_obvious_injection", evidence)
}
