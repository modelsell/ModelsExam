package imagecheck

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"model-check/common"
	"model-check/pkg/openaicheck"
)

// NewHTTPTransport sends probes to baseURL (an origin or OpenAI-style /v1
// base) with a bearer key. The supplied client decides network policy and is
// never allowed to follow redirects, so a credential cannot be replayed
// elsewhere. A non-2xx status is returned as a Response, not an error.
func NewHTTPTransport(client *http.Client, baseURL, key string, redactor *openaicheck.Redactor) Transport {
	if redactor == nil {
		redactor = openaicheck.NewRedactor(key)
	}
	redactor.Add(key)
	return func(ctx context.Context, probe Request) (result Response, err error) {
		defer func() {
			if err != nil {
				result.ErrorCode = openaicheck.FailureCode(result.Status, err)
				text := redactor.String(err.Error())
				if r := []rune(text); len(r) > 400 {
					text = string(r[:400])
				}
				err = errors.New(text)
			}
		}()
		if client == nil {
			return result, errors.New("HTTP client is not initialized")
		}
		if probe.Path != "/v1/images/generations" && probe.Path != "/v1/images/edits" {
			return result, errors.New("unsupported probe path")
		}
		var body []byte
		contentType := ""
		if len(probe.Files) > 0 || len(probe.Fields) > 0 {
			body, contentType, err = multipartBody(probe)
		} else {
			body, err = common.Marshal(probe.Body)
			contentType = "application/json"
		}
		if err != nil {
			return result, err
		}
		if len(body) > MaxRequestBytes {
			return result, errors.New("check request exceeds size limit")
		}
		req, err := http.NewRequestWithContext(ctx, probe.Method, baseURL+probe.Path, bytes.NewReader(body))
		if err != nil {
			return result, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
		if probe.Stream {
			req.Header.Set("Accept", "text/event-stream")
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
		if probe.Stream && resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			result.Events, result.FirstEventMS, err = readSSE(resp.Body, start)
			return result, err
		}
		result.Body, err = io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
		if err == nil && len(result.Body) > MaxResponseBytes {
			return result, errors.New("response exceeds size limit")
		}
		return result, err
	}
}

func multipartBody(probe Request) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, value := range probe.Fields {
		if err := w.WriteField(name, value); err != nil {
			return nil, "", err
		}
	}
	for _, f := range probe.Files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="`+f.Field+`"; filename="`+f.Filename+`"`)
		h.Set("Content-Type", f.Mime)
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err = part.Write(f.Data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// readSSE is openaicheck's reader with a bound that fits base64 image frames.
func readSSE(reader io.Reader, start time.Time) (events []openaicheck.SSEEvent, first *int64, err error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxResponseBytes+1))
	scanner.Buffer(make([]byte, 64<<10), MaxResponseBytes)
	var data []string
	name := ""
	consumed := 0
	flush := func() {
		if len(data) == 0 {
			name = ""
			return
		}
		payload := strings.Join(data, "\n")
		frame := name
		data, name = nil, ""
		if strings.TrimSpace(payload) == "[DONE]" {
			return
		}
		if first == nil {
			ms := time.Since(start).Milliseconds()
			first = &ms
		}
		events = append(events, openaicheck.SSEEvent{Name: frame, Data: []byte(payload)})
	}
	for scanner.Scan() {
		line := scanner.Text()
		if consumed += len(line) + 1; consumed > MaxResponseBytes {
			return events, first, errors.New("response exceeds size limit")
		}
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err = scanner.Err(); err != nil {
		return events, first, err
	}
	if len(data) > 0 {
		flush()
	}
	return events, first, nil
}

func parseSize(s string) (w, h int, ok bool) {
	a, b, found := strings.Cut(s, "x")
	if !found {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(a)
	h, err2 := strconv.Atoi(b)
	return w, h, err1 == nil && err2 == nil && w > 0 && h > 0
}
