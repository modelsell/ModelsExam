package claudecheck

import (
	"context"
	"errors"
	"net"
	"strings"
)

// FailureCode survives transport error redaction and separates access/route
// failures from a transient timeout. Never infer model quality from these.
func FailureCode(status int, err error) string {
	var networkError net.Error
	switch {
	case status == 408 || status == 504 || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()):
		return "probe_timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case status == 401 || status == 402 || status == 403:
		return "access_denied"
	case status == 404:
		return "endpoint_not_found"
	case err != nil && strings.Contains(strings.ToLower(err.Error()), "no available channel for model"):
		return "route_unavailable"
	case status == 429:
		return "rate_limited"
	case status >= 500:
		return "upstream_unavailable"
	case status >= 300:
		return "request_rejected"
	case err != nil:
		return "transport_error"
	default:
		return ""
	}
}

func baselineRecoverable(code string) bool {
	switch code {
	case "probe_timeout", "transport_error", "upstream_unavailable", "rate_limited", "invalid_response":
		return true
	default:
		return false
	}
}

func MaxRequests(options Options) int {
	options = options.normalized()
	if options.Suite == "focused" {
		n := 1 + PerformanceRequests + UsageTokenRequests + CapabilityRequests
		if options.Cache {
			n += RepeatedCacheRequests
		}
		if options.PromptAudit {
			n += SimplePromptRequests
		}
		if options.PDF {
			n++
		}
		if options.Vision {
			n++
		}
		if options.Bedrock {
			n += BedrockRequests
		}
		return n
	}
	n := 8
	if options.Thinking {
		n += 3
	}
	if options.Cache {
		n += 3
	}
	if options.Repeat {
		n += 3
	}
	if options.Vision {
		n++
	}
	if options.PDF {
		n++
	}
	if options.StreamComparison {
		n += 2
	}
	if options.Benchmark {
		n += BenchmarkRequests
	}
	if options.Performance {
		n += PerformanceRequests
	}
	if options.PromptAudit {
		n += PromptAuditRequests
	}
	if options.Bedrock {
		n += BedrockRequests
	}
	return n
}

func (r *runner) baselineResult(id string, ok bool, response Response) {
	code, state := "observed", "pass"
	if !ok {
		code, state = "invalid_response", "fail"
		if response.ErrorCode != "" && response.ErrorCode != "invalid_response" {
			code, state = response.ErrorCode, "inconclusive"
		}
	}
	if code == "cancelled" {
		state = "skipped"
	}
	r.check(id, state, code, map[string]any{"http_status": response.Status})
}
