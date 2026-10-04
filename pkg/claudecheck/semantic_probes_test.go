package claudecheck

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func isDocument(req Request) bool {
	if req.Count {
		return false
	}
	content, ok := req.Body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	return ok && content[0].(map[string]any)["type"] == "document"
}

func isComparison(req Request) bool {
	if req.Count {
		return false
	}
	return req.Body["messages"].([]any)[0].(map[string]any)["content"] == comparisonPrompt
}

func semanticTransport(t *testing.T) Transport {
	base := happyTransport(t, true)
	return func(ctx context.Context, req Request) (Response, error) {
		res, err := base(ctx, req)
		if req.Count {
			return res, err
		}
		var m message
		if len(res.Body) > 0 {
			require.NoError(t, common.Unmarshal(res.Body, &m))
		}
		if isDocument(req) {
			content := req.Body["messages"].([]any)[0].(map[string]any)["content"].([]any)
			source := content[0].(map[string]any)["source"].(map[string]any)
			require.Equal(t, "application/pdf", source["media_type"])
			data, err := base64.StdEncoding.DecodeString(source["data"].(string))
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(string(data), "%PDF-1.4"))
			identifier := regexp.MustCompile(`ORDER-[0-9A-F]{8}`).FindString(string(data))
			require.NotEmpty(t, identifier)
			require.NotContains(t, content[1].(map[string]any)["text"], identifier)
			m.Content = []map[string]any{{"type": "text", "text": identifier}}
			res = testResponse(t, m)
		} else if isComparison(req) {
			if len(res.Events) > 0 {
				for i := range res.Events {
					res.Events[i] = []byte(strings.ReplaceAll(string(res.Events[i]), "PONG", `{\"verify\":\"abc123\",\"n\":42}`))
				}
			} else {
				m.Content = []map[string]any{{"type": "text", "text": "```json\n{\"n\":42,\"verify\":\"abc123\"}\n```"}}
				res = testResponse(t, m)
			}
		}
		return res, err
	}
}

func TestIntegratedPlanAndBudgets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		options  Options
		requests int
	}{
		{"quick", Options{Model: "claude"}, 8},
		{"standard", Options{Model: "claude", Thinking: true, Cache: true, PDF: true, StreamComparison: true}, 17},
		{"extended", Options{Model: "claude", Thinking: true, Cache: true, Repeat: true, Vision: true, PDF: true, StreamComparison: true}, 21},
		{"legacy client", Options{Model: "claude", LegacySuite: true}, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Run(context.Background(), tc.options, semanticTransport(t))
			require.Equal(t, 8, r.Version)
			require.Len(t, r.Plan, 28)
			require.Len(t, r.Checks, len(r.Plan))
			require.Len(t, r.Samples, tc.requests)
			require.Equal(t, tc.requests, r.Limits.MaxRequests)
			require.Nil(t, r.Reference)
			require.False(t, r.Options.LegacySuite)
			require.Equal(t, "pass", resultCheck(t, r, "tool").Status)
			if tc.options.PDF || tc.options.LegacySuite {
				for _, id := range []string{"pdf", "stream_comparison", "stream_stop_reason"} {
					require.Equal(t, "pass", resultCheck(t, r, id).Status, id)
				}
			}
			wire, err := common.Marshal(r)
			require.NoError(t, err)
			for _, text := range []string{"veridrop", "source_url", "snapshots", "private reasoning", "opaque-signature"} {
				require.NotContains(t, string(wire), text)
			}
		})
	}
}

func TestSemanticScoringBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, probe, state, code string
		mutate                   func(*message)
		http                     int
	}{
		{"PDF wrong identifier", "pdf", "fail", "semantic_observed", func(m *message) { m.Content = []map[string]any{{"type": "text", "text": "WRONG"}} }, 200},
		{"PDF refusal", "pdf", "inconclusive", "request_refused", func(m *message) {
			m.StopReason = "refusal"
			m.Content = []map[string]any{}
			z := int64(0)
			m.Usage.Output = &z
		}, 200},
		{"PDF partial refusal", "pdf", "inconclusive", "request_refused", func(m *message) { m.StopReason = "refusal" }, 200},
		{"PDF truncated", "pdf", "inconclusive", "answer_truncated", func(m *message) {
			m.StopReason = "max_tokens"
			m.Content = []map[string]any{{"type": "text", "text": "ORDER-"}}
		}, 200},
		{"PDF missing content", "pdf", "fail", "invalid_response", func(m *message) { m.Content = nil }, 200},
		{"PDF unsupported", "pdf", "inconclusive", "request_rejected", nil, 400},
		{"JSON wrong content", "stream_comparison", "fail", "semantic_observed", func(m *message) { m.Content = []map[string]any{{"type": "text", "text": "{}"}} }, 200},
		{"JSON unavailable", "stream_comparison", "inconclusive", "rate_limited", nil, 429},
		{"JSON refusal", "stream_comparison", "inconclusive", "request_refused", func(m *message) { m.StopReason = "refusal"; m.Content = []map[string]any{} }, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			good := semanticTransport(t)
			r := Run(context.Background(), Options{Model: "claude", PDF: true, StreamComparison: true}, func(ctx context.Context, req Request) (Response, error) {
				res, err := good(ctx, req)
				if (tc.probe == "pdf" && isDocument(req)) || (tc.probe == "stream_comparison" && isComparison(req) && req.Body["stream"] != true) {
					if tc.http != 200 {
						return Response{Status: tc.http}, nil
					}
					var m message
					require.NoError(t, common.Unmarshal(res.Body, &m))
					tc.mutate(&m)
					return testResponse(t, m), nil
				}
				return res, err
			})
			check := resultCheck(t, r, tc.probe)
			require.Equal(t, tc.state, check.Status)
			require.Equal(t, tc.code, check.Code)
			if tc.name == "PDF refusal" {
				for _, s := range r.Samples {
					if s.Probe == "pdf" {
						require.True(t, *s.Valid)
						require.Empty(t, s.ValidationErrors)
					}
				}
			}
		})
	}
}

func TestStreamSemanticsAndMetadataScoreSeparately(t *testing.T) {
	good := semanticTransport(t)
	r := Run(context.Background(), Options{Model: "claude", StreamComparison: true}, func(ctx context.Context, req Request) (Response, error) {
		res, err := good(ctx, req)
		if isComparison(req) && req.Body["stream"] != true {
			var m message
			require.NoError(t, common.Unmarshal(res.Body, &m))
			m.StopReason = "max_tokens"
			res = testResponse(t, m)
		}
		return res, err
	})
	require.Equal(t, "pass", resultCheck(t, r, "stream_comparison").Status)
	require.Equal(t, "fail", resultCheck(t, r, "stream_stop_reason").Status)
}

func TestStructuredToolRejectsSchemaViolations(t *testing.T) {
	for _, mutation := range []func(map[string]any){
		func(b map[string]any) { b["input"].(map[string]any)["unit"] = "kelvin" },
		func(b map[string]any) { b["input"].(map[string]any)["extra"] = true },
		func(b map[string]any) { delete(b["input"].(map[string]any), "city") },
		func(b map[string]any) { b["name"] = "unexpected_tool" },
	} {
		good := semanticTransport(t)
		r := Run(context.Background(), Options{Model: "claude"}, func(ctx context.Context, req Request) (Response, error) {
			res, err := good(ctx, req)
			if req.Body["tools"] != nil {
				var m message
				require.NoError(t, common.Unmarshal(res.Body, &m))
				mutation(m.Content[0])
				res = testResponse(t, m)
			}
			return res, err
		})
		require.Equal(t, "fail", resultCheck(t, r, "tool").Status)
	}
}

func TestPDFXrefAndCancellation(t *testing.T) {
	pdf := string(documentFixture("ORDER-12345678"))
	offsets := regexp.MustCompile(`(?m)^(\d{10}) 00000 n `).FindAllStringSubmatch(pdf, -1)
	require.Len(t, offsets, 5)
	for i, match := range offsets {
		offset, err := strconv.Atoi(match[1])
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(pdf[offset:], fmt.Sprintf("%d 0 obj", i+1)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	good := semanticTransport(t)
	calls := 0
	r := Run(ctx, Options{Model: "claude", PDF: true, StreamComparison: true}, func(ctx context.Context, req Request) (Response, error) {
		res, err := good(ctx, req)
		calls++
		if isDocument(req) {
			cancel()
		}
		return res, err
	})
	require.Equal(t, 9, calls)
	require.True(t, r.Cancelled)
	for _, id := range []string{"pdf", "stream_comparison", "stream_stop_reason"} {
		require.Equal(t, "skipped", resultCheck(t, r, id).Status)
	}
}

func TestRefusalNeverEstablishesBaselineOrReplay(t *testing.T) {
	for _, probe := range []string{"basic", "signature_replay"} {
		active := ""
		good := semanticTransport(t)
		r := RunWithObserver(context.Background(), Options{Model: "claude", Thinking: true}, func(ctx context.Context, req Request) (Response, error) {
			res, err := good(ctx, req)
			if active == probe {
				var m message
				require.NoError(t, common.Unmarshal(res.Body, &m))
				m.StopReason = "refusal"
				m.Content = []map[string]any{}
				res = testResponse(t, m)
			}
			return res, err
		}, func(e Event) {
			if e.Type == "probe_start" {
				active = e.Probe
			}
		})
		c := resultCheck(t, r, probe)
		require.Equal(t, "inconclusive", c.Status)
		require.Equal(t, "request_refused", c.Code)
		if probe == "basic" {
			require.Equal(t, "request_refused", r.StopReason)
			require.Len(t, r.Samples, 1)
		}
	}
}

func TestComparisonChecksExpectedValuesNotJustEquality(t *testing.T) {
	r := Run(context.Background(), Options{Model: "claude", StreamComparison: true}, happyTransport(t, true))
	require.Equal(t, "fail", resultCheck(t, r, "stream_comparison").Status)
	require.Equal(t, "pass", resultCheck(t, r, "stream_stop_reason").Status)
	for _, text := range []string{`{"verify":"abc123","n":"42"}`, `{"verify":"abc123","n":42,"extra":true}`, `{}`, `PONG`} {
		require.False(t, comparisonJSONMatched(text))
	}
}

func TestComparisonStreamRefusalIsValidButUnscored(t *testing.T) {
	good := semanticTransport(t)
	r := Run(context.Background(), Options{Model: "claude", StreamComparison: true}, func(ctx context.Context, req Request) (Response, error) {
		res, err := good(ctx, req)
		if isComparison(req) && req.Body["stream"] == true {
			var start map[string]any
			require.NoError(t, common.Unmarshal(res.Events[0], &start))
			start["message"].(map[string]any)["content"] = []any{}
			res.Events = nil
			for _, e := range []any{start, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "refusal"}, "usage": map[string]any{"output_tokens": 0}}, map[string]any{"type": "message_stop"}} {
				b, err := common.Marshal(e)
				require.NoError(t, err)
				res.Events = append(res.Events, b)
			}
		}
		return res, err
	})
	for _, id := range []string{"stream_comparison", "stream_stop_reason"} {
		require.Equal(t, "inconclusive", resultCheck(t, r, id).Status)
		require.Equal(t, "request_refused", resultCheck(t, r, id).Code)
	}
	for _, s := range r.Samples {
		if s.Probe == "comparison_stream" {
			require.True(t, *s.Valid)
		}
	}
}
