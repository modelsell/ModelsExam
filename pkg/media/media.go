// Package media holds deterministic, standard-library-only image helpers for
// the image checks: format sniffing, header parsing, pixel statistics and
// synthetic fixtures. Nothing here calls the network and nothing judges an
// image semantically; every verdict is arithmetic over pixels.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"sort"
)

// MaxPixels bounds decoding so a hostile upstream cannot exhaust memory.
const MaxPixels = 36 << 20

// Info is what the header of an encoded image says about it.
type Info struct {
	Format string `json:"format"` // png, jpeg, webp or ""
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
}

// Sniff identifies an image container from its magic bytes only.
func Sniff(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "jpeg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp"
	}
	return ""
}

// Inspect reads format and dimensions from the header. PNG and JPEG use the
// standard decoder configuration; WebP is parsed by hand because the standard
// library has no WebP support.
func Inspect(data []byte) (Info, error) {
	info := Info{Format: Sniff(data), Bytes: len(data)}
	switch info.Format {
	case "png", "jpeg":
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return info, err
		}
		info.Width, info.Height = cfg.Width, cfg.Height
	case "webp":
		w, h, err := webpSize(data)
		if err != nil {
			return info, err
		}
		info.Width, info.Height = w, h
	default:
		return info, errors.New("unrecognized image format")
	}
	if info.Width <= 0 || info.Height <= 0 || info.Width*info.Height > MaxPixels {
		return info, errors.New("image dimensions out of range")
	}
	return info, nil
}

func webpSize(d []byte) (int, int, error) {
	if len(d) < 30 {
		return 0, 0, errors.New("short webp")
	}
	switch string(d[12:16]) {
	case "VP8 ": // lossy: frame header at byte 20, dimensions at 26
		if len(d) < 30 || d[23] != 0x9d || d[24] != 0x01 || d[25] != 0x2a {
			return 0, 0, errors.New("bad VP8 frame")
		}
		return int(binary.LittleEndian.Uint16(d[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(d[28:30]) & 0x3fff), nil
	case "VP8L": // lossless: signature 0x2f then 14-bit width-1, 14-bit height-1
		if d[20] != 0x2f {
			return 0, 0, errors.New("bad VP8L signature")
		}
		bits := binary.LittleEndian.Uint32(d[21:25])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	case "VP8X": // extended: 24-bit canvas width-1 and height-1
		w := int(d[24]) | int(d[25])<<8 | int(d[26])<<16
		h := int(d[27]) | int(d[28])<<8 | int(d[29])<<16
		return w + 1, h + 1, nil
	}
	return 0, 0, errors.New("unknown webp chunk")
}

// Decode returns pixels for PNG and JPEG. WebP has no decoder here, so pixel
// checks on WebP are reported as unavailable rather than guessed.
func Decode(data []byte) (image.Image, error) {
	info, err := Inspect(data)
	if err != nil {
		return nil, err
	}
	switch info.Format {
	case "png":
		return png.Decode(bytes.NewReader(data))
	case "jpeg":
		return jpeg.Decode(bytes.NewReader(data))
	}
	return nil, errors.New("pixel analysis is not available for " + info.Format)
}

// RGB is an 8-bit color.
type RGB struct{ R, G, B uint8 }

// Dist is the Euclidean distance in RGB space (0 to about 441).
func (a RGB) Dist(b RGB) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func (a RGB) NRGBA() color.NRGBA { return color.NRGBA{a.R, a.G, a.B, 255} }

// FracRect describes a region as fractions of the image size.
type FracRect struct{ X0, Y0, X1, Y1 float64 }

func (f FracRect) rect(b image.Rectangle) image.Rectangle {
	w, h := float64(b.Dx()), float64(b.Dy())
	r := image.Rect(b.Min.X+int(f.X0*w), b.Min.Y+int(f.Y0*h), b.Min.X+int(math.Ceil(f.X1*w)), b.Min.Y+int(math.Ceil(f.Y1*h)))
	return r.Intersect(b)
}

// AvgColor is the mean color over a region, compositing alpha onto white so a
// transparent pixel never reads as black.
func AvgColor(img image.Image, region FracRect) RGB {
	r := region.rect(img.Bounds())
	if r.Empty() {
		return RGB{}
	}
	var sr, sg, sb float64
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb := flatten(img.At(x, y))
			sr, sg, sb = sr+cr, sg+cg, sb+cb
			n++
		}
	}
	f := float64(n)
	return RGB{uint8(math.Round(sr / f)), uint8(math.Round(sg / f)), uint8(math.Round(sb / f))}
}

func flatten(c color.Color) (float64, float64, float64) {
	nr := color.NRGBAModel.Convert(c).(color.NRGBA)
	a := float64(nr.A) / 255
	return float64(nr.R)*a + 255*(1-a), float64(nr.G)*a + 255*(1-a), float64(nr.B)*a + 255*(1-a)
}

// Alpha reports whether the image carries an alpha channel with real
// transparency and what share of pixels is not fully opaque.
type Alpha struct {
	Channel          bool    `json:"channel"`
	TransparentRatio float64 `json:"transparent_ratio"`
}

func AlphaStats(img image.Image) Alpha {
	b := img.Bounds()
	switch img.(type) {
	case *image.NRGBA, *image.RGBA, *image.NRGBA64, *image.RGBA64:
	default:
		return Alpha{}
	}
	clear, total := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				clear++
			}
			total++
		}
	}
	if total == 0 {
		return Alpha{Channel: true}
	}
	return Alpha{Channel: true, TransparentRatio: float64(clear) / float64(total)}
}

func luma(c color.Color) float64 {
	r, g, b := flatten(c)
	return 0.299*r + 0.587*g + 0.114*b
}

// NotBlank is false for an image that is a single flat color or pure noise
// that carries no structure at all. flat is true when luminance barely varies.
func NotBlank(img image.Image) (ok bool, stddev float64) {
	b := img.Bounds()
	stepX, stepY := max(1, b.Dx()/128), max(1, b.Dy()/128)
	var sum, sumSq float64
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			l := luma(img.At(x, y))
			sum, sumSq = sum+l, sumSq+l*l
			n++
		}
	}
	if n == 0 {
		return false, 0
	}
	mean := sum / float64(n)
	stddev = math.Sqrt(math.Max(0, sumSq/float64(n)-mean*mean))
	return stddev > 1.0, stddev
}

// CountDark counts 4-connected dark blobs. A pixel is dark when its luminance
// is below threshold; blobs smaller than minFrac of the image are noise.
func CountDark(img image.Image, threshold, minFrac float64) int {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	// Work on a bounded grid so a 4K image costs the same as a 1K one.
	step := max(1, max(w, h)/512)
	gw, gh := (w+step-1)/step, (h+step-1)/step
	dark := make([]bool, gw*gh)
	for gy := 0; gy < gh; gy++ {
		for gx := 0; gx < gw; gx++ {
			dark[gy*gw+gx] = luma(img.At(b.Min.X+gx*step, b.Min.Y+gy*step)) < threshold
		}
	}
	minArea := max(3, int(minFrac*float64(gw*gh)))
	seen := make([]bool, len(dark))
	blobs := 0
	stack := make([]int, 0, 1024)
	for i := range dark {
		if !dark[i] || seen[i] {
			continue
		}
		area := 0
		stack = append(stack[:0], i)
		seen[i] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			area++
			x, y := p%gw, p/gw
			for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= gw || ny >= gh {
					continue
				}
				if q := ny*gw + nx; dark[q] && !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		if area >= minArea {
			blobs++
		}
	}
	return blobs
}

// RegionDiff is the mean absolute per-channel difference (0 to 255) between
// two images over a region. Images of different size are compared on the
// smaller common grid.
func RegionDiff(a, b image.Image, region FracRect) float64 {
	ra, rb := region.rect(a.Bounds()), region.rect(b.Bounds())
	w, h := min(ra.Dx(), rb.Dx()), min(ra.Dy(), rb.Dy())
	if w <= 0 || h <= 0 {
		return 255
	}
	var sum float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ar, ag, ab := flatten(a.At(ra.Min.X+x, ra.Min.Y+y))
			br, bg, bb := flatten(b.At(rb.Min.X+x, rb.Min.Y+y))
			sum += math.Abs(ar-br) + math.Abs(ag-bg) + math.Abs(ab-bb)
		}
	}
	return sum / float64(w*h*3)
}

// DHash is a 64-bit difference hash; Hamming distance between two hashes is a
// cheap perceptual similarity measure.
func DHash(img image.Image) uint64 {
	b := img.Bounds()
	var hash uint64
	bit := uint(0)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			l := luma(img.At(b.Min.X+(x*b.Dx())/9+b.Dx()/18, b.Min.Y+(y*b.Dy())/8+b.Dy()/16))
			r := luma(img.At(b.Min.X+((x+1)*b.Dx())/9+b.Dx()/18, b.Min.Y+(y*b.Dy())/8+b.Dy()/16))
			if l > r {
				hash |= 1 << bit
			}
			bit++
		}
	}
	return hash
}

func Hamming(a, b uint64) int {
	n, x := 0, a^b
	for ; x != 0; x &= x - 1 {
		n++
	}
	return n
}

// Thumbnail returns a JPEG no larger than maxEdge on its longest side.
func Thumbnail(img image.Image, maxEdge, quality int) (data []byte, w, h int, err error) {
	b := img.Bounds()
	scale := math.Min(1, float64(maxEdge)/float64(max(b.Dx(), b.Dy())))
	w, h = max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx0, sx1 := b.Min.X+x*b.Dx()/w, b.Min.X+max(x*b.Dx()/w+1, (x+1)*b.Dx()/w)
			sy0, sy1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+max(y*b.Dy()/h+1, (y+1)*b.Dy()/h)
			var sr, sg, sb float64
			n := 0
			for sy := sy0; sy < sy1 && sy < b.Max.Y; sy++ {
				for sx := sx0; sx < sx1 && sx < b.Max.X; sx++ {
					r, g, bl := flatten(img.At(sx, sy))
					sr, sg, sb = sr+r, sg+g, sb+bl
					n++
				}
			}
			if n > 0 {
				f := float64(n)
				dst.SetRGBA(x, y, color.RGBA{uint8(sr / f), uint8(sg / f), uint8(sb / f), 255})
			}
		}
	}
	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
}

// Median returns the middle value of xs (mean of the two middle values for an
// even count). It does not modify xs.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	if len(c)%2 == 1 {
		return c[len(c)/2]
	}
	return (c[len(c)/2-1] + c[len(c)/2]) / 2
}
