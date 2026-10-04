package imagecheck

import (
	"fmt"
	"image"
	"net/http"
	"slices"
	"strings"

	"model-check/common"
	"model-check/pkg/media"
	"model-check/pkg/openaicheck"
)

// pixelProbe sends one prompt, then lets decide judge the decoded pixels.
func (r *runner) pixelProbe(id, prompt string, decide func(*gen, image.Image) (bool, string, map[string]any)) {
	g := r.probe(id, r.call, genRequest(r.body(prompt, nil), false), 1)
	r.keep(id, g)
	if !g.ok {
		r.judge(id, g, false, "", "invalid_response", nil)
		return
	}
	px, err := g.images[0].pixels()
	if err != nil {
		r.check(id, "inconclusive", "pixels_unavailable", map[string]any{"format": g.images[0].info.Format})
		return
	}
	pass, failCode, ev := decide(g, px)
	r.judge(id, g, pass, "ok", failCode, ev)
}

// worst folds several probes into one check: a failure anywhere wins, then an
// availability problem, then success.
type verdict struct {
	fail, unavailable string
	evidence          map[string]any
}

func (v *verdict) note(key string, val any) {
	if v.evidence == nil {
		v.evidence = map[string]any{}
	}
	v.evidence[key] = val
}

func (r *runner) finish(id string, v *verdict, okCode string) {
	switch {
	case v.fail != "":
		r.check(id, "fail", v.fail, v.evidence)
	case v.unavailable != "":
		r.check(id, "inconclusive", v.unavailable, v.evidence)
	default:
		r.check(id, "pass", okCode, v.evidence)
	}
}

// absorb records a failed request into v and reports whether the probe's
// images are usable.
func (v *verdict) absorb(g *gen, key string) bool {
	if g.ok {
		return true
	}
	code := g.sample.ErrorCode
	if code == "" {
		code = "invalid_response"
	}
	v.note(key+"_error", code)
	if openaicheck.IsAvailabilityCode(code) {
		if v.unavailable == "" {
			v.unavailable = code
		}
	} else if v.fail == "" {
		v.fail = code
	}
	return false
}

func (r *runner) paramSuite() {
	r.outputFormats()
	r.sizeMatrix()
	r.background()
	r.multiple()
	r.qualityLevels()
	r.streamPartial()
	r.invalidSize()
}

func (r *runner) outputFormats() {
	const id = "img_output_format"
	if !r.live(id) {
		return
	}
	v := &verdict{}
	for _, f := range extraFormats(r.prof) {
		t := r.pickColors(1)[0]
		g := r.probe(id, r.call, genRequest(r.body(r.solidPrompt(t), map[string]any{"output_format": f}), false), 1)
		r.keep(id, g)
		if !v.absorb(g, f) {
			continue
		}
		d := g.images[0]
		v.note(f, d.info.Format)
		if d.info.Format != f {
			if v.fail == "" {
				v.fail = "format_mismatch"
			}
			continue
		}
		r.byFormat[f] = d
		if f == "jpeg" {
			if px, err := d.pixels(); err == nil && media.AlphaStats(px).Channel && v.fail == "" {
				v.fail = "jpeg_has_alpha"
			}
		}
	}
	r.finish(id, v, "ok")
}

func (r *runner) sizeMatrix() {
	const id = "img_size_matrix"
	if !r.live(id) {
		return
	}
	v := &verdict{}
	for _, size := range matrixSizes(r.prof, r.opts.full()) {
		t := r.pickColors(1)[0]
		g := r.probe(id, r.call, genRequest(r.body(r.solidPrompt(t), map[string]any{"size": size}), false), 1)
		r.keep(id, g)
		if !v.absorb(g, size) {
			continue
		}
		d := g.images[0]
		w, h, _ := parseSize(size)
		v.note(size, fmt.Sprintf("%dx%d", d.info.Width, d.info.Height))
		switch {
		case d.info.Width == w && d.info.Height == h:
		case max(d.info.Width, d.info.Height) < max(w, h):
			if v.fail == "" || v.fail == "size_mismatch" {
				v.fail = "silently_downscaled"
			}
		default:
			if v.fail == "" {
				v.fail = "size_mismatch"
			}
		}
	}
	r.finish(id, v, "ok")
}

func (r *runner) background() {
	const id = "img_background"
	if !r.live(id) {
		return
	}
	prompt := "A single solid red circle in the center on a fully transparent background. No shadow, no glow, no border, no backdrop."
	g := r.probe(id, r.call, genRequest(r.body(prompt, map[string]any{"background": "transparent", "output_format": "png"}), false), 1)
	r.keep(id, g)
	if !g.ok {
		r.judge(id, g, false, "", "invalid_response", nil)
		return
	}
	px, err := g.images[0].pixels()
	if err != nil {
		r.check(id, "inconclusive", "pixels_unavailable", nil)
		return
	}
	a := media.AlphaStats(px)
	ev := map[string]any{"alpha_channel": a.Channel, "transparent_ratio": round1(a.TransparentRatio * 100)}
	code := "no_alpha_channel"
	if a.Channel {
		code = "not_transparent"
	}
	r.judge(id, g, a.Channel && a.TransparentRatio >= 0.05, "ok", code, ev)
}

func (r *runner) multiple() {
	const id = "img_n"
	if !r.live(id) {
		return
	}
	t := r.pickColors(1)[0]
	g := r.probe(id, r.call, genRequest(r.body(r.solidPrompt(t), map[string]any{"n": 2}), false), 2)
	r.keep(id, g)
	if !g.ok {
		r.judge(id, g, false, "", "invalid_response", nil)
		return
	}
	ev := map[string]any{"requested": 2, "returned": len(g.images)}
	switch {
	case len(g.images) != 2:
		r.check(id, "fail", "n_ignored", ev)
	case g.images[0].sha == g.images[1].sha:
		r.check(id, "fail", "duplicate_images", ev)
	default:
		r.check(id, "pass", "ok", ev)
	}
}

// qualityLevels compares file size across the profile's quality tiers using
// one prompt. Size is a weak proxy for effort, so this is only an observation.
func (r *runner) qualityLevels() {
	const id = "img_quality_levels"
	if !r.live(id) {
		return
	}
	prompt := "A detailed close-up photograph of a tabby cat sitting on a wooden table, soft window light, fine fur texture."
	v := &verdict{}
	var sizes []int
	for _, q := range r.prof.Qualities {
		g := r.probe(id, r.call, genRequest(r.body(prompt, map[string]any{"quality": q}), false), 1)
		r.keep(id, g)
		if !v.absorb(g, q) {
			continue
		}
		sizes = append(sizes, g.images[0].info.Bytes)
		v.note(q, g.images[0].info.Bytes)
	}
	if v.fail == "" && v.unavailable == "" && !slices.IsSorted(sizes) {
		v.fail = "quality_not_ordered"
	}
	r.finish(id, v, "quality_ordered")
}

func (r *runner) streamPartial() {
	const id = "img_stream_partial"
	if !r.live(id) {
		return
	}
	t := r.pickColors(1)[0]
	g := r.probe(id, r.call, genRequest(r.body(r.solidPrompt(t), map[string]any{"stream": true, "partial_images": 1}), true), 1)
	r.keep(id, g)
	if !g.ok {
		r.judge(id, g, false, "", "invalid_response", nil)
		return
	}
	ev := map[string]any{"partial_events": g.partials, "completed_events": g.completed}
	if g.sample.FirstEventMS != nil {
		ev["first_event_ms"] = *g.sample.FirstEventMS
	}
	switch {
	case len(g.resp.Events) == 0:
		r.check(id, "fail", "stream_not_supported", ev)
	case g.partials > 1:
		r.check(id, "fail", "too_many_partials", ev)
	default:
		r.check(id, "pass", "ok", ev)
	}
}

// invalidSize asks for a size no family accepts. A 4xx is the healthy answer;
// accepting it only suggests the endpoint does not validate parameters.
func (r *runner) invalidSize() {
	const id = "img_size_invalid"
	if !r.live(id) {
		return
	}
	t := r.pickColors(1)[0]
	req := genRequest(r.body(r.solidPrompt(t), map[string]any{"size": "999x999"}), false)
	req.ExpectError = true
	g := r.probe(id, r.call, req, 1)
	r.keep(id, g)
	ev := map[string]any{"http_status": g.sample.Status}
	switch {
	case g.sample.Status >= 400 && g.sample.Status < 500:
		r.check(id, "pass", "rejected", ev)
	case g.ok:
		ev["actual"] = fmt.Sprintf("%dx%d", g.images[0].info.Width, g.images[0].info.Height)
		r.check(id, "fail", "accepted_invalid_size", ev)
	default:
		code := g.sample.ErrorCode
		if code == "" {
			code = "invalid_response"
		}
		r.check(id, "inconclusive", code, ev)
	}
}

// errorShape sends a request with no prompt; the endpoint must refuse it with
// an OpenAI-style error object. It costs no image.
func (r *runner) errorShape() {
	const id = "img_error_shape"
	if !r.live(id) {
		return
	}
	req := genRequest(map[string]any{"model": r.opts.Model, "size": probeSize}, false)
	req.ExpectError = true
	g := r.probe(id, r.call, req, 0)
	ev := map[string]any{"http_status": g.sample.Status}
	switch {
	case g.ok:
		r.check(id, "fail", "accepted_invalid_request", ev)
	case g.sample.Status >= 400 && g.sample.Status < 500:
		var doc struct {
			Error map[string]any `json:"error"`
		}
		_ = common.Unmarshal(g.resp.Body, &doc)
		msg, _ := doc.Error["message"].(string)
		_, hasType := doc.Error["type"]
		if strings.TrimSpace(msg) == "" || !hasType {
			r.check(id, "fail", "invalid_error_shape", ev)
			return
		}
		r.check(id, "pass", "ok", ev)
	default:
		code := g.sample.ErrorCode
		if code == "" {
			code = "invalid_response"
		}
		r.check(id, "inconclusive", code, ev)
	}
}

// editSuite probes /v1/images/edits with synthetic fixtures.
func (r *runner) editSuite() {
	left, right := palette[2], palette[3] // blue | yellow, fixed so the protected strip is known
	fx := media.NewEditFixture(1024, left.C, right.C)
	files := func(withMask bool) []Part {
		parts := []Part{{Field: "image[]", Filename: "base.png", Mime: "image/png", Data: fx.Base}}
		if withMask {
			parts = append(parts, Part{Field: "mask", Filename: "mask.png", Mime: "image/png", Data: fx.Mask})
		}
		return parts
	}
	fields := func(prompt string) map[string]string {
		return map[string]string{"model": r.opts.Model, "prompt": prompt, "size": probeSize, "quality": probeQuality(r.prof), "output_format": "png", "n": "1"}
	}
	edit := func(prompt string, withMask bool) Request {
		return Request{Method: http.MethodPost, Path: "/v1/images/edits", Fields: fields(prompt), Files: files(withMask)}
	}
	if id := "img_edit_basic"; r.live(id) {
		g := r.probe(id, r.call, edit("Add one small solid red circle in the middle of the image. Keep everything else the same.", false), 1)
		r.keep(id, g)
		if !g.ok {
			r.judge(id, g, false, "", "invalid_response", nil)
		} else {
			d := g.images[0]
			ev := map[string]any{"requested": probeSize, "actual": fmt.Sprintf("%dx%d", d.info.Width, d.info.Height)}
			r.judge(id, g, d.info.Width == 1024 && d.info.Height == 1024, "ok", "size_mismatch", ev)
		}
	}
	if id := "img_edit_mask"; r.live(id) {
		g := r.probe(id, r.call, edit("Paint the transparent masked area solid red RGB(220, 40, 40). Do not change anything outside the mask.", true), 1)
		r.keep(id, g)
		if !g.ok {
			r.judge(id, g, false, "", "invalid_response", nil)
			return
		}
		out, err := g.images[0].pixels()
		base, _ := media.Decode(fx.Base)
		if err != nil || base == nil {
			r.check(id, "inconclusive", "pixels_unavailable", nil)
			return
		}
		diff := media.RegionDiff(base, out, fx.Protected)
		window := media.AvgColor(out, fx.Window)
		red := palette[0]
		ev := map[string]any{"protected_diff": round1(diff), "window_color": rgbText(window), "window_name": nearest(window)}
		switch {
		case diff > 25:
			r.judge(id, g, false, "", "mask_not_respected", ev)
		case nearest(window) != red.Name:
			r.judge(id, g, false, "", "edit_not_applied", ev)
		default:
			r.judge(id, g, true, "ok", "", ev)
		}
	}
}

// usageFields checks usage consistency across every successful sample.
func (r *runner) usageFields() {
	const id = "img_usage_fields"
	with, issues := 0, []string{}
	for _, s := range r.report.Samples {
		if s.Status != http.StatusOK || s.Usage.Total == nil && s.Usage.Input == nil && s.Usage.Output == nil {
			continue
		}
		with++
		issues = append(issues, s.UsageIssues...)
	}
	ev := map[string]any{"samples_with_usage": with}
	switch {
	case with == 0:
		r.check(id, "inconclusive", "usage_absent", ev)
	case len(issues) > 0:
		ev["issues"] = dedupe(issues)
		r.check(id, "fail", "usage_inconsistent", ev)
	default:
		r.check(id, "pass", "ok", ev)
	}
}

func (r *runner) performance() {
	const id = "img_performance"
	var ms []float64
	var worst int64
	for _, s := range r.report.Samples {
		if s.Kind == "image" && s.Status == http.StatusOK {
			ms = append(ms, float64(s.DurationMS))
			worst = max(worst, s.DurationMS)
		}
	}
	if len(ms) == 0 {
		r.check(id, "inconclusive", "no_samples", nil)
		return
	}
	r.check(id, "pass", "ok", map[string]any{"samples": len(ms), "median_ms": int64(media.Median(ms)), "max_ms": worst})
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	out := items[:0:0]
	for _, i := range items {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}
