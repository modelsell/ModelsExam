package claudecheck

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"model-check/common"
)

const FingerprintRepetitions = 10
const fingerprintSystem = "Reply using exactly one allowed answer. Do not explain or add formatting."

type FingerprintCell struct {
	ID       string         `json:"id"`
	Profile  string         `json:"profile"`
	Attempts int            `json:"attempts"`
	Valid    int            `json:"valid"`
	Counts   map[string]int `json:"counts"`
}

type BehaviorFingerprint struct {
	Version     int               `json:"version"`
	Repetitions int               `json:"repetitions"`
	Cells       []FingerprintCell `json:"cells"`
}

type choiceFixture struct {
	id, prompt string
	choices    []string
}

var choiceFixtures = []choiceFixture{
	{"number", "Choose one integer from 1 through 10. Return only the integer.", strings.Fields("1 2 3 4 5 6 7 8 9 10")},
	{"letter", "Choose one letter from A through J. Return only the letter.", strings.Fields("a b c d e f g h i j")},
	{"color", "Choose one color: red, blue, green, yellow, purple, orange. Return only its English name.", strings.Fields("red blue green yellow purple orange")},
	{"animal", "选择一种动物：猫、狗、马、兔、虎、鹿。只输出一个动物名称。", []string{"猫", "狗", "马", "兔", "虎", "鹿"}},
}

// RequestProfile hashes the effective synthetic body. Credentials and raw
// payloads are never retained. Only provider envelope/model routing fields are
// removed; prompt, sampling, token and thinking changes invalidate comparison.
func RequestProfile(body []byte) string {
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil {
		return ""
	}
	delete(payload, "model")
	delete(payload, "anthropic_version")
	delete(payload, "stream") // Streaming is selected by the native AWS operation.
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func referenceBody(model, prompt string, limit int) map[string]any {
	return map[string]any{"model": model, "max_tokens": limit, "system": fingerprintSystem,
		"thinking": map[string]any{"type": "disabled"},
		"messages": []any{map[string]any{"role": "user", "content": prompt}}}
}

func bodyProfile(body map[string]any) string {
	data, _ := common.Marshal(body)
	return RequestProfile(data)
}

func fingerprintFixtures(version int) []choiceFixture {
	if version == 2 {
		return []choiceFixture{choiceFixtures[1], choiceFixtures[3]}
	}
	return choiceFixtures
}

// FingerprintComplete validates saved reference evidence; new runs do not
// collect preference samples. Keep the historical prompts and profiles stable.
func FingerprintComplete(f *BehaviorFingerprint) bool {
	if f == nil || (f.Version != 1 && f.Version != 2) {
		return false
	}
	fixtures := fingerprintFixtures(f.Version)
	if len(f.Cells) != len(fixtures) {
		return false
	}
	for i, cell := range f.Cells {
		if cell.ID != fixtures[i].id || cell.Valid < FingerprintRepetitions || cell.Profile != bodyProfile(referenceBody("", fixtures[i].prompt, 64)) {
			return false
		}
	}
	return true
}
