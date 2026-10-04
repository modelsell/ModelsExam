package imagecheck

import (
	"slices"
	"strings"
)

// Stage names group checks in the report.
const (
	StageGenerate    = "generate"
	StageContent     = "content"
	StageParams      = "params"
	StageEdits       = "edits"
	StageProvenance  = "provenance"
	StageReliability = "reliability"
)

// DefaultProfile guesses the capability profile from the model name. Unknown
// names get the gpt-image-1 shape, the most widely copied one.
func DefaultProfile(model string) Profile {
	name := strings.ToLower(model)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	p := Profile{
		Family:      "generic",
		FixedSizes:  []string{"1024x1024", "1024x1536", "1536x1024"},
		Qualities:   []string{"low", "medium", "high"},
		Formats:     []string{"png", "jpeg", "webp"},
		Transparent: true, Stream: true, Edits: true, Mask: true, NMax: 10,
	}
	switch {
	case strings.HasPrefix(name, "gpt-image-2"):
		p.Family, p.FixedSizes, p.CustomSizes = "gpt-image-2", nil, true
	case strings.HasPrefix(name, "gpt-image"):
		p.Family = "gpt-image-1"
	}
	return p
}

// ValidOptions rejects unknown enum values and absurd profiles before any
// upstream request.
func ValidOptions(o Options) bool {
	switch o.Suite {
	case "", SuiteBasic, SuiteStandard, SuiteFull:
	default:
		return false
	}
	if o.Model == "" || len(o.Model) > 200 {
		return false
	}
	if p := o.Profile; p != nil {
		if len(p.Family) > 40 || p.NMax < 1 || p.NMax > 10 || len(p.Qualities) == 0 || len(p.Qualities) > 8 || len(p.Formats) == 0 || len(p.Formats) > 4 || len(p.FixedSizes) > 12 {
			return false
		}
		for _, list := range [][]string{p.Qualities, p.Formats} {
			for _, s := range list {
				if s == "" || len(s) > 16 {
					return false
				}
			}
		}
		for _, s := range p.FixedSizes {
			if !validSize(s) {
				return false
			}
		}
		if !slices.Contains(p.Formats, "png") {
			return false // content probes decode PNG pixels
		}
		if !p.CustomSizes && len(p.FixedSizes) == 0 {
			return false
		}
	}
	return true
}

func validSize(s string) bool {
	w, h, ok := parseSize(s)
	if !ok {
		return false
	}
	return w >= 256 && h >= 256 && w <= 3840 && h <= 3840
}

func (o Options) normalized() Options {
	if o.Suite == "" {
		o.Suite = SuiteStandard
	}
	if o.Baseline {
		o.Provenance = true
	}
	if o.Profile == nil {
		p := DefaultProfile(o.Model)
		o.Profile = &p
	}
	return o
}

func (o Options) std() bool  { return o.Suite != SuiteBasic }
func (o Options) full() bool { return o.Suite == SuiteFull }

// probeSize is the square size used by every probe that is not about size.
const probeSize = "1024x1024"

// matrixSizes lists the sizes the size matrix requests, besides probeSize.
func matrixSizes(p Profile, full bool) []string {
	var sizes []string
	if p.CustomSizes {
		sizes = []string{"1536x1024", "1280x768"}
		if full {
			sizes = append(sizes, "1024x1536")
		}
		return sizes
	}
	for _, s := range p.FixedSizes {
		if s != probeSize {
			sizes = append(sizes, s)
		}
	}
	if !full && len(sizes) > 2 {
		sizes = sizes[:2]
	}
	return sizes
}

// extraFormats are the non-PNG formats the output-format probe requests.
func extraFormats(p Profile) []string {
	var out []string
	for _, f := range p.Formats {
		if f == "jpeg" || f == "webp" {
			out = append(out, f)
		}
	}
	return out
}

func probeQuality(p Profile) string {
	if slices.Contains(p.Qualities, "low") {
		return "low"
	}
	return p.Qualities[0]
}

// Plan is persisted with the report so later releases cannot retroactively
// add unexecuted checks to historical reports.
func Plan(options Options) []PlanItem {
	options = options.normalized()
	p := *options.Profile
	std, full, prov := options.std(), options.full(), options.Provenance
	var plan []PlanItem
	add := func(stage, kind string, selected bool, ids ...string) {
		for _, id := range ids {
			plan = append(plan, PlanItem{ID: id, Stage: stage, Kind: kind, Selected: selected})
		}
	}
	add(StageGenerate, KindAssertion, true, "img_generate_basic", "img_response_shape", "img_size_exact", "img_error_shape")
	add(StageContent, KindAssertion, true, "img_solid_color")
	add(StageContent, KindAssertion, std, "img_split_layout", "img_centered_shape", "img_count_shapes")
	add(StageParams, KindAssertion, std && len(extraFormats(p)) > 0, "img_output_format")
	add(StageParams, KindAssertion, std && len(matrixSizes(p, full)) > 0, "img_size_matrix")
	add(StageParams, KindAssertion, std && p.Transparent, "img_background")
	add(StageParams, KindAssertion, std && p.NMax >= 2, "img_n")
	add(StageParams, KindObservation, full && len(p.Qualities) > 1, "img_quality_levels")
	add(StageParams, KindAssertion, full && p.Stream, "img_stream_partial")
	add(StageParams, KindObservation, full, "img_size_invalid")
	add(StageEdits, KindAssertion, full && p.Edits, "img_edit_basic")
	add(StageEdits, KindObservation, full && p.Edits && p.Mask, "img_edit_mask")
	add(StageProvenance, KindProvenance, prov, "img_prov_control", "img_prov_c2pa", "img_prov_synthid", "img_prov_model_match")
	add(StageProvenance, KindProvenance, prov && full && len(extraFormats(p)) > 0, "img_prov_format_matrix")
	add(StageProvenance, KindProvenance, options.Baseline, "img_prov_baseline")
	add(StageReliability, KindObservation, true, "img_usage_fields", "img_performance")
	return plan
}

// Budget is the exact upper bound of what a run can spend.
type Budget struct {
	MaxRequests    int `json:"max_requests"`
	MaxImages      int `json:"max_images"`
	MaxVerifyCalls int `json:"max_verify_calls"`
}

// MaxBudget computes the bound from the plan, before anything is sent.
func MaxBudget(options Options) Budget {
	options = options.normalized()
	p := *options.Profile
	var b Budget
	spend := func(req, img, verify int) {
		b.MaxRequests, b.MaxImages, b.MaxVerifyCalls = b.MaxRequests+req, b.MaxImages+img, b.MaxVerifyCalls+verify
	}
	for _, item := range Plan(options) {
		if !item.Selected {
			continue
		}
		switch item.ID {
		case "img_generate_basic", "img_background", "img_split_layout", "img_centered_shape", "img_count_shapes", "img_stream_partial", "img_size_invalid", "img_edit_basic", "img_edit_mask":
			spend(1, 1, 0)
		case "img_error_shape":
			spend(1, 0, 0)
		case "img_n":
			spend(1, 2, 0)
		case "img_output_format":
			n := len(extraFormats(p))
			spend(n, n, 0)
		case "img_size_matrix":
			n := len(matrixSizes(p, options.full()))
			spend(n, n, 0)
		case "img_quality_levels":
			n := len(p.Qualities)
			spend(n, n, 0)
		case "img_prov_control", "img_prov_c2pa":
			spend(0, 0, 1)
		case "img_prov_format_matrix":
			spend(0, 0, len(extraFormats(p)))
		case "img_prov_baseline":
			spend(1, 1, 1)
		}
	}
	return b
}
