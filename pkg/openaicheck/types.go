// Package openaicheck runs bounded, synthetic OpenAI-compatible API probes.
//
// It checks both the request side (which OpenAI parameters an endpoint accepts)
// and the response side (whether the returned envelope, streaming events,
// tool calls, structured output and usage fields match the OpenAI contract).
// Responses are observations of API behavior; they are never proof that the
// upstream is the genuine OpenAI model named in the request.
package openaicheck

import (
	"context"
	"net/http"
	"time"
)

const (
	MaxResponseBytes = 2 << 20
	ProbeTimeout     = 90 * time.Second
	RunTimeout       = 10 * time.Minute
	ReportVersion    = 1

	// Probe kinds select which response contract a sample is validated against.
	KindModels    = "models"
	KindChat      = "chat"
	KindResponses = "responses"
	KindError     = "error"
)

type Options struct {
	Model string `json:"model"`
	// Suite is "basic", "standard" (default) or "full".
	Suite string `json:"suite,omitempty"`
	// Responses adds the /v1/responses suite on top of the selected suite.
	Responses bool `json:"responses,omitempty"`
	// Vision adds an image_url probe; Logprobs adds a logprobs probe.
	Vision   bool `json:"vision,omitempty"`
	Logprobs bool `json:"logprobs,omitempty"`
	// LimitParam is the chat token-limit parameter: "max_completion_tokens"
	// (default, current OpenAI name) or "max_tokens" (legacy compatible servers).
	LimitParam string `json:"limit_param,omitempty"`
}

const (
	SuiteBasic    = "basic"
	SuiteStandard = "standard"
	SuiteFull     = "full"
)

// ValidOptions rejects unknown enum values before any upstream request.
func ValidOptions(o Options) bool {
	switch o.Suite {
	case "", SuiteBasic, SuiteStandard, SuiteFull:
	default:
		return false
	}
	switch o.LimitParam {
	case "", "max_completion_tokens", "max_tokens":
	default:
		return false
	}
	return o.Model != "" && len(o.Model) <= 200
}

func (o Options) normalized() Options {
	if o.Suite == "" {
		o.Suite = SuiteStandard
	}
	if o.LimitParam == "" {
		o.LimitParam = "max_completion_tokens"
	}
	if o.Suite == SuiteFull {
		o.Responses, o.Logprobs = true, true
	}
	return o
}

func (o Options) full() bool     { return o.Suite == SuiteFull }
func (o Options) basic() bool    { return o.Suite == SuiteBasic }
func (o Options) standard() bool { return !o.basic() }

// Request is generated locally; callers cannot supply arbitrary probe payloads.
type Request struct {
	Method string
	Path   string // e.g. /v1/chat/completions; the transport prefixes the base URL
	Body   map[string]any
	Stream bool
}

// SSEEvent is one server-sent event frame.
type SSEEvent struct {
	Name string `json:"event,omitempty"`
	Data []byte `json:"-"`
}

type Response struct {
	Status       int
	Header       http.Header
	Body         []byte
	Events       []SSEEvent
	Done         bool // a literal "data: [DONE]" frame was received
	FirstEventMS *int64
	ErrorCode    string
	Model        string // upstream model name sent after any channel mapping
}

type Transport func(context.Context, Request) (Response, error)

// Event is the progress stream. Request bodies and credentials never enter it.
type Event struct {
	Type   string  `json:"type"`
	Probe  string  `json:"probe,omitempty"`
	Report *Report `json:"report,omitempty"`
	Sample *Sample `json:"sample,omitempty"`
	Check  *Check  `json:"check,omitempty"`
	// Markdown is set only on the terminal "done" event by the caller, after
	// credentials have been redacted from the report.
	Markdown string `json:"markdown,omitempty"`
}

type Usage struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
	Total  *int64 `json:"total_tokens"`
}

type Sample struct {
	Probe            string            `json:"probe"`
	Kind             string            `json:"kind"`
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	Status           int               `json:"http_status"`
	DurationMS       int64             `json:"duration_ms"`
	FirstEventMS     *int64            `json:"first_event_ms,omitempty"`
	Stream           bool              `json:"stream,omitempty"`
	Valid            *bool             `json:"valid_response,omitempty"`
	ValidationErrors []string          `json:"validation_errors,omitempty"`
	UsageIssues      []string          `json:"usage_issues,omitempty"`
	ErrorCode        string            `json:"error_code,omitempty"`
	Error            string            `json:"error,omitempty"`
	ResponseModel    string            `json:"response_model,omitempty"`
	ResponseID       string            `json:"response_id,omitempty"`
	FinishReason     string            `json:"finish_reason,omitempty"`
	Fingerprint      string            `json:"system_fingerprint,omitempty"`
	Usage            Usage             `json:"usage"`
	Headers          map[string]string `json:"headers,omitempty"`
}

type Check struct {
	ID       string         `json:"id"`
	Stage    string         `json:"stage"`
	Kind     string         `json:"kind"` // assertion or observation
	Status   string         `json:"status"`
	Code     string         `json:"code"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

type PlanItem struct {
	ID       string `json:"id"`
	Stage    string `json:"stage"`
	Kind     string `json:"kind"`
	Selected bool   `json:"selected"`
}

type RunLimits struct {
	ProbeTimeoutSeconds int `json:"probe_timeout_seconds"`
	RunTimeoutSeconds   int `json:"run_timeout_seconds"`
	MaxRequests         int `json:"max_requests"`
}

type Report struct {
	Version     int            `json:"version"`
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	Model       string         `json:"model"`
	Endpoint    string         `json:"endpoint,omitempty"`
	Options     *Options       `json:"options,omitempty"`
	Limits      *RunLimits     `json:"limits,omitempty"`
	Plan        []PlanItem     `json:"plan,omitempty"`
	StartedAt   string         `json:"started_at"`
	DurationMS  int64          `json:"duration_ms"`
	Checks      []Check        `json:"checks"`
	Samples     []Sample       `json:"samples"`
	Summary     map[string]int `json:"summary"`
	Score       *int           `json:"score,omitempty"`
	StopReason  string         `json:"stop_reason,omitempty"`
	StopProbe   string         `json:"stop_probe,omitempty"`
	Cancelled   bool           `json:"cancelled"`
	RequestsRun int            `json:"requests_run"`
	Remark      string         `json:"remark,omitempty"`
	// HistorySaved is set by the server: true when the run was stored in the
	// user's check history, false when saving failed.
	HistorySaved *bool `json:"history_saved,omitempty"`
}
