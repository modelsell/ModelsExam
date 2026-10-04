package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"testing"
)

func circles(w, h, k int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	r := min(h/10, w/(k+1)/3)
	for i := 0; i < k; i++ {
		cx, cy := (i+1)*w/(k+1), h/2
		for y := cy - r; y <= cy+r; y++ {
			for x := cx - r; x <= cx+r; x++ {
				if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
					img.Set(x, y, color.Black)
				}
			}
		}
	}
	return img
}

func TestSniffAndInspect(t *testing.T) {
	data := SolidPNG(64, 32, RGB{200, 30, 40})
	if Sniff(data) != "png" {
		t.Fatal("png not sniffed")
	}
	info, err := Inspect(data)
	if err != nil || info.Width != 64 || info.Height != 32 || info.Format != "png" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	img, _ := Decode(data)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if Sniff(buf.Bytes()) != "jpeg" {
		t.Fatal("jpeg not sniffed")
	}
	if _, err := Inspect([]byte("not an image")); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestWebPHeaders(t *testing.T) {
	riff := func(chunk string, payload []byte) []byte {
		b := append([]byte("RIFF\x00\x00\x00\x00WEBP"), chunk...)
		b = append(b, 0, 0, 0, 0)
		return append(b, payload...)
	}
	// VP8X: canvas 1535 x 1023 stored as width-1 / height-1 in 24 bits.
	x := make([]byte, 18)
	x[4], x[5] = 0xFE, 0x05 // 1534
	x[7], x[8] = 0xFE, 0x03 // 1022
	if info, err := Inspect(riff("VP8X", x)); err != nil || info.Width != 1535 || info.Height != 1023 || info.Format != "webp" {
		t.Fatalf("vp8x %+v %v", info, err)
	}
	// VP8L: width 100, height 50.
	l := make([]byte, 10)
	l[0] = 0x2f
	binary.LittleEndian.PutUint32(l[1:], uint32(99)|uint32(49)<<14)
	if info, err := Inspect(riff("VP8L", l)); err != nil || info.Width != 100 || info.Height != 50 {
		t.Fatalf("vp8l %+v %v", info, err)
	}
	// VP8: start code then 16-bit width and height.
	v := make([]byte, 14)
	v[3], v[4], v[5] = 0x9d, 0x01, 0x2a
	binary.LittleEndian.PutUint16(v[6:], 640)
	binary.LittleEndian.PutUint16(v[8:], 480)
	if info, err := Inspect(riff("VP8 ", v)); err != nil || info.Width != 640 || info.Height != 480 {
		t.Fatalf("vp8 %+v %v", info, err)
	}
	if _, err := Decode(riff("VP8X", x)); err == nil {
		t.Fatal("webp pixel decode should be unavailable")
	}
}

func TestAvgColorAndDist(t *testing.T) {
	img, _ := Decode(SolidPNG(40, 40, RGB{10, 200, 90}))
	got := AvgColor(img, FracRect{0, 0, 1, 1})
	if got != (RGB{10, 200, 90}) {
		t.Fatalf("avg %v", got)
	}
	if (RGB{0, 0, 0}).Dist(RGB{3, 4, 0}) != 5 {
		t.Fatal("dist")
	}
}

func TestEditFixtureAndRegionDiff(t *testing.T) {
	f := NewEditFixture(256, RGB{40, 80, 220}, RGB{240, 210, 40})
	base, _ := Decode(f.Base)
	mask, _ := Decode(f.Mask)
	if AvgColor(base, FracRect{0, 0, 0.4, 1}).Dist(f.Left) > 1 {
		t.Fatal("left half color")
	}
	if a := AlphaStats(mask); !a.Channel || a.TransparentRatio < 0.05 || a.TransparentRatio > 0.2 {
		t.Fatalf("mask alpha %+v", a)
	}
	if d := RegionDiff(base, base, f.Protected); d != 0 {
		t.Fatalf("self diff %v", d)
	}
	solid, _ := Decode(SolidPNG(256, 256, RGB{255, 0, 0}))
	if d := RegionDiff(base, solid, f.Protected); d < 50 {
		t.Fatalf("diff against red should be large, got %v", d)
	}
}

func TestAlphaOpaqueVsTransparent(t *testing.T) {
	op, _ := Decode(SolidPNG(16, 16, RGB{1, 2, 3}))
	if a := AlphaStats(op); a.TransparentRatio != 0 {
		t.Fatalf("opaque png reports transparency %+v", a)
	}
	tr := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	if a := AlphaStats(tr); !a.Channel || a.TransparentRatio != 1 {
		t.Fatalf("empty nrgba %+v", a)
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, op, nil)
	j, _ := Decode(buf.Bytes())
	if AlphaStats(j).Channel {
		t.Fatal("jpeg has no alpha channel")
	}
}

func TestCountDark(t *testing.T) {
	for k := 1; k <= 5; k++ {
		if got := CountDark(circles(1024, 1024, k), 100, 0.002); got != k {
			t.Fatalf("k=%d counted %d", k, got)
		}
	}
	// A touching pair merges into one blob; specks below the area floor are ignored.
	img := circles(512, 512, 1).(*image.NRGBA)
	img.Set(5, 5, color.Black)
	img.Set(6, 5, color.Black)
	if got := CountDark(img, 100, 0.002); got != 1 {
		t.Fatalf("speck counted: %d", got)
	}
}

func TestNotBlank(t *testing.T) {
	flat, _ := Decode(SolidPNG(64, 64, RGB{9, 9, 9}))
	if ok, _ := NotBlank(flat); ok {
		t.Fatal("flat image reported as content")
	}
	if ok, sd := NotBlank(circles(256, 256, 3)); !ok || sd < 10 {
		t.Fatalf("circles blank? sd=%v", sd)
	}
}

func TestDHashAndThumbnail(t *testing.T) {
	a, b := circles(256, 256, 2), circles(256, 256, 2)
	if Hamming(DHash(a), DHash(b)) != 0 {
		t.Fatal("identical images differ")
	}
	if Hamming(DHash(a), DHash(circles(256, 256, 5))) == 0 {
		t.Fatal("different images hash equal")
	}
	data, w, h, err := Thumbnail(circles(1024, 512, 3), 128, 70)
	if err != nil || w != 128 || h != 64 || Sniff(data) != "jpeg" {
		t.Fatalf("thumb %d %d %v", w, h, err)
	}
	if len(data) > 8<<10 {
		t.Fatalf("thumbnail too large: %d", len(data))
	}
	if Median([]float64{3, 1, 2}) != 2 || Median([]float64{1, 2, 3, 4}) != 2.5 || Median(nil) != 0 {
		t.Fatal("median")
	}
}
