package openaicheck

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReadSSE reads the whole stream with a hard size bound. Unlike a plain data
// reader it keeps the optional "event:" name (the Responses API uses it) and
// reports whether the literal [DONE] terminator arrived (Chat Completions).
func ReadSSE(reader io.Reader, start time.Time) (events []SSEEvent, done bool, first *int64, err error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxResponseBytes+1))
	scanner.Buffer(make([]byte, 4096), MaxResponseBytes)
	var data []string
	name := ""
	consumed := 0
	flush := func() {
		if len(data) == 0 {
			name = ""
			return
		}
		payload := strings.Join(data, "\n")
		frameName := name
		data, name = nil, ""
		if strings.TrimSpace(payload) == "[DONE]" {
			done = true
			return
		}
		if first == nil {
			ms := time.Since(start).Milliseconds()
			first = &ms
		}
		events = append(events, SSEEvent{Name: frameName, Data: []byte(payload)})
	}
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(line) + 1
		if consumed > MaxResponseBytes {
			return events, done, first, errors.New("response exceeds size limit")
		}
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, ":"): // comment / keep-alive
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err = scanner.Err(); err != nil {
		return events, done, first, err
	}
	// Some servers omit the final blank line before closing; a trailing [DONE]
	// frame is still a complete terminator, anything else is truncated.
	if len(data) > 0 {
		if strings.TrimSpace(strings.Join(data, "\n")) == "[DONE]" {
			done = true
			return events, done, first, nil
		}
		return events, done, first, errors.New("incomplete SSE frame")
	}
	return events, done, first, nil
}

// FailureCode classifies a transport result. Availability codes describe the
// path to the model (credentials, quota, network) and never count against the
// model's API compatibility; request_rejected means the endpoint understood the
// request and refused an input that OpenAI accepts, which is a finding.
func FailureCode(status int, err error) string {
	switch {
	case status == http.StatusUnauthorized:
		return "unauthorized"
	case status == http.StatusForbidden:
		return "forbidden"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status >= 500:
		return "upstream_error"
	case status >= 400:
		return "request_rejected"
	case err != nil:
		text := err.Error()
		if strings.Contains(text, "deadline exceeded") || strings.Contains(text, "timeout") {
			return "timeout"
		}
		if strings.Contains(text, "context canceled") {
			return "cancelled"
		}
		return "network_error"
	}
	return ""
}

// IsAvailabilityCode reports codes that make a result inconclusive.
func IsAvailabilityCode(code string) bool {
	switch code {
	case "unauthorized", "forbidden", "rate_limited", "upstream_error", "timeout", "network_error", "cancelled", "access_denied":
		return true
	}
	return false
}
