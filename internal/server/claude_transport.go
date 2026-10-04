package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"model-check/common"
	"model-check/pkg/claudecheck"
)

// claudeTransport sends the generated probes to an Anthropic-compatible
// /v1/messages endpoint using the caller's own base URL and key.
type claudeTransport struct {
	client     *http.Client
	baseURL    string // without /v1/messages
	key        string
	secrets    []string
	countBody  []byte
	countModel string
	countProf  string
}

func newClaudeTransport(client *http.Client, baseURL, key string) *claudeTransport {
	t := &claudeTransport{client: client, baseURL: baseURL, key: key, secrets: []string{key}}
	if u, err := url.Parse(baseURL); err == nil {
		for _, values := range u.Query() {
			t.secrets = append(t.secrets, values...)
		}
	}
	return t
}

func (t *claudeTransport) redactText(text string) string {
	for _, s := range t.secrets {
		if len(s) >= 4 {
			text = strings.ReplaceAll(text, s, "[REDACTED]")
		}
	}
	return text
}

func (t *claudeTransport) redactJSON(data []byte) []byte {
	sort.Slice(t.secrets, func(i, j int) bool { return len(t.secrets[i]) > len(t.secrets[j]) })
	for _, secret := range t.secrets {
		if len(secret) < 4 {
			continue
		}
		if encoded, err := common.Marshal(secret); err == nil {
			data = bytes.ReplaceAll(data, encoded[1:len(encoded)-1], []byte("[REDACTED]"))
		}
	}
	return data
}

// validatePayload keeps a bounded diagnostic from becoming an unbounded job.
func validatePayload(body []byte) error {
	if len(body) > 128<<10 {
		return errors.New("check request exceeds size limit")
	}
	var p struct {
		MaxTokens *float64 `json:"max_tokens"`
	}
	if common.Unmarshal(body, &p) != nil || p.MaxTokens == nil || *p.MaxTokens < 0 || *p.MaxTokens > 2048 || *p.MaxTokens != float64(int64(*p.MaxTokens)) {
		return errors.New("check requests require max_tokens between 0 and 2048")
	}
	return nil
}

func (t *claudeTransport) call(ctx context.Context, probe claudecheck.Request) (result claudecheck.Response, err error) {
	defer func() {
		result.Diagnostic = claudecheck.DiagnoseBedrock(result, err)
		if err != nil {
			result.ErrorCode = claudecheck.FailureCode(result.Status, err)
			text := t.redactText(err.Error())
			if len(text) > 1200 {
				text = text[:1200]
			}
			err = errors.New(text)
		}
	}()
	if u, e := url.Parse(t.baseURL); e == nil {
		result.Bedrock = strings.HasPrefix(u.Hostname(), "bedrock-mantle.") && strings.HasSuffix(u.Hostname(), ".api.aws")
	}
	body, err := common.Marshal(probe.Body)
	if err != nil {
		return result, err
	}
	if err = validatePayload(body); err != nil {
		return result, err
	}
	endpoint := t.baseURL + "/v1/messages"
	stream, _ := probe.Body["stream"].(bool)
	result.Model, _ = probe.Body["model"].(string)
	result.PromptProfile = claudecheck.PromptContentProfile(body)
	result.RequestProfile = claudecheck.RequestProfile(body)
	if probe.Beta != "" {
		result.RequestProfile = ""
	}
	if !probe.Count {
		var mt struct {
			MaxTokens *int64 `json:"max_tokens"`
		}
		if common.Unmarshal(body, &mt) == nil {
			result.EffectiveMaxTokens = mt.MaxTokens
		}
	}
	if probe.Count {
		if t.countBody == nil {
			return result, errors.New("no baseline request available for token counting")
		}
		result.RequestProfile, result.Model = t.countProf, t.countModel
		var source map[string]any
		if err = common.Unmarshal(t.countBody, &source); err != nil {
			return result, err
		}
		count := map[string]any{}
		for _, key := range []string{"model", "messages", "system", "tools", "tool_choice", "thinking"} {
			if value, ok := source[key]; ok {
				count[key] = value
			}
		}
		if body, err = common.Marshal(count); err != nil {
			return result, err
		}
		endpoint += "/count_tokens"
		stream = false
	} else {
		t.countBody = append([]byte(nil), body...)
		t.countModel, t.countProf = result.Model, result.RequestProfile
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", t.key)
	req.Header.Set("anthropic-version", "2023-06-01")
	if probe.Beta != "" {
		req.Header.Set("anthropic-beta", probe.Beta)
	}
	bounded := *t.client
	bounded.Timeout = claudecheck.ProbeTimeout
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	start := time.Now()
	resp, err := bounded.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	result.Status, result.Header = resp.StatusCode, resp.Header
	if stream && !probe.Count && resp.StatusCode == http.StatusOK {
		result.Stream = &claudecheck.StreamMetrics{}
		result.Events, result.FirstEventMS, err = claudecheck.ReadSSEWithMetrics(resp.Body, start, result.Stream)
	} else {
		result.Body, err = io.ReadAll(io.LimitReader(resp.Body, claudecheck.MaxResponseBytes+1))
		if len(result.Body) > claudecheck.MaxResponseBytes {
			return result, errors.New("response exceeds size limit")
		}
	}
	if err == nil && resp.StatusCode != http.StatusOK {
		err = responseError(result.Body)
	}
	return result, err
}

func responseError(body []byte) error {
	var e struct {
		Message string `json:"message"`
		Error   struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if common.Unmarshal(body, &e) != nil {
		return errors.New("upstream returned a non-JSON error")
	}
	if e.Error.Message != "" {
		if e.Error.Type != "" && e.Error.Type != "<nil>" {
			return errors.New(e.Error.Type + ": " + e.Error.Message)
		}
		return errors.New(e.Error.Message)
	}
	if e.Message != "" {
		return errors.New(e.Message)
	}
	return errors.New("upstream request failed")
}
