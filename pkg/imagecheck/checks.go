package imagecheck

import (
	"fmt"
	"image"

	"model-check/pkg/media"
	"model-check/pkg/openaicheck"
)

type namedColor struct {
	Name string
	C    media.RGB
}

var palette = []namedColor{
	{"red", media.RGB{R: 220, G: 40, B: 40}},
	{"green", media.RGB{R: 40, G: 170, B: 60}},
	{"blue", media.RGB{R: 40, G: 80, B: 220}},
	{"yellow", media.RGB{R: 240, G: 210, B: 40}},
	{"purple", media.RGB{R: 140, G: 60, B: 190}},
	{"orange", media.RGB{R: 240, G: 140, B: 30}},
}

// target is a palette color with this run's random jitter applied.
type target struct {
	name string
	c    media.RGB
}

func (t target) phrase() string {
	return fmt.Sprintf("%s (RGB %d, %d, %d)", t.name, t.c.R, t.c.G, t.c.B)
}

func jitter(v uint8, d int) uint8 { return uint8(min(255, max(0, int(v)+d))) }

// pickColors returns n distinct palette colors with random jitter, so the
// prompt text differs on every run.
func (r *runner) pickColors(n int) []target {
	idx := r.rng.Perm(len(palette))[:n]
	out := make([]target, n)
	for i, k := range idx {
		c := palette[k].C
		out[i] = target{palette[k].Name, media.RGB{R: jitter(c.R, r.rng.IntN(25)-12), G: jitter(c.G, r.rng.IntN(25)-12), B: jitter(c.B, r.rng.IntN(25)-12)}}
	}
	return out
}

func nearest(c media.RGB) string {
	best, name := 1e9, ""
	for _, p := range palette {
		if d := c.Dist(p.C); d < best {
			best, name = d, p.Name
		}
	}
	return name
}

// colorOK requires the color to be close to the target and, among the six
// palette hues, closest to the requested one.
func colorOK(got media.RGB, want target) bool {
	return got.Dist(want.c) <= 80 && nearest(got) == want.name
}

func rgbText(c media.RGB) string { return fmt.Sprintf("%d,%d,%d", c.R, c.G, c.B) }

func (r *runner) solidPrompt(t target) string {
	return fmt.Sprintf("A flat solid color image filled entirely with %s. No text, no gradient, no pattern, no objects, no border.", t.phrase())
}

// generateBasic is the baseline: one cheap image whose color is checked.
// Credential, quota, network and upstream failures stop the run, because every
// later probe would only repeat them.
func (r *runner) generateBasic() bool {
	const id = "img_generate_basic"
	t := r.pickColors(1)[0]
	g := r.probe(id, r.call, genRequest(r.body(r.solidPrompt(t), nil), false), 1)
	r.basic = g
	if !g.ok {
		code := g.sample.ErrorCode
		if code == "" {
			code = "invalid_response"
		}
		r.judge(id, g, false, "", code, nil)
		if openaicheck.IsAvailabilityCode(code) {
			r.report.StopReason, r.report.StopProbe = code, id
		}
		return false
	}
	r.keep(id, g)
	d := g.images[0]
	if d.info.Format == "png" {
		r.byFormat["png"] = d
	}
	r.check(id, "pass", "ok", map[string]any{"images": len(g.images), "format": d.info.Format, "width": d.info.Width, "height": d.info.Height})

	if len(g.sample.UsageIssues) > 0 {
		r.check("img_response_shape", "fail", "usage_inconsistent", map[string]any{"issues": g.sample.UsageIssues})
	} else {
		r.check("img_response_shape", "pass", "ok", map[string]any{"usage_present": g.sample.Usage.Total != nil})
	}
	r.checkSize("img_size_exact", probeSize, d)
	r.checkSolid(d, t)
	return true
}

func (r *runner) checkSize(id, want string, d *decoded) {
	w, h, _ := parseSize(want)
	ev := map[string]any{"requested": want, "actual": fmt.Sprintf("%dx%d", d.info.Width, d.info.Height)}
	switch {
	case d.info.Width == w && d.info.Height == h:
		r.check(id, "pass", "ok", ev)
	case max(d.info.Width, d.info.Height) < max(w, h):
		r.check(id, "fail", "silently_downscaled", ev)
	default:
		r.check(id, "fail", "size_mismatch", ev)
	}
}

func (r *runner) checkSolid(d *decoded, t target) {
	const id = "img_solid_color"
	px, err := d.pixels()
	if err != nil {
		r.check(id, "inconclusive", "pixels_unavailable", map[string]any{"format": d.info.Format})
		return
	}
	got := media.AvgColor(px, media.FracRect{X0: 0, Y0: 0, X1: 1, Y1: 1})
	ev := map[string]any{"requested": rgbText(t.c), "requested_name": t.name, "got": rgbText(got), "got_name": nearest(got), "distance": round1(got.Dist(t.c))}
	if colorOK(got, t) {
		r.check(id, "pass", "ok", ev)
	} else {
		r.check(id, "fail", "color_mismatch", ev)
	}
}

// contentSuite runs the layout probes. Every verdict is pixel arithmetic.
func (r *runner) contentSuite() {
	if r.live("img_split_layout") {
		cs := r.pickColors(2)
		prompt := fmt.Sprintf("An image divided exactly in half by a sharp vertical line: the left half is solid %s and the right half is solid %s. No text, no gradient, no other elements.", cs[0].phrase(), cs[1].phrase())
		r.pixelProbe("img_split_layout", prompt, func(g *gen, px image.Image) (bool, string, map[string]any) {
			left := media.AvgColor(px, media.FracRect{X0: 0.05, Y0: 0.1, X1: 0.4, Y1: 0.9})
			right := media.AvgColor(px, media.FracRect{X0: 0.6, Y0: 0.1, X1: 0.95, Y1: 0.9})
			ev := map[string]any{"left_requested": rgbText(cs[0].c), "left_got": rgbText(left), "right_requested": rgbText(cs[1].c), "right_got": rgbText(right)}
			return colorOK(left, cs[0]) && colorOK(right, cs[1]), "layout_mismatch", ev
		})
	}
	if r.live("img_centered_shape") {
		cs := r.pickColors(2)
		prompt := fmt.Sprintf("A solid %s square centered on a solid %s background. The square covers about one third of the image width. No text, no shadow, no other elements.", cs[0].phrase(), cs[1].phrase())
		r.pixelProbe("img_centered_shape", prompt, func(g *gen, px image.Image) (bool, string, map[string]any) {
			center := media.AvgColor(px, media.FracRect{X0: 0.44, Y0: 0.44, X1: 0.56, Y1: 0.56})
			corners := []media.FracRect{{X0: 0, Y0: 0, X1: 0.1, Y1: 0.1}, {X0: 0.9, Y0: 0, X1: 1, Y1: 0.1}, {X0: 0, Y0: 0.9, X1: 0.1, Y1: 1}, {X0: 0.9, Y0: 0.9, X1: 1, Y1: 1}}
			ok := colorOK(center, cs[0])
			var got []string
			for _, c := range corners {
				col := media.AvgColor(px, c)
				got = append(got, rgbText(col))
				ok = ok && colorOK(col, cs[1])
			}
			return ok, "layout_mismatch", map[string]any{"center_requested": rgbText(cs[0].c), "center_got": rgbText(center), "background_requested": rgbText(cs[1].c), "corners_got": got}
		})
	}
	if r.live("img_count_shapes") {
		k := 2 + r.rng.IntN(4)
		prompt := fmt.Sprintf("On a plain white background, exactly %d solid black circles of equal size, evenly spaced in a single horizontal row, none touching each other or the image edges. No text, no other elements.", k)
		r.pixelProbe("img_count_shapes", prompt, func(g *gen, px image.Image) (bool, string, map[string]any) {
			got := media.CountDark(px, 110, 0.002)
			return got == k, "count_mismatch", map[string]any{"requested": k, "counted": got}
		})
	}
}
