// Package modellist asks an endpoint which models it serves (GET /v1/models),
// so the check form can offer them as choices. The reply is untrusted text from
// a remote site: ids are length-limited and de-duplicated here, and the
// credential is only ever sent to the endpoint the visitor named.
package modellist

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	KindClaude = "claude"
	KindOpenAI = "openai"
	KindImage  = "image"

	maxBody    = 1 << 20
	maxModels  = 500
	maxIDRunes = 120
	timeout    = 12 * time.Second
)

// Error codes the form turns into a message. They never carry upstream text.
var (
	ErrURL         = errors.New("url")
	ErrUnreachable = errors.New("unreachable")
	ErrAuth        = errors.New("auth")
	ErrNotFound    = errors.New("not_found")
	ErrStatus      = errors.New("status")
	ErrFormat      = errors.New("format")
)

// Code returns the stable machine name of an error from Fetch.
func Code(err error) string {
	for _, e := range []error{ErrURL, ErrAuth, ErrNotFound, ErrStatus, ErrFormat} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return ErrUnreachable.Error()
}

// Result lists the models. Filtered says the list was narrowed to the check
// type; when nothing matched, All is returned instead with Filtered=false.
type Result struct {
	Models   []string `json:"models"`
	Total    int      `json:"total"`
	Filtered bool     `json:"filtered"`
}

type Validator func(ctx context.Context, rawURL string) error

// ModelsURL turns whatever the visitor pasted (origin, /v1 base or a full
// endpoint) into <origin and prefix>/v1/models. Credentials, queries and
// fragments are refused.
func ModelsURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || len(raw) > 2048 || u == nil || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", ErrURL
	}
	p := strings.TrimRight(u.Path, "/")
	for _, s := range []string{"/messages", "/chat/completions", "/responses", "/images/generations", "/models"} {
		p = strings.TrimSuffix(p, s)
	}
	p = strings.TrimSuffix(p, "/v1")
	u.Path, u.RawPath = p+"/v1/models", ""
	return u.String(), nil
}

// Fetch reads the model list. hc must not follow redirects (a redirect would
// carry the key to another host); validate is the SSRF check.
func Fetch(ctx context.Context, hc *http.Client, validate Validator, base, key, kind string) (Result, error) {
	endpoint, err := ModelsURL(base)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if validate != nil {
		if err := validate(ctx, endpoint); err != nil {
			return Result{}, ErrUnreachable
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{}, ErrURL
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ModelsExam-model-list/1.0")
	req.Header.Set("Authorization", "Bearer "+key)
	if kind == KindClaude {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Result{}, ErrUnreachable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Result{}, ErrAuth
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		return Result{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return Result{}, ErrStatus
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Result{}, ErrUnreachable
	}
	ids, err := Parse(body)
	if err != nil {
		return Result{}, err
	}
	return Select(ids, kind), nil
}

// Parse accepts {"data":[{"id":..}]}, {"models":[...]} and a bare array of
// ids or objects.
func Parse(body []byte) ([]string, error) {
	var root any
	if json.Unmarshal(body, &root) != nil {
		return nil, ErrFormat
	}
	var list []any
	switch v := root.(type) {
	case []any:
		list = v
	case map[string]any:
		for _, k := range []string{"data", "models"} {
			if l, ok := v[k].([]any); ok {
				list = l
				break
			}
		}
	}
	if list == nil {
		return nil, ErrFormat
	}
	seen := map[string]bool{}
	var ids []string
	for _, item := range list {
		var id string
		switch v := item.(type) {
		case string:
			id = v
		case map[string]any:
			for _, k := range []string{"id", "name", "model"} {
				if s, ok := v[k].(string); ok && s != "" {
					id = s
					break
				}
			}
		}
		id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
		if id == "" || len([]rune(id)) > maxIDRunes || strings.ContainsAny(id, "\r\n\t") || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if len(ids) >= maxModels {
			break
		}
	}
	return ids, nil
}

var (
	claudeRe = regexp.MustCompile(`(?i)claude`)
	imageRe  = regexp.MustCompile(`(?i)(gpt-image|dall-e|image|imagen|flux|midjourney|sdxl|stable-diffusion)`)
	// Chat-capable OpenAI-style models: skip embeddings, speech, images, moderation.
	openaiRe    = regexp.MustCompile(`(?i)(gpt|^o\d|chatgpt|codex|deepseek|qwen|gemini|glm|kimi|llama|mistral|grok)`)
	openaiNotRe = regexp.MustCompile(`(?i)(embed|whisper|tts|audio|transcribe|moderation|rerank|image|dall-e|realtime|claude)`)
)

// Select narrows ids to the check type and sorts them so the newest-looking
// names come first. If nothing matches, every id is returned unfiltered.
func Select(ids []string, kind string) Result {
	var match []string
	for _, id := range ids {
		switch kind {
		case KindClaude:
			if claudeRe.MatchString(id) {
				match = append(match, id)
			}
		case KindImage:
			if imageRe.MatchString(id) {
				match = append(match, id)
			}
		default:
			if openaiRe.MatchString(id) && !openaiNotRe.MatchString(id) {
				match = append(match, id)
			}
		}
	}
	res := Result{Total: len(ids), Filtered: len(match) > 0}
	if len(match) == 0 {
		match = append([]string(nil), ids...)
	}
	sort.SliceStable(match, func(i, j int) bool { return match[i] > match[j] })
	res.Models = match
	return res
}
