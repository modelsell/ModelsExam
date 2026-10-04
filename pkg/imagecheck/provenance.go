package imagecheck

import (
	"context"
	"errors"
	"slices"

	"model-check/pkg/media"
	"model-check/pkg/openaicheck"
	"model-check/pkg/provenance"
)

var provIDs = []string{"img_prov_control", "img_prov_c2pa", "img_prov_synthid", "img_prov_model_match"}

// verify uploads one image. A non-empty code means no result: the service did
// not answer, which says nothing about the endpoint under test. Failures that
// would repeat for every later call block the rest of the stage.
func (r *runner) verify(data []byte, mime string) (*provenance.Result, string, map[string]any) {
	if r.verifyBlock != "" {
		return nil, r.verifyBlock, nil
	}
	if r.deps.Verify == nil {
		r.verifyBlock = "verifier_not_configured"
		return nil, r.verifyBlock, nil
	}
	if r.ctx.Err() != nil {
		return nil, "cancelled", nil
	}
	ctx, cancel := context.WithTimeout(r.ctx, provenance.Timeout)
	defer cancel()
	r.verifyCalls++
	res, err := r.deps.Verify(ctx, data, mime)
	if err == nil {
		return res, "", nil
	}
	ev := map[string]any{}
	code := openaicheck.FailureCode(0, err)
	var api *provenance.APIError
	if errors.As(err, &api) {
		code = api.Kind()
		ev["http_status"] = api.Status
		if api.RetryAfter != "" {
			ev["retry_after"] = api.RetryAfter
		}
		if api.Message != "" {
			ev["upstream_error"] = api.Message
		}
	}
	switch code {
	case "rate_limited", "no_access", "unauthorized", "upstream_error", "timeout", "network_error", "cancelled":
		r.verifyBlock = code
	}
	return nil, code, ev
}

// provUnavailable marks every provenance check that has not run as
// inconclusive with the reason the service could not be used.
func (r *runner) provUnavailable(code string, ev map[string]any) {
	for _, id := range provIDs {
		if r.selected(id) && !r.hasCheck(id) {
			r.check(id, "inconclusive", code, ev)
		}
	}
	r.prov.Level, r.prov.UnavailableCode = "unavailable", code
}

func detected(res *provenance.Result) bool {
	return res != nil && (res.C2PA != nil && res.C2PA.Outcome == "detected" || res.SynthID != nil && res.SynthID.Outcome == "detected")
}

func (r *runner) provenanceBasic() {
	if !r.selected("img_prov_control") || r.ctx.Err() != nil {
		return
	}
	// Negative control: a locally made image no generator touched must come
	// back clean, or the verification path cannot be trusted at all.
	ctrl := media.SolidPNG(256, 256, media.RGB{R: uint8(r.rng.IntN(256)), G: uint8(r.rng.IntN(256)), B: uint8(r.rng.IntN(256))})
	res, code, ev := r.verify(ctrl, "image/png")
	if code != "" {
		r.provUnavailable(code, ev)
		return
	}
	clean := !detected(res)
	r.prov.ControlOK = &clean
	if !clean {
		r.check("img_prov_control", "fail", "control_detected", map[string]any{"c2pa": res.C2PA, "synthid": res.SynthID})
		r.prov.Level, r.prov.UnavailableCode = "unavailable", "control_failed"
		for _, id := range provIDs[1:] {
			r.check(id, "inconclusive", "control_failed", nil)
		}
		return
	}
	r.check("img_prov_control", "pass", "control_clean", nil)

	d := r.basic.images[0]
	res, code, ev = r.verify(d.data, d.mime())
	if code != "" {
		r.provUnavailable(code, ev)
		return
	}
	v := provenance.Classify(res, r.opts.Model)
	r.prov.Verdict = &v
	evidence := map[string]any{"validation_state": v.C2PAState, "issuer": v.Issuer, "model": v.Model, "generated_at": v.GeneratedAt}
	switch {
	case v.Level == provenance.LevelTrusted:
		r.check("img_prov_c2pa", "pass", "trusted", evidence)
	case res.C2PA == nil:
		r.check("img_prov_c2pa", "inconclusive", "no_c2pa_entry", evidence)
	case res.C2PA.Outcome != "detected":
		r.check("img_prov_c2pa", "inconclusive", "c2pa_not_detected", evidence)
	case v.C2PAState == "trusted":
		r.check("img_prov_c2pa", "inconclusive", "c2pa_issuer_not_openai", evidence)
	default:
		r.check("img_prov_c2pa", "inconclusive", "c2pa_"+v.C2PAState, evidence)
	}
	switch {
	case v.SynthID:
		r.check("img_prov_synthid", "pass", "synthid_detected", nil)
	case res.SynthID == nil:
		r.check("img_prov_synthid", "inconclusive", "no_synthid_entry", nil)
	default:
		r.check("img_prov_synthid", "inconclusive", "synthid_not_detected", nil)
	}
	switch {
	case v.ModelMatch == nil:
		r.check("img_prov_model_match", "inconclusive", "no_model_claim", nil)
	case *v.ModelMatch:
		r.check("img_prov_model_match", "pass", "model_match", map[string]any{"requested": r.opts.Model, "claimed": v.Model})
	default:
		r.check("img_prov_model_match", "fail", "model_mismatch", map[string]any{"requested": r.opts.Model, "claimed": v.Model})
	}
}

func strong(level string) bool {
	return level == provenance.LevelTrusted || level == provenance.LevelSynthID
}

// provenanceFormats verifies the JPEG/WebP images from the output-format probe
// to show which formats keep the signal.
func (r *runner) provenanceFormats() {
	const id = "img_prov_format_matrix"
	if !r.live(id) {
		return
	}
	if r.prov.ControlOK == nil || !*r.prov.ControlOK || r.prov.Verdict == nil {
		r.check(id, "inconclusive", "relay_unverified", nil)
		return
	}
	levels := map[string]string{"png": r.prov.Verdict.Level}
	var lost []string
	for _, f := range extraFormats(r.prof) {
		d := r.byFormat[f]
		if d == nil {
			continue
		}
		res, code, ev := r.verify(d.data, d.mime())
		if code != "" {
			r.check(id, "inconclusive", code, ev)
			return
		}
		levels[f] = provenance.Classify(res, r.opts.Model).Level
	}
	r.prov.Formats = levels
	ev := map[string]any{"levels": levels}
	if len(levels) < 2 {
		r.check(id, "inconclusive", "no_format_images", ev)
		return
	}
	for f, level := range levels {
		if !strong(level) {
			lost = append(lost, f)
		}
	}
	slices.Sort(lost)
	switch {
	case len(lost) == 0:
		r.check(id, "pass", "all_formats_retain", ev)
	default:
		ev["without_signal"] = lost
		r.check(id, "inconclusive", "signal_lost", ev)
	}
}

// baselineCompare generates the same image straight from OpenAI and compares
// its provenance with what the endpoint under test returned.
func (r *runner) baselineCompare() {
	const id = "img_prov_baseline"
	if !r.live(id) {
		return
	}
	switch {
	case r.deps.Baseline == nil:
		r.check(id, "inconclusive", "baseline_not_configured", nil)
		return
	case r.basic == nil || !r.basic.ok || r.prov.Verdict == nil || r.prov.ControlOK == nil || !*r.prov.ControlOK:
		r.check(id, "inconclusive", "relay_unverified", nil)
		return
	}
	t := r.pickColors(1)[0]
	g := r.probe(id, r.deps.Baseline, genRequest(r.body(r.solidPrompt(t), nil), false), 1)
	r.keep(id, g)
	if !g.ok {
		code := g.sample.ErrorCode
		if code == "" {
			code = "baseline_generation_failed"
		}
		r.check(id, "inconclusive", code, map[string]any{"http_status": g.sample.Status, "upstream_error": g.sample.Error})
		return
	}
	d := g.images[0]
	res, code, ev := r.verify(d.data, d.mime())
	if code != "" {
		r.check(id, "inconclusive", code, ev)
		return
	}
	direct := provenance.Classify(res, r.opts.Model)
	r.prov.Baseline = &BaselineSummary{Level: direct.Level, Verdict: &direct}
	evidence := map[string]any{"endpoint_level": r.prov.Verdict.Level, "direct_level": direct.Level}
	switch {
	case !strong(direct.Level):
		r.check(id, "inconclusive", "baseline_no_signal", evidence)
	case strong(r.prov.Verdict.Level):
		r.check(id, "pass", "both_signed", evidence)
	default:
		r.check(id, "fail", "endpoint_signal_missing", evidence)
	}
}

func (r *runner) finishProvenance() {
	r.prov.VerifyCalls = r.verifyCalls
	switch {
	case !r.prov.Enabled:
		r.prov.Level = "off"
	case r.prov.Verdict != nil:
		r.prov.Level = r.prov.Verdict.Level
	case r.prov.Level == "off" || r.prov.Level == "":
		r.prov.Level, r.prov.UnavailableCode = "unavailable", firstNonEmpty(r.prov.UnavailableCode, "not_run")
	}
	r.report.Provenance = &r.prov
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
