package geminicheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"model-check/common"
	"model-check/pkg/openaicheck"
)

// MaxRequestBytes and MaxTokenLimit keep a bounded diagnostic from becoming an
// unbounded job, whatever a future probe tries to send.
const (
	MaxRequestBytes = 128 << 10
	MaxTokenLimit   = 4096
)

// ModelID strips an optional "models/" resource prefix.
func ModelID(model string) string {
	return strings.TrimPrefix(strings.TrimSpace(model), "models/")
}

// NormalizeBaseURL accepts a bare origin, a ".../v1beta" base, or a full
// endpoint URL, and returns the origin plus any custom path prefix without the
// API version. Credentials, queries and fragments in the URL are refused, so a
// "?key=" query can never carry the key past the redactor.
func NormalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || len(raw) > 2048 || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("invalid base URL")
	}
	path := strings.TrimRight(u.Path, "/")
	if i := strings.Index(path, "/models"); i >= 0 {
		path = path[:i]
	}
	for _, version := range []string{"/v1beta", "/v1alpha", "/v1"} {
		path = strings.TrimSuffix(path, version)
	}
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
	config, _ := payload["generationConfig"].(map[string]any)
	if value, ok := config["maxOutputTokens"]; ok {
		n, isNumber := value.(float64)
		if !isNumber || n < 1 || n > MaxTokenLimit || n != float64(int64(n)) {
			return errors.New("check requests require maxOutputTokens between 1 and 4096")
		}
	}
	return nil
}

// FailureCode classifies a transport result. Gemini reports a bad API key as
// 400 INVALID_ARGUMENT with reason API_KEY_INVALID, so the body is consulted
// before a 400 is treated as a rejected input.
func FailureCode(status int, body []byte, err error) string {
	if status == http.StatusBadRequest && (bytes.Contains(body, []byte("API_KEY_INVALID")) || bytes.Contains(body, []byte("API key not valid"))) {
		return "unauthorized"
	}
	if status == http.StatusPaymentRequired {
		return "forbidden"
	}
	return openaicheck.FailureCode(status, err)
}

// IsAvailabilityCode reports codes that make a result inconclusive.
func IsAvailabilityCode(code string) bool { return openaicheck.IsAvailabilityCode(code) }

// NewHTTPTransport sends probes with the x-goog-api-key header. The supplied
// client decides network policy and is never allowed to follow redirects, so a
// credential cannot be replayed elsewhere. A non-2xx status is returned as a
// Response, not an error; only transport failures are errors.
func NewHTTPTransport(client *http.Client, baseURL, key string, redactor *openaicheck.Redactor) Transport {
	if redactor == nil {
		redactor = openaicheck.NewRedactor(key)
	}
	redactor.Add(key)
	return func(ctx context.Context, probe Request) (result Response, err error) {
		defer func() {
			if err != nil {
				result.ErrorCode = FailureCode(result.Status, result.Body, err)
				err = errors.New(truncate(redactor.String(err.Error()), 400))
			}
		}()
		if client == nil {
			return result, errors.New("HTTP client is not initialized")
		}
		if !strings.HasPrefix(probe.Path, "/"+APIVersion+"/models/") {
			return result, errors.New("unsupported probe path")
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
		req, err := http.NewRequestWithContext(ctx, probe.Method, baseURL+probe.Path, reader)
		if err != nil {
			return result, err
		}
		req.Header.Set("x-goog-api-key", key)
		req.Header.Set("Accept", "application/json")
		if probe.Stream {
			req.Header.Set("Accept", "text/event-stream")
		}
		if reader != nil {
			req.Header.Set("Content-Type", "application/json")
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
			result.Events, result.Done, result.FirstEventMS, err = openaicheck.ReadSSE(resp.Body, start)
			return result, err
		}
		result.Body, err = io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
		if err == nil && len(result.Body) > MaxResponseBytes {
			return result, errors.New("response exceeds size limit")
		}
		return result, err
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}
