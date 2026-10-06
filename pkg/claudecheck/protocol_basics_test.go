package claudecheck

import (
	"testing"

	"github.com/stretchr/testify/require"
	"model-check/common"
)

// Basic Messages API shape checks, per
// https://platform.claude.com/docs/en/api/messages and
// https://platform.claude.com/docs/en/build-with-claude/streaming.

func officialMessage(mutate func(map[string]any)) message {
	doc := map[string]any{
		"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-test",
		"content":     []any{map[string]any{"type": "text", "text": "hi"}},
		"stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": 5, "output_tokens": 1},
	}
	if mutate != nil {
		mutate(doc)
	}
	data, _ := common.Marshal(doc)
	var m message
	_ = common.Unmarshal(data, &m)
	return m
}

func TestMessageEnvelopeRequiresDocumentedFields(t *testing.T) {
	require.Empty(t, officialMessage(nil).validationErrors(false))
	for code, mutate := range map[string]func(map[string]any){
		"type_not_message":       func(m map[string]any) { m["type"] = "chat.completion" },
		"role_not_assistant":     func(m map[string]any) { m["role"] = "user" },
		"missing_message_id":     func(m map[string]any) { delete(m, "id") },
		"missing_model":          func(m map[string]any) { delete(m, "model") },
		"missing_content_array":  func(m map[string]any) { delete(m, "content") },
		"empty_content":          func(m map[string]any) { m["content"] = []any{} },
		"missing_stop_reason":    func(m map[string]any) { m["stop_reason"] = nil },
		"missing_input_tokens":   func(m map[string]any) { m["usage"] = map[string]any{"output_tokens": 1} },
		"missing_output_tokens":  func(m map[string]any) { m["usage"] = map[string]any{"input_tokens": 1} },
		"negative_input_tokens":  func(m map[string]any) { m["usage"] = map[string]any{"input_tokens": -1, "output_tokens": 1} },
		"negative_output_tokens": func(m map[string]any) { m["usage"] = map[string]any{"input_tokens": 1, "output_tokens": -1} },
	} {
		require.Contains(t, officialMessage(mutate).validationErrors(false), code)
	}
	// A refusal may legitimately carry no content blocks.
	refusal := officialMessage(func(m map[string]any) { m["content"], m["stop_reason"] = []any{}, "refusal" })
	require.Empty(t, refusal.validationErrors(false))
}

func streamEvents(lines ...string) [][]byte {
	events := make([][]byte, len(lines))
	for i, line := range lines {
		events[i] = []byte(line)
	}
	return events
}

const (
	evStart     = `{"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":4}}}`
	evTextStart = `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	evTextA     = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`
	evTextB     = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`
	evStop0     = `{"type":"content_block_stop","index":0}`
	evToolStart = `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{}}}`
	evToolA     = `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\": \"Pa"}}`
	evToolB     = `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"ris\"}"}}`
	evStop1     = `{"type":"content_block_stop","index":1}`
	evDelta     = `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":27}}`
	evMsgStop   = `{"type":"message_stop"}`
	evPing      = `{"type":"ping"}`
)

func TestStreamAssemblesTextToolInputAndUsage(t *testing.T) {
	m, ok := parseStream(streamEvents(evStart, evPing, evTextStart, evTextA, evPing, evTextB, evStop0,
		evToolStart, evToolA, evToolB, evStop1, evDelta, evMsgStop))
	require.True(t, ok)
	require.Equal(t, "Hello", m.text())
	require.Equal(t, map[string]any{"city": "Paris"}, m.Content[1]["input"])
	require.Equal(t, "tool_use", m.StopReason)
	// message_start carries input usage; message_delta carries the cumulative output.
	require.Equal(t, int64(12), *m.Usage.Input)
	require.Equal(t, int64(27), *m.Usage.Output)
	require.Equal(t, int64(4), *m.Usage.CacheRead)
}

func TestStreamRejectsLifecycleViolations(t *testing.T) {
	badJSON := `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`
	for name, events := range map[string][][]byte{
		"duplicate message_start":       streamEvents(evStart, evStart, evTextStart, evTextA, evStop0, evDelta, evMsgStop),
		"block before message_start":    streamEvents(evTextStart, evStart, evTextA, evStop0, evDelta, evMsgStop),
		"duplicate block index":         streamEvents(evStart, evTextStart, evTextA, evStop0, evTextStart, evStop0, evDelta, evMsgStop),
		"delta after block stop":        streamEvents(evStart, evTextStart, evStop0, evTextA, evDelta, evMsgStop),
		"block stop without start":      streamEvents(evStart, evStop1, evTextStart, evTextA, evStop0, evDelta, evMsgStop),
		"message_delta with open block": streamEvents(evStart, evTextStart, evTextA, evDelta, evStop0, evMsgStop),
		"block after message_delta":     streamEvents(evStart, evTextStart, evTextA, evStop0, evDelta, evToolStart, evStop1, evMsgStop),
		"missing message_delta":         streamEvents(evStart, evTextStart, evTextA, evStop0, evMsgStop),
		"event after message_stop":      streamEvents(evStart, evTextStart, evTextA, evStop0, evDelta, evMsgStop, evTextA),
		"unparseable tool input":        streamEvents(evStart, evToolStart, badJSON, evStop1, evDelta, evMsgStop),
		"invalid event json":            streamEvents(evStart, `{"type":`, evTextStart, evTextA, evStop0, evDelta, evMsgStop),
		"empty stream":                  nil,
	} {
		_, ok := parseStream(events)
		require.False(t, ok, name)
	}
}
