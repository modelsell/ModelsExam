package imagecheck

import (
	"bytes"
	"context"
	"errors"
	mrand "math/rand/v2"
	"strings"
	"testing"

	"model-check/common"
	"model-check/pkg/provenance"
)

func seeded(n uint64) *mrand.Rand { return mrand.New(mrand.NewPCG(n, n+1)) }

func exec(t *testing.T, o Options, f *fakeImageAPI, deps Deps) (Report, *fakeImageAPI) {
	t.Helper()
	tr, _ := newFake(t, f)
	return run(context.Background(), o, tr, deps, nil, seeded(7)), f
}

func status(r Report, id string) (string, string) {
	for _, c := range r.Checks {
		if c.ID == id {
			return c.Status, c.Code
		}
	}
	return "missing", ""
}

func want(t *testing.T, r Report, id, st, code string) {
	t.Helper()
	gotSt, gotCode := status(r, id)
	if gotSt != st || (code != "" && gotCode != code) {
		t.Errorf("%s = %s/%s, want %s/%s", id, gotSt, gotCode, st, code)
	}
}

func wantAllPass(t *testing.T, r Report, except ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, e := range except {
		skip[e] = true
	}
	for _, c := range r.Checks {
		if c.Status == "skipped" && c.Code == "not_requested" {
			continue
		}
		if c.Status != "pass" && !skip[c.ID] {
			t.Errorf("%s = %s/%s evidence=%v", c.ID, c.Status, c.Code, c.Evidence)
		}
	}
}

func TestBudgets(t *testing.T) {
	cases := []struct {
		o                Options
		req, img, verify int
	}{
		{Options{Model: "gpt-image-2", Suite: "basic"}, 2, 1, 0},
		{Options{Model: "gpt-image-2"}, 11, 11, 0},
		{Options{Model: "gpt-image-2", Suite: "full"}, 19, 19, 0},
		{Options{Model: "gpt-image-2", Suite: "basic", Provenance: true}, 2, 1, 2},
		{Options{Model: "gpt-image-2", Suite: "full", Provenance: true}, 19, 19, 4},
		{Options{Model: "gpt-image-2", Suite: "full", Baseline: true}, 20, 20, 5},
		{Options{Model: "gpt-image-1", Suite: "full"}, 18, 18, 0},
		{Options{Model: "gpt-image-1", Suite: "standard"}, 11, 11, 0},
	}
	for _, c := range cases {
		b := MaxBudget(c.o)
		if b.MaxRequests != c.req || b.MaxImages != c.img || b.MaxVerifyCalls != c.verify {
			t.Errorf("%+v: got %+v want %d/%d/%d", c.o, b, c.req, c.img, c.verify)
		}
	}
}

func TestValidOptions(t *testing.T) {
	bad := []Options{
		{}, {Model: "m", Suite: "huge"}, {Model: strings.Repeat("x", 201)},
		{Model: "m", Profile: &Profile{NMax: 0, Qualities: []string{"low"}, Formats: []string{"png"}, CustomSizes: true}},
		{Model: "m", Profile: &Profile{NMax: 2, Qualities: []string{"low"}, Formats: []string{"jpeg"}, CustomSizes: true}},
		{Model: "m", Profile: &Profile{NMax: 2, Qualities: []string{"low"}, Formats: []string{"png"}}},
		{Model: "m", Profile: &Profile{NMax: 2, Qualities: []string{"low"}, Formats: []string{"png"}, FixedSizes: []string{"9999x9999"}}},
	}
	for i, o := range bad {
		if ValidOptions(o) {
			t.Errorf("bad[%d] accepted", i)
		}
	}
	if !ValidOptions(Options{Model: "gpt-image-2", Suite: "full", Provenance: true}) {
		t.Fatal("good options rejected")
	}
}

func TestGoodModelBasicAndStandard(t *testing.T) {
	for _, suite := range []string{"basic", "standard"} {
		o := Options{Model: "gpt-image-2", Suite: suite}
		r, f := exec(t, o, &fakeImageAPI{}, Deps{})
		wantAllPass(t, r)
		b := MaxBudget(o)
		if r.RequestsRun != b.MaxRequests || r.ImagesBilled != b.MaxImages || f.requests != b.MaxRequests {
			t.Errorf("%s: ran %d/%d images, fake saw %d, budget %+v", suite, r.RequestsRun, r.ImagesBilled, f.requests, b)
		}
		if r.Score == nil || *r.Score != 100 || r.StopReason != "" {
			t.Errorf("%s: score=%v stop=%q", suite, r.Score, r.StopReason)
		}
		if r.Provenance == nil || r.Provenance.Level != "off" {
			t.Errorf("%s: provenance %+v", suite, r.Provenance)
		}
	}
}

func TestGoodModelFull(t *testing.T) {
	o := Options{Model: "gpt-image-2", Suite: "full"}
	r, f := exec(t, o, &fakeImageAPI{}, Deps{})
	wantAllPass(t, r)
	if b := MaxBudget(o); r.RequestsRun != b.MaxRequests || f.requests != b.MaxRequests {
		t.Errorf("ran %d, fake saw %d, budget %+v", r.RequestsRun, f.requests, b)
	}
	if len(r.Images) == 0 || r.Images[0].Thumb == "" {
		t.Fatal("no thumbnails")
	}
	n := 0
	for _, im := range r.Images {
		if im.Thumb != "" {
			n++
		}
	}
	if n > maxThumbs {
		t.Fatalf("%d thumbnails", n)
	}
	raw, _ := common.Marshal(r)
	StripThumbs(&r)
	stripped, _ := common.Marshal(r)
	if len(stripped) >= len(raw) || bytes.Contains(stripped, []byte("data:image/jpeg")) {
		t.Fatal("thumbnails not stripped")
	}
	if md := Markdown(r); !strings.Contains(md, "img_generate_basic") || strings.Contains(md, "data:image") {
		t.Fatal("markdown")
	}
}

func TestMisbehavingEndpoints(t *testing.T) {
	full := Options{Model: "gpt-image-2", Suite: "full"}
	r, _ := exec(t, full, &fakeImageAPI{downscale: true}, Deps{})
	want(t, r, "img_size_exact", "fail", "silently_downscaled")
	want(t, r, "img_size_matrix", "fail", "silently_downscaled")
	want(t, r, "img_edit_basic", "pass", "")

	r, _ = exec(t, full, &fakeImageAPI{ignoreN: true}, Deps{})
	want(t, r, "img_n", "fail", "n_ignored")

	r, _ = exec(t, full, &fakeImageAPI{noAlpha: true}, Deps{})
	want(t, r, "img_background", "fail", "not_transparent")

	r, _ = exec(t, full, &fakeImageAPI{wrongColor: true}, Deps{})
	want(t, r, "img_solid_color", "fail", "color_mismatch")
	want(t, r, "img_split_layout", "fail", "layout_mismatch")
	want(t, r, "img_centered_shape", "fail", "layout_mismatch")

	r, _ = exec(t, full, &fakeImageAPI{pngOnly: true}, Deps{})
	want(t, r, "img_output_format", "fail", "format_mismatch")

	r, _ = exec(t, full, &fakeImageAPI{noStream: true}, Deps{})
	want(t, r, "img_stream_partial", "fail", "stream_not_supported")

	r, _ = exec(t, full, &fakeImageAPI{acceptInvalidSize: true}, Deps{})
	want(t, r, "img_size_invalid", "inconclusive", "accepted_invalid_size")
	if r.Score == nil || *r.Score != 100 {
		t.Errorf("an observation must not move the score: %v", r.Score)
	}

	r, _ = exec(t, full, &fakeImageAPI{ignoreMask: true}, Deps{})
	want(t, r, "img_edit_mask", "inconclusive", "mask_not_respected")

	r, _ = exec(t, full, &fakeImageAPI{missingPromptAs200: true}, Deps{})
	want(t, r, "img_error_shape", "fail", "accepted_invalid_request")
}

func TestAvailabilityFailureStopsRun(t *testing.T) {
	r, f := exec(t, Options{Model: "gpt-image-2", Suite: "full", Provenance: true}, &fakeImageAPI{status: 401}, Deps{Verify: func(context.Context, []byte, string) (*provenance.Result, error) {
		t.Error("verifier must not be called when nothing was generated")
		return nil, nil
	}})
	want(t, r, "img_generate_basic", "inconclusive", "unauthorized")
	want(t, r, "img_solid_color", "skipped", "run_stopped")
	want(t, r, "img_error_shape", "skipped", "run_stopped")
	if r.StopReason != "unauthorized" || r.StopProbe != "img_generate_basic" || f.requests != 1 {
		t.Errorf("stop=%q probe=%q requests=%d", r.StopReason, r.StopProbe, f.requests)
	}
	if r.Score != nil {
		t.Errorf("score without decided assertions: %v", *r.Score)
	}
	if r.Provenance.Level != "unavailable" {
		t.Errorf("provenance %+v", r.Provenance)
	}
}

func TestRejectedBaselineDoesNotStop(t *testing.T) {
	// A 400 on the first request is a compatibility finding, not an outage.
	r, _ := exec(t, Options{Model: "m", Suite: "basic"}, &fakeImageAPI{status: 400}, Deps{})
	want(t, r, "img_generate_basic", "fail", "request_rejected")
	if r.StopReason != "" {
		t.Fatalf("stop %q", r.StopReason)
	}
	want(t, r, "img_solid_color", "skipped", "baseline_failed")
}

// verifier answers like OpenAI's service: images carrying the marker are signed.
func markerVerifier(model string, calls *int) Verifier {
	return func(_ context.Context, data []byte, _ string) (*provenance.Result, error) {
		*calls++
		if bytes.Contains(data, []byte(signalMarker)) {
			return &provenance.Result{
				C2PA:    &provenance.C2PA{Outcome: "detected", ValidationState: "trusted", Issuer: "OpenAI", Model: model},
				SynthID: &provenance.SynthID{Outcome: "detected"},
			}, nil
		}
		return &provenance.Result{
			C2PA:    &provenance.C2PA{Outcome: "not_detected", ValidationState: "not_present"},
			SynthID: &provenance.SynthID{Outcome: "not_detected"},
		}, nil
	}
}

func TestProvenanceSigned(t *testing.T) {
	calls := 0
	o := Options{Model: "gpt-image-2", Suite: "full", Provenance: true}
	r, _ := exec(t, o, &fakeImageAPI{signed: true}, Deps{Verify: markerVerifier("gpt-image-2-2026-04-21", &calls)})
	wantAllPass(t, r)
	if calls != MaxBudget(o).MaxVerifyCalls || r.Provenance.VerifyCalls != calls {
		t.Errorf("verify calls %d budget %+v summary %d", calls, MaxBudget(o), r.Provenance.VerifyCalls)
	}
	p := r.Provenance
	if p.Level != "trusted" || p.ControlOK == nil || !*p.ControlOK || p.Verdict == nil || !p.Verdict.SynthID || len(p.Formats) != 3 {
		t.Fatalf("summary %+v", p)
	}
}

func TestProvenanceUnsignedProvesNothing(t *testing.T) {
	calls := 0
	r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "full", Provenance: true}, &fakeImageAPI{}, Deps{Verify: markerVerifier("", &calls)})
	want(t, r, "img_prov_control", "pass", "control_clean")
	want(t, r, "img_prov_c2pa", "inconclusive", "c2pa_not_detected")
	want(t, r, "img_prov_synthid", "inconclusive", "synthid_not_detected")
	want(t, r, "img_prov_model_match", "inconclusive", "no_model_claim")
	want(t, r, "img_prov_format_matrix", "inconclusive", "signal_lost")
	if r.Provenance.Level != "none" {
		t.Fatalf("level %q", r.Provenance.Level)
	}
	if r.Score == nil || *r.Score != 100 {
		t.Fatalf("provenance must not touch the score: %v", r.Score)
	}
	if r.Summary["fail"] != 0 {
		t.Fatalf("summary %v", r.Summary)
	}
}

func TestProvenanceModelMismatchFails(t *testing.T) {
	calls := 0
	r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "basic", Provenance: true}, &fakeImageAPI{signed: true}, Deps{Verify: markerVerifier("gpt-image-1", &calls)})
	want(t, r, "img_prov_c2pa", "pass", "trusted")
	want(t, r, "img_prov_model_match", "fail", "model_mismatch")
	if r.Score == nil || *r.Score != 100 {
		t.Fatalf("score %v", r.Score)
	}
}

func TestProvenanceWatermarkOnly(t *testing.T) {
	v := func(_ context.Context, data []byte, _ string) (*provenance.Result, error) {
		res := &provenance.Result{C2PA: &provenance.C2PA{Outcome: "not_detected", ValidationState: "not_present"}, SynthID: &provenance.SynthID{Outcome: "not_detected"}}
		if bytes.Contains(data, []byte(signalMarker)) {
			res.SynthID.Outcome = "detected"
		}
		return res, nil
	}
	r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "basic", Provenance: true}, &fakeImageAPI{signed: true}, Deps{Verify: v})
	want(t, r, "img_prov_c2pa", "inconclusive", "c2pa_not_detected")
	want(t, r, "img_prov_synthid", "pass", "synthid_detected")
	if r.Provenance.Level != "synthid" {
		t.Fatalf("level %q", r.Provenance.Level)
	}
}

func TestProvenanceBrokenControl(t *testing.T) {
	always := func(context.Context, []byte, string) (*provenance.Result, error) {
		return &provenance.Result{SynthID: &provenance.SynthID{Outcome: "detected"}}, nil
	}
	r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "basic", Provenance: true}, &fakeImageAPI{}, Deps{Verify: always})
	want(t, r, "img_prov_control", "fail", "control_detected")
	want(t, r, "img_prov_c2pa", "inconclusive", "control_failed")
	want(t, r, "img_prov_model_match", "inconclusive", "control_failed")
	if r.Provenance.Level != "unavailable" || r.Provenance.UnavailableCode != "control_failed" {
		t.Fatalf("%+v", r.Provenance)
	}
}

func TestProvenanceServiceUnavailable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{&provenance.APIError{Status: 429, RetryAfter: "9"}, "rate_limited"},
		{&provenance.APIError{Status: 404}, "no_access"},
		{errors.New("context deadline exceeded"), "timeout"},
	} {
		calls := 0
		v := func(context.Context, []byte, string) (*provenance.Result, error) { calls++; return nil, tc.err }
		r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "full", Provenance: true}, &fakeImageAPI{signed: true}, Deps{Verify: v})
		for _, id := range provIDs {
			want(t, r, id, "inconclusive", tc.code)
		}
		want(t, r, "img_prov_format_matrix", "inconclusive", "relay_unverified")
		if calls != 1 {
			t.Errorf("%s: service kept being called (%d)", tc.code, calls)
		}
		if r.Provenance.Level != "unavailable" || r.Provenance.UnavailableCode != tc.code {
			t.Errorf("%+v", r.Provenance)
		}
		if r.Summary["fail"] != 0 {
			t.Errorf("an outage must not fail anything: %v", r.Summary)
		}
	}
}

func TestProvenanceWithoutVerifier(t *testing.T) {
	r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "basic", Provenance: true}, &fakeImageAPI{}, Deps{})
	want(t, r, "img_prov_control", "inconclusive", "verifier_not_configured")
}

func TestBaselineComparison(t *testing.T) {
	cases := []struct {
		name           string
		relay, direct  bool
		status, code   string
		level, baseLvl string
	}{
		{"both signed", true, true, "pass", "both_signed", "trusted", "trusted"},
		{"relay lost the signal", false, true, "fail", "endpoint_signal_missing", "none", "trusted"},
		{"direct has no signal", false, false, "inconclusive", "baseline_no_signal", "none", "none"},
	}
	for _, tc := range cases {
		calls := 0
		direct, _ := newFake(t, &fakeImageAPI{signed: tc.direct})
		o := Options{Model: "gpt-image-2", Suite: "basic", Baseline: true}
		r, _ := exec(t, o, &fakeImageAPI{signed: tc.relay}, Deps{Verify: markerVerifier("gpt-image-2", &calls), Baseline: direct})
		want(t, r, "img_prov_baseline", tc.status, tc.code)
		if r.Provenance.Level != tc.level || r.Provenance.Baseline == nil || r.Provenance.Baseline.Level != tc.baseLvl {
			t.Errorf("%s: %+v", tc.name, r.Provenance)
		}
		if b := MaxBudget(o); r.RequestsRun != b.MaxRequests || calls != b.MaxVerifyCalls {
			t.Errorf("%s: requests %d calls %d budget %+v", tc.name, r.RequestsRun, calls, b)
		}
		var baselineSamples int
		for _, s := range r.Samples {
			if s.Kind == "baseline_image" {
				baselineSamples++
			}
		}
		if baselineSamples != 1 {
			t.Errorf("%s: %d baseline samples", tc.name, baselineSamples)
		}
	}
	r, _ := exec(t, Options{Model: "gpt-image-2", Suite: "basic", Baseline: true}, &fakeImageAPI{signed: true}, Deps{Verify: markerVerifier("gpt-image-2", new(int))})
	want(t, r, "img_prov_baseline", "inconclusive", "baseline_not_configured")
}

func TestProbesAreRandomizedPerRun(t *testing.T) {
	prompts := func(seed uint64) []string {
		tr, srv := newFake(t, &fakeImageAPI{})
		_ = srv
		f := &fakeImageAPI{}
		tr2, _ := newFake(t, f)
		_ = tr
		run(context.Background(), Options{Model: "gpt-image-2", Suite: "standard"}, tr2, Deps{}, nil, seeded(seed))
		return f.prompts
	}
	a, a2, b := prompts(1), prompts(1), prompts(2)
	if strings.Join(a, "|") != strings.Join(a2, "|") {
		t.Fatal("same seed must give the same probes")
	}
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	if same == len(a) {
		t.Fatal("different seeds produced identical prompts")
	}
}

func TestURLResponsesNeedFetcher(t *testing.T) {
	var gotURL string
	tr := func(context.Context, Request) (Response, error) {
		return Response{Status: 200, Body: []byte(`{"data":[{"url":"https://cdn.example/x.png"}]}`)}, nil
	}
	r := run(context.Background(), Options{Model: "gpt-image-2", Suite: "basic"}, tr, Deps{}, nil, seeded(3))
	want(t, r, "img_generate_basic", "fail", "invalid_response")
	fetch := func(_ context.Context, u string) ([]byte, error) { gotURL = u; return nil, errors.New("blocked") }
	r = run(context.Background(), Options{Model: "gpt-image-2", Suite: "basic"}, tr, Deps{Fetch: fetch}, nil, seeded(3))
	if gotURL != "https://cdn.example/x.png" {
		t.Fatalf("fetcher got %q", gotURL)
	}
	want(t, r, "img_generate_basic", "fail", "invalid_response")
	if len(r.Samples) == 0 || len(r.Samples[0].ValidationErrors) == 0 || !strings.Contains(r.Samples[0].ValidationErrors[0], "url_fetch_failed") {
		t.Fatalf("samples %+v", r.Samples)
	}
}

func TestCancelledRunIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr, _ := newFake(t, &fakeImageAPI{})
	r := run(ctx, Options{Model: "gpt-image-2", Suite: "standard"}, tr, Deps{}, nil, seeded(1))
	if !r.Cancelled || r.Summary["fail"] != 0 {
		t.Fatalf("cancelled=%v summary=%v", r.Cancelled, r.Summary)
	}
}
