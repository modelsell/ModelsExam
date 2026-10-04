package claudecheck

import (
	"bufio"
	"errors"
	"io"
	"math"
	"strings"
	"time"

	"model-check/common"
)

type message struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Role       string           `json:"role"`
	Model      string           `json:"model"`
	Content    []map[string]any `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      Usage            `json:"usage"`
}

func (m message) text() string {
	var text strings.Builder
	for _, b := range m.Content {
		if b["type"] == "text" {
			s, _ := b["text"].(string)
			text.WriteString(s)
		}
	}
	return strings.TrimSpace(text.String())
}

func (m message) valid() bool {
	return len(m.validationErrors(false)) == 0
}

// Empty arrays are legitimate for zero output and tiny token limits. Missing
// content/usage remain invalid. Official refusal responses can also be empty.
func (m message) validationErrors(allowEmpty bool) []string {
	var issues []string
	add := func(ok bool, code string) {
		if !ok {
			issues = append(issues, code)
		}
	}
	add(m.Type == "message", "type_not_message")
	add(m.Role == "assistant", "role_not_assistant")
	add(m.ID != "", "missing_message_id")
	add(m.Model != "", "missing_model")
	add(m.Content != nil, "missing_content_array")
	if m.Content != nil && !allowEmpty && m.StopReason != "refusal" {
		add(len(m.Content) > 0, "empty_content")
	}
	add(m.StopReason != "", "missing_stop_reason")
	add(m.Usage.Input != nil, "missing_input_tokens")
	add(m.Usage.Output != nil, "missing_output_tokens")
	if m.Usage.Input != nil {
		add(*m.Usage.Input >= 0, "negative_input_tokens")
	}
	if m.Usage.Output != nil {
		add(*m.Usage.Output >= 0, "negative_output_tokens")
	}
	return issues
}

// ReadSSE accepts multiline data frames and requires the caller to validate the
// event lifecycle. It bounds the entire stream, not just each individual line.
func ReadSSE(reader io.Reader, start time.Time) ([][]byte, *int64, error) {
	return ReadSSEWithMetrics(reader, start, nil)
}

func ReadSSEWithMetrics(reader io.Reader, start time.Time, metrics *StreamMetrics) ([][]byte, *int64, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxResponseBytes+1))
	scanner.Buffer(make([]byte, 4096), MaxResponseBytes)
	var events [][]byte
	var first *int64
	var lines []string
	consumed := 0
	flush := func() {
		if len(lines) == 0 {
			return
		}
		data := strings.Join(lines, "\n")
		lines = nil
		if data == "[DONE]" {
			return
		}
		if first == nil {
			ms := time.Since(start).Milliseconds()
			first = &ms
		}
		events = append(events, []byte(data))
		if metrics != nil {
			metrics.Observe([]byte(data), time.Since(start).Milliseconds())
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(line) + 1
		if consumed > MaxResponseBytes {
			return events, first, errors.New("response exceeds size limit")
		}
		if line == "" {
			flush()
		} else if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return events, first, err
	}
	// A frame without a terminating blank line is incomplete.
	if len(lines) > 0 {
		return events, first, errors.New("incomplete SSE frame")
	}
	return events, first, nil
}

func parseStream(events [][]byte) (message, bool) {
	var m message
	started, delta, stopped, invalid := false, false, false, false
	blocks := map[int]bool{}
	contents := map[int]map[string]any{}
	partialJSON := map[int]string{}
	for _, data := range events {
		var e struct {
			Type         string         `json:"type"`
			Index        int            `json:"index"`
			Message      message        `json:"message"`
			ContentBlock map[string]any `json:"content_block"`
			Delta        struct {
				StopReason  string `json:"stop_reason"`
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			Usage Usage `json:"usage"`
		}
		if common.Unmarshal(data, &e) != nil {
			invalid = true
			continue
		}
		if e.Type == "ping" {
			continue
		}
		if stopped {
			invalid = true
		}
		switch e.Type {
		case "message_start":
			if started {
				invalid = true
			}
			started = true
			m = e.Message
		case "content_block_start":
			if !started || delta || contents[e.Index] != nil || e.Index < 0 {
				invalid = true
			}
			blocks[e.Index] = true
			contents[e.Index] = e.ContentBlock
			m.Content = append(m.Content, e.ContentBlock)
		case "content_block_delta":
			if !blocks[e.Index] || delta {
				invalid = true
				continue
			}
			block := contents[e.Index]
			if block == nil {
				invalid = true
				continue
			}
			appendString := func(key, value string) { previous, _ := block[key].(string); block[key] = previous + value }
			switch e.Delta.Type {
			case "text_delta":
				appendString("text", e.Delta.Text)
			case "thinking_delta":
				appendString("thinking", e.Delta.Thinking)
			case "signature_delta":
				appendString("signature", e.Delta.Signature)
			case "input_json_delta":
				partialJSON[e.Index] += e.Delta.PartialJSON
			}
		case "content_block_stop":
			if !blocks[e.Index] {
				invalid = true
			}
			delete(blocks, e.Index)
			if raw := partialJSON[e.Index]; raw != "" {
				var input map[string]any
				if common.UnmarshalJsonStr(raw, &input) != nil || contents[e.Index] == nil {
					invalid = true
				} else {
					contents[e.Index]["input"] = input
				}
			}
		case "message_delta":
			if !started || len(blocks) != 0 {
				invalid = true
			}
			delta = true
			m.StopReason = e.Delta.StopReason
			mergeUsage(&m.Usage, e.Usage)
		case "message_stop":
			if !started || !delta || len(blocks) != 0 {
				invalid = true
			}
			stopped = true
		case "error":
			invalid = true
		default:
			// Anthropic may add event types. Preserve known lifecycle checks
			// while allowing forward-compatible metadata events.
		}
	}
	return m, !invalid && started && delta && stopped && m.valid()
}

func mergeUsage(dst *Usage, src Usage) {
	if src.Input != nil {
		dst.Input = src.Input
	}
	if src.Output != nil {
		dst.Output = src.Output
	}
	if src.CacheWrite != nil {
		dst.CacheWrite = src.CacheWrite
	}
	if src.CacheRead != nil {
		dst.CacheRead = src.CacheRead
	}
}

func totalInput(u Usage) (int64, bool) {
	if u.Input == nil || *u.Input < 0 {
		return 0, false
	}
	total := *u.Input
	for _, n := range []*int64{u.CacheRead, u.CacheWrite} {
		if n != nil {
			if *n < 0 {
				return 0, false
			}
			if *n > math.MaxInt64-total {
				return 0, false
			}
			total += *n
		}
	}
	return total, true
}
