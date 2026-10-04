package claudecheck

import (
	"model-check/common"
	"strings"
)

// Only timings and counts leave the transport. Thinking/signature contents
// remain ephemeral. TTFT here is the first nonempty visible text fragment.
type StreamMetrics struct {
	FirstTextMS *int64 `json:"first_text_ms"`
	LastTextMS  *int64 `json:"last_text_ms"`
	MaxGapMS    *int64 `json:"max_text_gap_ms"`
	TextEvents  int    `json:"text_events"`
	Events      int    `json:"events"`
}

func (m *StreamMetrics) Observe(data []byte, elapsed int64) {
	m.Events++
	var event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content_block"`
	}
	if common.Unmarshal(data, &event) != nil {
		return
	}
	text := (event.Type == "content_block_delta" && event.Delta.Type == "text_delta" && strings.TrimSpace(event.Delta.Text) != "") ||
		(event.Type == "content_block_start" && event.Block.Type == "text" && strings.TrimSpace(event.Block.Text) != "")
	if !text {
		return
	}
	if m.FirstTextMS == nil {
		m.FirstTextMS = &elapsed
	}
	if m.LastTextMS != nil {
		gap := elapsed - *m.LastTextMS
		if m.MaxGapMS == nil || gap > *m.MaxGapMS {
			m.MaxGapMS = &gap
		}
	}
	m.LastTextMS = &elapsed
	m.TextEvents++
}
