package imagecheck

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"model-check/common"
	"model-check/pkg/media"
)

const signalMarker = "OPENAI-SIGNAL"

// fakeImageAPI is a configurable stand-in for an OpenAI-style image endpoint.
// Its default behavior is a well-behaved model that follows every prompt.
type fakeImageAPI struct {
	mu                 sync.Mutex
	prompts            []string
	requests           int
	downscale          bool // always returns 512 px on the long edge
	ignoreN            bool
	noAlpha            bool
	wrongColor         bool
	pngOnly            bool // ignores output_format
	noStream           bool
	acceptInvalidSize  bool
	ignoreMask         bool
	status             int  // when set, every request fails with it
	signed             bool // appends signalMarker to every image
	stripWebP          bool // WebP responses lose the marker
	missingPromptAs200 bool
}

var rgbRe = regexp.MustCompile(`RGB (\d+), (\d+), (\d+)`)

func rgbs(prompt string) []media.RGB {
	var out []media.RGB
	for _, m := range rgbRe.FindAllStringSubmatch(prompt, -1) {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		out = append(out, media.RGB{R: uint8(r), G: uint8(g), B: uint8(b)})
	}
	return out
}

func fill(img *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

func (f *fakeImageAPI) render(prompt string, w, h int, quality string) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cs := rgbs(prompt)
	if f.wrongColor && len(cs) > 0 {
		cs[0] = media.RGB{R: 255 - cs[0].R, G: 255 - cs[0].G, B: 255 - cs[0].B}
	}
	switch {
	case strings.Contains(prompt, "divided exactly in half"):
		fill(img, image.Rect(0, 0, w/2, h), cs[0].NRGBA())
		fill(img, image.Rect(w/2, 0, w, h), cs[1].NRGBA())
	case strings.Contains(prompt, "centered on a solid"):
		fill(img, img.Bounds(), cs[1].NRGBA())
		fill(img, image.Rect(w/3, h/3, 2*w/3, 2*h/3), cs[0].NRGBA())
	case strings.Contains(prompt, "solid black circles"):
		fill(img, img.Bounds(), color.NRGBA{255, 255, 255, 255})
		var k int
		fmt.Sscanf(prompt[strings.Index(prompt, "exactly "):], "exactly %d", &k)
		rad := min(h/10, w/(k+1)/3)
		for i := 0; i < k; i++ {
			cx, cy := (i+1)*w/(k+1), h/2
			for y := cy - rad; y <= cy+rad; y++ {
				for x := cx - rad; x <= cx+rad; x++ {
					if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= rad*rad {
						img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
					}
				}
			}
		}
	case strings.Contains(prompt, "transparent background"):
		bg := color.NRGBA{0, 0, 0, 0}
		if f.noAlpha {
			bg = color.NRGBA{255, 255, 255, 255}
		}
		fill(img, img.Bounds(), bg)
		fill(img, image.Rect(w/3, h/3, 2*w/3, 2*h/3), color.NRGBA{220, 40, 40, 255})
	case strings.Contains(prompt, "tabby cat"):
		fill(img, img.Bounds(), color.NRGBA{120, 90, 60, 255})
		rows := map[string]int{"low": 20, "medium": 80, "high": 200}[quality]
		rng := rand.New(rand.NewPCG(1, 2))
		for y := 0; y < rows && y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, color.NRGBA{uint8(rng.IntN(256)), uint8(rng.IntN(256)), uint8(rng.IntN(256)), 255})
			}
		}
	default:
		if len(cs) > 0 {
			fill(img, img.Bounds(), cs[0].NRGBA())
		} else {
			fill(img, img.Bounds(), color.NRGBA{128, 128, 128, 255})
		}
	}
	return img
}

func webpHeader(w, h int) []byte {
	b := []byte("RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00")
	dim := make([]byte, 6)
	copy(dim[0:3], []byte{byte(w - 1), byte((w - 1) >> 8), byte((w - 1) >> 16)})
	copy(dim[3:6], []byte{byte(h - 1), byte((h - 1) >> 8), byte((h - 1) >> 16)})
	return append(b, dim...)
}

func (f *fakeImageAPI) encode(img *image.NRGBA, format string) []byte {
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		_ = jpeg.Encode(&buf, img, nil)
	case "webp":
		buf.Write(webpHeader(img.Bounds().Dx(), img.Bounds().Dy()))
		if f.signed && !f.stripWebP {
			buf.WriteString(signalMarker)
		}
		return buf.Bytes()
	default:
		_ = png.Encode(&buf, img)
	}
	if f.signed {
		buf.WriteString(signalMarker)
	}
	return buf.Bytes()
}

func errorBody(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"message":%q,"type":"invalid_request_error","param":null,"code":"x"}}`, msg)
}

func (f *fakeImageAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	status := f.status
	f.mu.Unlock()
	if status != 0 {
		errorBody(w, status, "nope")
		return
	}
	if r.Header.Get("Authorization") == "" {
		errorBody(w, 401, "no key")
		return
	}
	var prompt, size, format, quality, background string
	n := 1
	stream := false
	var base, mask []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(32 << 20)
		prompt, size, format, quality = r.FormValue("prompt"), r.FormValue("size"), r.FormValue("output_format"), r.FormValue("quality")
		if fh, _, err := r.FormFile("image[]"); err == nil {
			base, _ = io.ReadAll(fh)
		}
		if fh, _, err := r.FormFile("mask"); err == nil {
			mask, _ = io.ReadAll(fh)
		}
	} else {
		var body map[string]any
		_ = common.DecodeJson(r.Body, &body)
		prompt, _ = body["prompt"].(string)
		size, _ = body["size"].(string)
		format, _ = body["output_format"].(string)
		quality, _ = body["quality"].(string)
		background, _ = body["background"].(string)
		if v, ok := body["n"].(float64); ok {
			n = int(v)
		}
		stream, _ = body["stream"].(bool)
		if _, has := body["prompt"]; !has && !f.missingPromptAs200 {
			errorBody(w, 400, "Missing required parameter: 'prompt'.")
			return
		}
	}
	_ = background
	f.mu.Lock()
	f.prompts = append(f.prompts, prompt)
	f.mu.Unlock()
	if size == "999x999" && !f.acceptInvalidSize {
		errorBody(w, 400, "Invalid size")
		return
	}
	wd, ht := 1024, 1024
	if a, b, ok := strings.Cut(size, "x"); ok {
		wd, _ = strconv.Atoi(a)
		ht, _ = strconv.Atoi(b)
	}
	if f.downscale {
		scale := 512.0 / float64(max(wd, ht))
		wd, ht = int(float64(wd)*scale), int(float64(ht)*scale)
	}
	if f.pngOnly || format == "" {
		format = "png"
	}
	if f.ignoreN {
		n = 1
	}
	var imgs [][]byte
	if base != nil {
		out, _ := png.Decode(bytes.NewReader(base))
		dst := image.NewNRGBA(out.Bounds())
		draw.Draw(dst, dst.Bounds(), out, image.Point{}, draw.Src)
		if mask != nil && !f.ignoreMask {
			b := dst.Bounds()
			fill(dst, image.Rect(3*b.Dx()/8, 3*b.Dy()/8, 5*b.Dx()/8, 5*b.Dy()/8), color.NRGBA{220, 40, 40, 255})
		} else if mask != nil {
			fill(dst, dst.Bounds(), color.NRGBA{220, 40, 40, 255})
		}
		imgs = append(imgs, f.encode(dst, "png"))
	} else {
		for i := 0; i < n; i++ {
			img := f.render(prompt, wd, ht, quality)
			if n > 1 {
				img.SetNRGBA(i, 0, color.NRGBA{uint8(i + 1), 0, 0, 255}) // make the images differ
			}
			imgs = append(imgs, f.encode(img, format))
		}
	}
	usage := `"usage":{"input_tokens":12,"output_tokens":240,"total_tokens":252}`
	if stream && !f.noStream {
		w.Header().Set("Content-Type", "text/event-stream")
		b64 := base64.StdEncoding.EncodeToString(imgs[0])
		_, _ = fmt.Fprintf(w, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"partial_image_index\":0,\"b64_json\":%q}\n\n", b64)
		_, _ = fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":%q,%s}\n\n", b64, usage)
		return
	}
	var data []string
	for _, im := range imgs {
		data = append(data, fmt.Sprintf(`{"b64_json":%q}`, base64.StdEncoding.EncodeToString(im)))
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"created":1790000000,"data":[%s],%s}`, strings.Join(data, ","), usage)
}

func newFake(t *testing.T, f *fakeImageAPI) (Transport, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return NewHTTPTransport(srv.Client(), srv.URL, "sk-test-image-key", nil), srv
}
