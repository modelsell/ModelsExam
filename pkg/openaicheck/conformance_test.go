package openaicheck

import (
	"strings"
	"testing"
)

func hasIssue(issues []string, code string) bool {
	for _, issue := range issues {
		if issue == code {
			return true
		}
	}
	return false
}

const chatBody = `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-x",
"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":%q}],
"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8%s}}`

func chat(finish, usageExtra string) []byte {
	return []byte(strings.NewReplacer("%q", `"`+finish+`"`, "%s", usageExtra).Replace(chatBody))
}

func TestChatFinishReasonMustBeDocumented(t *testing.T) {
	for _, ok := range []string{"stop", "length", "tool_calls", "content_filter", "function_call"} {
		if _, issues := parseChat(chat(ok, "")); hasIssue(issues, "unknown_finish_reason") {
			t.Errorf("%s was rejected: %v", ok, issues)
		}
	}
	if _, issues := parseChat(chat("end_turn", "")); !hasIssue(issues, "unknown_finish_reason") {
		t.Errorf("end_turn must not be accepted: %v", issues)
	}
}

func TestUsageDetailCountersAreValidated(t *testing.T) {
	good := `,"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}`
	if _, issues := parseChat(chat("stop", good)); len(issues) != 0 {
		t.Fatalf("conforming usage details rejected: %v", issues)
	}
	negative := `,"prompt_tokens_details":{"cached_tokens":-1}`
	if _, issues := parseChat(chat("stop", negative)); !hasIssue(issues, "invalid_prompt_tokens_details_cached_tokens") {
		t.Errorf("negative cached_tokens accepted: %v", issues)
	}
	tooMany := `,"completion_tokens_details":{"reasoning_tokens":9}`
	if _, issues := parseChat(chat("stop", tooMany)); !hasIssue(issues, "reasoning_tokens_exceed_output") {
		t.Errorf("reasoning above completion tokens accepted: %v", issues)
	}
}

func TestResponsesStatusAndUsageDetails(t *testing.T) {
	body := func(status, usage string) []byte {
		return []byte(`{"id":"resp_1","object":"response","created_at":1,"model":"gpt-x","status":"` + status + `","output":[],"usage":` + usage + `}`)
	}
	usage := `{"input_tokens":4,"output_tokens":2,"total_tokens":6,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":1}}`
	if _, issues := parseResponses(body("completed", usage)); len(issues) != 0 {
		t.Fatalf("conforming response rejected: %v", issues)
	}
	if _, issues := parseResponses(body("done", usage)); !hasIssue(issues, "unknown_status") {
		t.Errorf("undocumented status accepted: %v", issues)
	}
	bad := `{"input_tokens":4,"output_tokens":2,"total_tokens":6,"output_tokens_details":{"reasoning_tokens":5}}`
	if _, issues := parseResponses(body("completed", bad)); !hasIssue(issues, "reasoning_tokens_exceed_output") {
		t.Errorf("reasoning above output accepted: %v", issues)
	}
}

func TestModelListEntriesNeedCreatedAndOwner(t *testing.T) {
	good := `{"object":"list","data":[{"id":"gpt-x","object":"model","created":1,"owned_by":"openai"}]}`
	if listed, _, issues := parseModels([]byte(good), "gpt-x"); !listed || len(issues) != 0 {
		t.Fatalf("conforming list rejected: %v", issues)
	}
	bare := `{"object":"list","data":[{"id":"gpt-x","object":"model"}]}`
	_, _, issues := parseModels([]byte(bare), "gpt-x")
	if !hasIssue(issues, "model_missing_created") || !hasIssue(issues, "model_missing_owned_by") {
		t.Errorf("missing created/owned_by not reported: %v", issues)
	}
}
