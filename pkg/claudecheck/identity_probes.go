package claudecheck

import (
	"encoding/base64"
	"regexp"
	"sort"
	"strings"
)

// IdentityRequests is the focused prefill probe; thinking adds ThinkingRequests.
const IdentityRequests = 1
const ThinkingRequests = 2

// Thinking signatures embed the name of the model that produced them as a
// length-delimited protobuf string (field 6). Older models use the public ID;
// newer ones use an internal codename. The format is undocumented, so unknown
// names are reported as unknown, never as a mismatch.
var signatureModelName = regexp.MustCompile(`^claude-[a-z0-9][a-z0-9.-]{1,62}$`)

// Observed on genuine Bedrock responses (2026-10-10 request log). Add entries
// only from responses whose model is otherwise established.
var signatureCodenames = map[string]string{
	"claude-quince":  "opus-4-8",
	"claude-honey":   "opus-5",
	"claude-saffron": "sonnet-5",
}

func decodeSignature(signature string) []byte {
	signature = strings.TrimSpace(signature)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if data, err := encoding.DecodeString(signature); err == nil {
			return data
		}
	}
	return nil
}

// signatureModel extracts the embedded model name without retaining or
// validating the signature. It returns "" when no name is present.
func signatureModel(signature string) string {
	data := decodeSignature(signature)
	for i := 2; i < len(data); i++ {
		if data[i-2] != 0x32 || !strings.HasPrefix(string(data[i:]), "claude-") {
			continue
		}
		n := int(data[i-1])
		if n > 64 || i+n > len(data) {
			continue
		}
		if name := string(data[i : i+n]); signatureModelName.MatchString(name) {
			return name
		}
	}
	return ""
}

func messageSignatureModel(m message) string {
	for _, block := range m.Content {
		if block["type"] != "thinking" {
			continue
		}
		if signature, _ := block["signature"].(string); signature != "" {
			if name := signatureModel(signature); name != "" {
				return name
			}
		}
	}
	return ""
}

// canonicalModel reduces a declared or embedded model name to family-version,
// e.g. "opus-4-8". It returns "" for names that cannot be resolved.
func canonicalModel(raw string) string {
	if family, ok := signatureCodenames[strings.ToLower(raw)]; ok {
		return family
	}
	m := parseDeclaredModel(raw)
	if !m.known {
		return ""
	}
	if m.minor == "" {
		return m.name + "-" + m.major
	}
	return m.name + "-" + m.major + "-" + m.minor
}

// signatureModelCheck compares every embedded model name with the requested
// model. A known name for a different model is direct evidence that some
// requests were answered by another model.
func (r *runner) signatureModelCheck() {
	expected := canonicalModel(r.report.Model)
	observed := map[string]int{}
	for _, sample := range r.report.Samples {
		if sample.SignatureModel != "" {
			observed[sample.SignatureModel]++
		}
	}
	if len(observed) == 0 {
		r.check("signature_model", "inconclusive", "no_signature", nil)
		return
	}
	names := make([]string, 0, len(observed))
	for name := range observed {
		names = append(names, name)
	}
	sort.Strings(names)
	var details []map[string]any
	matched, mismatched, unknown := 0, 0, 0
	for _, name := range names {
		resolved := canonicalModel(name)
		detail := map[string]any{"signature_model": name, "samples": observed[name]}
		if resolved != "" {
			detail["resolved"] = resolved
		}
		details = append(details, detail)
		switch {
		case resolved == "":
			unknown += observed[name]
		case expected != "" && resolved == expected:
			matched += observed[name]
		case expected != "":
			mismatched += observed[name]
		}
	}
	evidence := map[string]any{"expected": expected, "observed": details, "matched_samples": matched, "mismatched_samples": mismatched, "unknown_samples": unknown}
	switch {
	case expected == "":
		r.check("signature_model", "inconclusive", "requested_model_unknown", evidence)
	case mismatched > 0:
		r.check("signature_model", "fail", "signature_model_mismatch", evidence)
	case matched > 0:
		r.check("signature_model", "pass", "signature_model_match", evidence)
	default:
		r.check("signature_model", "inconclusive", "signature_model_unknown", evidence)
	}
}

// Prefill support by generation, from Bedrock responses in the 2026-10-10
// request log (rejected) and the documented behavior of earlier models.
func prefillRule(model string) string {
	switch canonicalModel(model) {
	case "opus-4-6", "opus-4-7", "opus-4-8", "opus-5", "sonnet-5":
		return "rejected"
	case "sonnet-4-5", "haiku-4-5", "opus-4-1", "opus-4", "sonnet-4", "sonnet-3-7", "haiku-3-5":
		return "accepted"
	}
	return ""
}

var prefillRejection = regexp.MustCompile(`(?i)prefill|must end with a user message|final assistant content`)

// prefill sends a conversation ending in an assistant turn. Generations that
// no longer support prefill reject it; acceptance means the request reached a
// different model or was converted on the way.
func (r *runner) prefill() {
	body := r.body("Reply with a JSON object containing a greeting.")
	body["max_tokens"] = 32
	body["messages"] = append(body["messages"].([]any), map[string]any{"role": "assistant", "content": "{"})
	before := len(r.report.Samples)
	_, response, ok := r.probe("prefill", Request{Body: body})
	if len(r.report.Samples) == before {
		return
	}
	sample := r.report.Samples[len(r.report.Samples)-1]
	rejected := response.Status == 400 && prefillRejection.MatchString(sample.Error)
	rule := prefillRule(r.report.Model)
	evidence := map[string]any{"http_status": response.Status, "expected": rule, "rejected": rejected, "accepted": ok}
	switch {
	case rule == "rejected" && rejected, rule == "accepted" && ok:
		r.check("prefill", "pass", "prefill_as_expected", evidence)
	case rule == "rejected" && ok:
		r.check("prefill", "fail", "prefill_accepted", evidence)
	case rule == "accepted" && rejected:
		r.check("prefill", "fail", "prefill_rejected", evidence)
	case rule == "" && (ok || rejected):
		r.check("prefill", "inconclusive", "prefill_rule_unknown", evidence)
	default:
		r.check("prefill", "inconclusive", "prefill_unresolved", evidence)
	}
}

// focusedThinking requests a streamed, signed thinking block and then checks
// that the upstream rejects a forged signature in the current tool turn.
func (r *runner) focusedThinking() {
	body := r.body("What is the greatest common divisor of 1071 and 462? Work it out step by step, then give only the number on the last line.")
	body["max_tokens"] = 2048
	body["stream"] = true
	mode := r.thinkingMode()
	body["thinking"] = mode
	if mode["type"] == "adaptive" {
		body["output_config"] = map[string]any{"effort": "high"}
	}
	m, response, ok := r.probe("thinking", Request{Body: body})
	signed, thinkingBlocks := false, 0
	for _, block := range m.Content {
		if block["type"] == "thinking" || block["type"] == "redacted_thinking" {
			thinkingBlocks++
			signature, _ := block["signature"].(string)
			data, _ := block["data"].(string)
			signed = signed || signature != "" || data != ""
		}
	}
	evidence := map[string]any{"http_status": response.Status, "mode": mode["type"], "thinking_blocks": thinkingBlocks, "answer_correct": strings.HasSuffix(strings.TrimSpace(m.text()), "21")}
	if m.Usage.OutputDetails != nil && m.Usage.OutputDetails.ThinkingTokens != nil {
		evidence["thinking_tokens"] = *m.Usage.OutputDetails.ThinkingTokens
	}
	switch {
	case ok && signed:
		r.check("thinking", "pass", "signature_present", evidence)
	case ok && thinkingBlocks > 0:
		r.check("thinking", "fail", "signature_missing", evidence)
	case ok:
		r.check("thinking", "inconclusive", "thinking_not_returned", evidence)
	default:
		r.check("thinking", "inconclusive", "thinking_unavailable", evidence)
	}
	if r.ctx.Err() != nil || r.focusedStop() {
		return
	}

	before := len(r.report.Samples)
	_, response, ok = r.probe("signature_forgery", Request{Body: r.bedrockSignatureBody()})
	if len(r.report.Samples) == before {
		return
	}
	sample := r.report.Samples[len(r.report.Samples)-1]
	evidence = map[string]any{"http_status": response.Status, "mode": mode["type"]}
	switch {
	case response.Status == 400 && bedrockSignatureRejection(sample.Error):
		r.check("signature_forgery", "pass", "forged_signature_rejected", evidence)
	case ok:
		// Anthropic, Bedrock and Vertex all validate signatures in the current
		// tool turn; accepting a forged one means it never reached them intact.
		r.check("signature_forgery", "fail", "forged_signature_accepted", evidence)
	default:
		r.check("signature_forgery", "inconclusive", "forgery_unresolved", evidence)
	}
}

// bedrockMetricsCheck compares Bedrock's own invocation token counts with the
// usage block the client is billed by. Missing metrics are neutral.
func (r *runner) bedrockMetricsCheck() {
	tiers, geos, prefixes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var mismatches []map[string]any
	compared := 0
	for _, sample := range r.report.Samples {
		if sample.Usage.ServiceTier != nil {
			tiers[*sample.Usage.ServiceTier] = true
		}
		if sample.Usage.InferenceGeo != nil {
			geos[*sample.Usage.InferenceGeo] = true
		}
		if prefix := messageIDPrefix(sample.MessageID); prefix != "" {
			prefixes[prefix] = true
		}
		metrics := sample.BedrockMetrics
		if metrics == nil {
			continue
		}
		compared++
		for _, field := range []struct {
			name           string
			usage, metrics *int64
		}{
			{"input", sample.Usage.Input, metrics.Input},
			{"output", sample.Usage.Output, metrics.Output},
			{"cache_read", sample.Usage.CacheRead, metrics.CacheRead},
			{"cache_write", sample.Usage.CacheWrite, metrics.CacheWrite},
		} {
			if field.usage != nil && field.metrics != nil && *field.usage != *field.metrics {
				mismatches = append(mismatches, map[string]any{"probe": sample.Probe, "field": field.name, "usage": *field.usage, "bedrock": *field.metrics})
			}
		}
	}
	evidence := map[string]any{"samples_with_metrics": compared, "service_tiers": sortedKeys(tiers), "inference_geos": sortedKeys(geos), "message_id_prefixes": sortedKeys(prefixes)}
	switch {
	case compared == 0:
		r.check("invocation_metrics", "inconclusive", "metrics_absent", evidence)
	case len(mismatches) > 0:
		evidence["mismatches"] = mismatches
		r.check("invocation_metrics", "fail", "metrics_mismatch", evidence)
	default:
		r.check("invocation_metrics", "pass", "metrics_consistent", evidence)
	}
}

// messageIDPrefix keeps only the shape of a message ID (for example
// "msg_bdrk_01" or "msg_bdrk_"), never the ID itself.
func messageIDPrefix(id string) string {
	for _, prefix := range []string{"msg_bdrk_01", "msg_bdrk_", "msg_vrtx_01", "msg_vrtx_", "msg_01", "msg_"} {
		if strings.HasPrefix(id, prefix) {
			return prefix
		}
	}
	if id != "" {
		return "other"
	}
	return ""
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
