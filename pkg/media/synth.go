package media

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// SolidPNG is a flat, fully opaque PNG. It is the negative control for the
// provenance check: no generator ever touched it.
func SolidPNG(w, h int, c RGB) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c.NRGBA()), image.Point{}, draw.Src)
	return encodePNG(img)
}

// EditFixture is a base image plus a mask for the edit probes. The editable
// window is the central square; the mask is fully transparent there (the
// convention image-edit APIs use to mark the area to repaint) and opaque
// everywhere else.
type EditFixture struct {
	Base, Mask []byte
	Window     FracRect // editable region
	Protected  FracRect // region that must stay untouched (far left strip)
	Left       RGB
	Right      RGB
}

// NewEditFixture builds a size x size base image split into two flat colors.
func NewEditFixture(size int, left, right RGB) EditFixture {
	base := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(base, image.Rect(0, 0, size/2, size), image.NewUniform(left.NRGBA()), image.Point{}, draw.Src)
	draw.Draw(base, image.Rect(size/2, 0, size, size), image.NewUniform(right.NRGBA()), image.Point{}, draw.Src)
	mask := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(mask, mask.Bounds(), image.NewUniform(color.NRGBA{0, 0, 0, 255}), image.Point{}, draw.Src)
	q := size / 4
	window := image.Rect(q+q/2, q+q/2, size-q-q/2, size-q-q/2)
	draw.Draw(mask, window, image.NewUniform(color.NRGBA{0, 0, 0, 0}), image.Point{}, draw.Src)
	return EditFixture{
		Base: encodePNG(base), Mask: encodePNG(mask),
		Window:    FracRect{0.375, 0.375, 0.625, 0.625},
		Protected: FracRect{0, 0, 0.2, 1},
		Left:      left, Right: right,
	}
}
