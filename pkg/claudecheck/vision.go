package claudecheck

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/google/uuid"
)

func (r *runner) vision() {
	if r.ctx.Err() != nil {
		return
	}
	colors := []color.RGBA{{255, 0, 0, 255}, {0, 0, 255, 255}, {0, 180, 0, 255}}
	names := []string{"RED", "BLUE", "GREEN"}
	choice := int(uuid.New()[0]) % len(colors)
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, colors[choice])
		}
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, img) != nil {
		r.check("vision", "skipped", "dependency_unavailable", nil)
		return
	}
	body := r.body("")
	body["messages"] = []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": base64.StdEncoding.EncodeToString(encoded.Bytes())}},
		map[string]any{"type": "text", "text": "What is the solid color in this image? Reply with exactly RED, BLUE, or GREEN."},
	}}}
	m, response, ok := r.probe("vision", Request{Body: body})
	state := "inconclusive"
	if ok {
		state = status(strings.ToUpper(m.text()) == names[choice])
	}
	r.check("vision", state, "vision_observation", map[string]any{"expected_color": names[choice], "matched": ok && strings.ToUpper(m.text()) == names[choice], "http_status": response.Status})
}
