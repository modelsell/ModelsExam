package imagecheck

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"strings"
	"time"

	"model-check/common"
	"model-check/pkg/media"
	"model-check/pkg/openaicheck"
)

// decoded is one image received from an upstream.
type decoded struct {
	data []byte
	info media.Info
	sha  string
	img  image.Image // nil until pixels are needed, and for WebP
	err  error
}

func (d *decoded) pixels() (image.Image, error) {
	if d.img == nil && d.err == nil {
		d.img, d.err = media.Decode(d.data)
	}
	return d.img, d.err
}

func (d *decoded) mime() string {
	switch d.info.Format {
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	}
	return "image/png"
}

// gen is the outcome of one generation or edit request.
type gen struct {
	sample Sample
	resp   Response
	images []*decoded
	issues []string
	ok     bool // HTTP 200, no transport error, at least one decodable image, no contract issue
	// stream bookkeeping
	partials, completed int
}

type runner struct {
	ctx     context.Context
	call    Transport
	deps    Deps
	observe func(Event)
	opts    Options
	prof    Profile
	report  Report
	plan    map[string]PlanItem
	rng     *mrand.Rand

	requests, imagesAsked, verifyCalls int
	thumbs                             int

	basic *gen // the img_generate_basic result, once it exists
	// images kept for later provenance checks, by output format
	byFormat map[string]*decoded

	verifyBlock string // availability code once the verification service stops answering
	prov        ProvenanceSummary
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func cryptoSeed() (uint64, uint64) {
	var b [16]byte
	_, _ = rand.Read(b[:])
	var s1, s2 uint64
	for i := 0; i < 8; i++ {
		s1, s2 = s1<<8|uint64(b[i]), s2<<8|uint64(b[8+i])
	}
	return s1, s2
}

// Run executes the selected suite. Requests are never retried.
func Run(ctx context.Context, options Options, transport Transport, deps Deps, observe func(Event)) Report {
	s1, s2 := cryptoSeed()
	return run(ctx, options, transport, deps, observe, mrand.New(mrand.NewPCG(s1, s2)))
}

func run(ctx context.Context, options Options, transport Transport, deps Deps, observe func(Event), rng *mrand.Rand) Report {
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()
	start := time.Now()
	options = options.normalized()
	r := &runner{ctx: ctx, call: transport, deps: deps, observe: observe, opts: options, prof: *options.Profile, plan: map[string]PlanItem{}, rng: rng, byFormat: map[string]*decoded{}}
	r.report = Report{Version: ReportVersion, ID: newID(), Provider: Provider, Model: options.Model, StartedAt: start.UTC().Format(time.RFC3339), Checks: []Check{}, Samples: []Sample{}, Images: []ImageRef{}, Summary: map[string]int{}}
	r.report.Options, r.report.Profile = &options, options.Profile
	r.report.Plan = Plan(options)
	for _, item := range r.report.Plan {
		r.plan[item.ID] = item
	}
	b := MaxBudget(options)
	r.report.Limits = &Limits{ProbeTimeoutSeconds: int(ProbeTimeout / time.Second), RunTimeoutSeconds: int(RunTimeout / time.Second), MaxRequests: b.MaxRequests, MaxImages: b.MaxImages, MaxVerifyCalls: b.MaxVerifyCalls}
	r.prov = ProvenanceSummary{Enabled: options.Provenance, Level: "off"}
	r.emit(Event{Type: "start", Report: &r.report})

	if r.generateBasic() {
		r.provenanceBasic()
		r.contentSuite()
		r.paramSuite()
		r.editSuite()
	}
	r.errorShape()
	r.baselineCompare()
	r.provenanceFormats()
	r.usageFields()
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
		case r.basic == nil || !r.basic.ok:
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
	r.report.RequestsRun, r.report.ImagesBilled = r.requests, r.imagesAsked
	r.finishProvenance()
	Finalize(&r.report)
	r.emit(Event{Type: "done", Report: &r.report})
	return r.report
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
		kind = KindAssertion
	}
	if kind == KindObservation && state == "fail" {
		state = "inconclusive"
	}
	if r.ctx.Err() != nil && state == "fail" {
		state, code = "skipped", "cancelled"
	}
	c := Check{ID: id, Stage: item.Stage, Kind: kind, Status: state, Code: code, Evidence: evidence}
	r.report.Checks = append(r.report.Checks, c)
	r.emit(Event{Type: "check", Check: &c})
}

// judge turns a generation result into a check. A transport or access problem
// is availability evidence and stays inconclusive; a refused or malformed
// response is a compatibility failure.
func (r *runner) judge(id string, g *gen, pass bool, okCode, failCode string, evidence map[string]any) {
	if evidence == nil {
		evidence = map[string]any{}
	}
	if !g.ok {
		code := g.sample.ErrorCode
		if code == "" {
			code = "invalid_response"
		}
		evidence["http_status"] = g.sample.Status
		if len(g.issues) > 0 {
			evidence["issues"] = g.issues
		}
		if g.sample.Error != "" {
			evidence["upstream_error"] = g.sample.Error
		}
		if openaicheck.IsAvailabilityCode(code) {
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

var headerNames = []string{"x-request-id", "openai-processing-ms", "openai-version", "openai-model", "retry-after", "x-ratelimit-remaining-requests", "server", "via", "cf-ray"}

// body builds a generation request body from the profile defaults.
func (r *runner) body(prompt string, extra map[string]any) map[string]any {
	b := map[string]any{"model": r.opts.Model, "prompt": prompt, "size": probeSize, "quality": probeQuality(r.prof), "output_format": "png", "n": 1}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func genRequest(body map[string]any, stream bool) Request {
	return Request{Method: http.MethodPost, Path: "/v1/images/generations", Body: body, Stream: stream}
}

// probe sends one request through t, records the sample and parses images.
// wantImages is what the request asks for and is charged to the budget.
func (r *runner) probe(id string, t Transport, req Request, wantImages int) *gen {
	g := &gen{}
	kind := "image"
	if id == "img_prov_baseline" {
		kind = "baseline_image"
	}
	if r.ctx.Err() != nil {
		g.sample = Sample{Probe: id, Kind: kind, ErrorCode: "cancelled"}
		return g
	}
	ctx, cancel := context.WithTimeout(r.ctx, ProbeTimeout)
	defer cancel()
	r.emit(Event{Type: "probe_start", Probe: id})
	r.requests++
	r.imagesAsked += wantImages
	started := time.Now()
	resp, err := t(ctx, req)
	g.resp = resp
	s := Sample{Probe: id, Kind: kind, Method: req.Method, Path: req.Path, Status: resp.Status, DurationMS: time.Since(started).Milliseconds(), FirstEventMS: resp.FirstEventMS, Stream: req.Stream, ErrorCode: resp.ErrorCode, Headers: map[string]string{}}
	for _, name := range headerNames {
		if v := resp.Header.Get(name); v != "" {
			s.Headers[name] = clip(v, 512)
		}
	}
	if s.ErrorCode == "" && (!req.ExpectError || resp.Status >= 500 || err != nil) {
		s.ErrorCode = openaicheck.FailureCode(resp.Status, err)
	}
	if err != nil {
		s.Error = clip(err.Error(), 400)
	} else if resp.Status != http.StatusOK && !req.ExpectError {
		s.Error = upstreamError(resp.Body)
	}
	httpOK := err == nil && resp.Status == http.StatusOK
	if httpOK {
		r.parseImages(g, &s)
	}
	g.ok = httpOK && len(g.issues) == 0 && len(g.images) > 0
	if !req.ExpectError {
		valid := g.ok
		s.Valid, s.ValidationErrors = &valid, g.issues
	}
	if httpOK && !g.ok && s.ErrorCode == "" {
		s.ErrorCode = "invalid_response"
	}
	g.sample = s
	r.report.Samples = append(r.report.Samples, s)
	r.emit(Event{Type: "sample", Sample: &s})
	return g
}

type imageItem struct {
	B64 string `json:"b64_json"`
	URL string `json:"url"`
}

type usagePayload struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
	Total  *int64 `json:"total_tokens"`
}

func (r *runner) parseImages(g *gen, s *Sample) {
	var items []imageItem
	var usage *usagePayload
	if len(g.resp.Events) > 0 {
		for _, ev := range g.resp.Events {
			var e struct {
				Type string `json:"type"`
				imageItem
				Usage *usagePayload `json:"usage"`
			}
			if common.Unmarshal(ev.Data, &e) != nil {
				g.issues = append(g.issues, "stream_frame_not_json")
				continue
			}
			kind := e.Type
			if kind == "" {
				kind = ev.Name
			}
			switch {
			case strings.Contains(kind, "partial_image"):
				g.partials++
			case strings.Contains(kind, "completed"):
				g.completed++
				items = append(items, e.imageItem)
				usage = e.Usage
			}
		}
		if g.completed == 0 {
			g.issues = append(g.issues, "no_completed_event")
		}
	} else {
		var doc struct {
			Data  []imageItem   `json:"data"`
			Usage *usagePayload `json:"usage"`
		}
		if err := common.Unmarshal(g.resp.Body, &doc); err != nil {
			g.issues = append(g.issues, "body_not_json")
			return
		}
		if len(doc.Data) == 0 {
			g.issues = append(g.issues, "data_empty")
		}
		items, usage = doc.Data, doc.Usage
	}
	for i, it := range items {
		raw, issue := r.imageBytes(it)
		if issue != "" {
			g.issues = append(g.issues, fmt.Sprintf("image_%d_%s", i, issue))
			continue
		}
		info, err := media.Inspect(raw)
		if err != nil {
			g.issues = append(g.issues, fmt.Sprintf("image_%d_undecodable", i))
			continue
		}
		sum := sha256.Sum256(raw)
		g.images = append(g.images, &decoded{data: raw, info: info, sha: hex.EncodeToString(sum[:])})
	}
	if usage != nil {
		s.Usage = Usage{Input: usage.Input, Output: usage.Output, Total: usage.Total}
		s.UsageIssues = usageIssues(usage)
	}
}

func (r *runner) imageBytes(it imageItem) ([]byte, string) {
	switch {
	case it.B64 != "":
		raw, err := base64.StdEncoding.DecodeString(it.B64)
		if err != nil {
			if raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(it.B64, "=")); err != nil {
				return nil, "bad_base64"
			}
		}
		if len(raw) > MaxImageBytes {
			return nil, "too_large"
		}
		return raw, ""
	case it.URL != "":
		if r.deps.Fetch == nil {
			return nil, "url_not_fetchable"
		}
		ctx, cancel := context.WithTimeout(r.ctx, 60*time.Second)
		defer cancel()
		raw, err := r.deps.Fetch(ctx, it.URL)
		if err != nil {
			return nil, "url_fetch_failed"
		}
		if len(raw) > MaxImageBytes {
			return nil, "too_large"
		}
		return raw, ""
	}
	return nil, "no_image_field"
}

func usageIssues(u *usagePayload) []string {
	var issues []string
	if u.Input == nil || u.Output == nil || u.Total == nil {
		return []string{"usage_field_missing"}
	}
	if *u.Input < 0 || *u.Output < 0 || *u.Total < 0 {
		issues = append(issues, "usage_negative")
	}
	if *u.Total != *u.Input+*u.Output {
		issues = append(issues, "usage_total_mismatch")
	}
	return issues
}

// keep records the images of a probe for the gallery and for later stages.
func (r *runner) keep(probe string, g *gen) {
	for i, d := range g.images {
		ref := ImageRef{Probe: probe, Index: i, Format: d.info.Format, Width: d.info.Width, Height: d.info.Height, Bytes: d.info.Bytes, SHA256: d.sha}
		if r.thumbs < maxThumbs {
			if px, err := d.pixels(); err == nil {
				if data, _, _, err := media.Thumbnail(px, 192, 70); err == nil {
					ref.Thumb = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
					r.thumbs++
				}
			}
		}
		r.report.Images = append(r.report.Images, ref)
	}
}

func clip(s string, n int) string {
	if rs := []rune(s); len(rs) > n {
		return string(rs[:n])
	}
	return s
}

func upstreamError(body []byte) string {
	var doc struct {
		Error struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if common.Unmarshal(body, &doc) != nil {
		return ""
	}
	msg := doc.Error.Message
	if msg == "" {
		msg = doc.Message
	}
	if c, ok := doc.Error.Code.(string); ok && c != "" {
		msg = "[" + c + "] " + msg
	}
	return clip(msg, 240)
}

// Finalize recomputes the summary and score from the recorded checks.
func Finalize(report *Report) {
	report.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, c := range report.Checks {
		report.Summary[c.Status]++
	}
	report.Score = Score(*report)
}

// Score is the share of passed assertions among decided assertions.
// Observations, provenance evidence, inconclusive and skipped checks never move
// it, and it is nil when nothing was decided. It rates API compatibility, not
// model authenticity.
func Score(report Report) *int {
	pass, fail := 0, 0
	for _, c := range report.Checks {
		if c.Kind != KindAssertion {
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

func round1(f float64) float64 { return math.Round(f*10) / 10 }
