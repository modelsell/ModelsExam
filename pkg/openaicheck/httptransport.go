package openaicheck

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
)

// MaxRequestBytes and MaxTokenLimit keep a bounded diagnostic from becoming an
// unbounded job, whatever a future probe or override tries to send.
const (
	MaxRequestBytes = 128 << 10
	MaxTokenLimit   = 4096
)

// NormalizeBaseURL accepts a bare origin, an OpenAI-style /v1 base, or a full
// endpoint URL, and returns the origin plus any custom path prefix, without a
// trailing /v1. Credentials, queries and fragments in the URL are refused.
func NormalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || len(raw) > 2048 || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("invalid base URL")
	}
	path := strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/chat/completions", "/responses", "/models"} {
		path = strings.TrimSuffix(path, suffix)
	}
	path = strings.TrimSuffix(path, "/v1")
	u.Path, u.RawPath = path, ""
	return strings.TrimRight(u.String(), "/"), nil
}

// ValidatePayload bounds a generated request body.
func ValidatePayload(body []byte) error {
	if len(body) > MaxRequestBytes {
		return errors.New("check request exceeds size limit")
	}
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil {
		return nil // GET probes have no body
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if value, ok := payload[key]; ok {
			n, isNumber := value.(float64)
			if !isNumber || n < 0 || n > MaxTokenLimit || n != float64(int64(n)) {
				return errors.New("check requests require " + key + " between 0 and 4096")
			}
		}
	}
	return nil
}

// Redactor removes credentials from anything that leaves the server.
type Redactor struct{ secrets []string }

func NewRedactor(secrets ...string) *Redactor {
	r := &Redactor{}
	r.Add(secrets...)
	return r
}

func (r *Redactor) Add(secrets ...string) {
	for _, s := range secrets {
		if len(s) >= 4 {
			r.secrets = append(r.secrets, s)
			r.secrets = append(r.secrets, strings.TrimPrefix(strings.TrimPrefix(s, "Bearer "), "Basic "))
		}
	}
	sort.Slice(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
}

func (r *Redactor) String(text string) string {
	for _, secret := range r.secrets {
		if len(secret) >= 4 {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

// JSON redacts a JSON document, matching secrets in their escaped form too.
func (r *Redactor) JSON(data []byte) []byte {
	for _, secret := range r.secrets {
		if len(secret) < 4 {
			continue
		}
		if encoded, err := common.Marshal(secret); err == nil {
			data = bytes.ReplaceAll(data, encoded[1:len(encoded)-1], []byte("[REDACTED]"))
		}
	}
	return data
}

// NewHTTPTransport sends probes to baseURL with a bearer key. The supplied
// client decides network policy (for example SSRF protection) and is never
// allowed to follow redirects, so a credential cannot be replayed elsewhere.
// A non-2xx status is returned as a Response, not an error; only transport
// failures are errors.
func NewHTTPTransport(client *http.Client, baseURL, key string, redactor *Redactor) Transport {
	if redactor == nil {
		redactor = NewRedactor(key)
	}
	redactor.Add(key)
	return func(ctx context.Context, probe Request) (result Response, err error) {
		defer func() {
			if err != nil {
				result.ErrorCode = FailureCode(result.Status, err)
				text := redactor.String(err.Error())
				err = errors.New(truncate(text, 400))
			}
		}()
		if client == nil {
			return result, errors.New("HTTP client is not initialized")
		}
		var reader io.Reader
		if probe.Body != nil {
			body, marshalErr := common.Marshal(probe.Body)
			if marshalErr != nil {
				return result, marshalErr
			}
			if err = ValidatePayload(body); err != nil {
				return result, err
			}
			reader = bytes.NewReader(body)
		}
		if !strings.HasPrefix(probe.Path, "/v1/") {
			return result, errors.New("unsupported probe path")
		}
		req, err := http.NewRequestWithContext(ctx, probe.Method, baseURL+probe.Path, reader)
		if err != nil {
			return result, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Accept", "application/json")
		if probe.Stream {
			req.Header.Set("Accept", "text/event-stream")
		}
		if reader != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if model, ok := probe.Body["model"].(string); ok {
			result.Model = model
		}
		bounded := *client
		bounded.Timeout = ProbeTimeout
		bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		start := time.Now()
		resp, err := bounded.Do(req)
		if err != nil {
			return result, err
		}
		defer resp.Body.Close()
		result.Status, result.Header = resp.StatusCode, resp.Header
		if probe.Stream && resp.StatusCode == http.StatusOK {
			result.Events, result.Done, result.FirstEventMS, err = ReadSSE(resp.Body, start)
			return result, err
		}
		result.Body, err = io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
		if err == nil && len(result.Body) > MaxResponseBytes {
			return result, errors.New("response exceeds size limit")
		}
		return result, err
	}
}
