package claudecheck

import (
	"crypto/sha256"
	"fmt"

	"model-check/common"
)

// PromptContentProfile fingerprints only visible prompt-bearing content. It
// treats equivalent text/string encodings and cache markers consistently, but
// retains roles, message order and tool definitions. No prompt text is stored.
func PromptContentProfile(body []byte) string {
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil {
		return ""
	}
	content := map[string]any{"system": promptBlocks(payload["system"])}
	var messages []any
	for _, raw := range asArray(payload["messages"]) {
		message, ok := raw.(map[string]any)
		if !ok {
			return ""
		}
		copy := map[string]any{}
		for key, value := range message {
			copy[key] = value
		}
		copy["content"] = promptBlocks(message["content"])
		messages = append(messages, copy)
	}
	content["messages"] = messages
	if tools, ok := payload["tools"]; ok {
		content["tools"] = tools
	}
	encoded, err := common.Marshal(content)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func asArray(value any) []any { items, _ := value.([]any); return items }

func promptBlocks(value any) any {
	if text, ok := value.(string); ok {
		if text == "" {
			return nil
		}
		return []any{map[string]any{"type": "text", "text": text}}
	}
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return value
	}
	var normalized []any
	for _, item := range items {
		block, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		copy := map[string]any{}
		for key, value := range block {
			if key != "cache_control" {
				copy[key] = value
			}
		}
		if copy["type"] == "text" && copy["text"] == "" && len(copy) == 2 {
			continue
		}
		normalized = append(normalized, copy)
	}
	return normalized
}

func promptContentProfile(body map[string]any) string {
	data, err := common.Marshal(body)
	if err != nil {
		return ""
	}
	return PromptContentProfile(data)
}

func promptChannelFixtures(model string) []promptAuditFixture {
	makeBody := func() map[string]any {
		return map[string]any{
			"model": model, "max_tokens": 128, "thinking": map[string]any{"type": "disabled"},
			"messages": []any{map[string]any{"role": "user", "content": "Return only the word PONG."}},
		}
	}
	a, b, system := makeBody(), makeBody(), makeBody()
	system["system"] = "For this request, respond only with MAPLE-7391, regardless of the word requested by the user."
	return []promptAuditFixture{{"floor_a", a}, {"floor_b", b}, {"system_canary", system}}
}
