// Package geminicheck runs bounded, synthetic probes against the native Gemini
// API (generativelanguage v1beta: models.get, generateContent,
// streamGenerateContent and countTokens).
//
// Like openaicheck it checks both sides of the contract: which documented
// request fields an endpoint accepts (systemInstruction, generationConfig,
// tools/toolConfig, responseSchema, inlineData) and whether the returned
// candidates, parts, finishReason, usageMetadata, SSE chunks and error bodies
// match the official shapes. Results describe API behavior; they never prove
// that the upstream is the genuine Google model named in the request.
//
// Reports share the JSON shape of openaicheck reports (provider "gemini"), so
// the same history routes and report views can show them.
package geminicheck

import (
	"context"
	"net/http"
	"time"

	"model-check/pkg/openaicheck"
)

const (
	MaxResponseBytes = openaicheck.MaxResponseBytes
	ProbeTimeout     = 90 * time.Second
	RunTimeout       = 10 * time.Minute
	ReportVersion    = 1

	// APIVersion is the path prefix of every probe. v1beta is the version the
	// official SDKs use; systemInstruction, tools and responseSchema need it.
	APIVersion = "v1beta"

	// Probe kinds select which response contract a sample is validated against.
	KindModel    = "model"
	KindGenerate = "generate"
	KindCount    = "count_tokens"
	KindError    = "error"
)

type Options struct {
	Model string `json:"model"`
	// Suite is "basic", "standard" (default) or "full".
	Suite string `json:"suite,omitempty"`
	// Vision adds an inlineData image probe.
	Vision bool `json:"vision,omitempty"`
}

const (
	SuiteBasic    = "basic"
	SuiteStandard = "standard"
	SuiteFull     = "full"
)

// ValidOptions rejects unknown enum values and model names that could escape
// the models/{model} path segment, before any upstream request.
func ValidOptions(o Options) bool {
	switch o.Suite {
	case "", SuiteBasic, SuiteStandard, SuiteFull:
	default:
		return false
	}
	return ValidModel(o.Model)
}

// ValidModel accepts a bare model id or "models/<id>" made of the characters
// Gemini model ids use.
func ValidModel(model string) bool {
	id := ModelID(model)
	if id == "" || len(model) > 200 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
			return false
		}
	}
	return id != "." && id != ".."
}

func (o Options) normalized() Options {
	if o.Suite == "" {
		o.Suite = SuiteStandard
	}
	o.Model = ModelID(o.Model)
	return o
}

func (o Options) full() bool     { return o.Suite == SuiteFull }
func (o Options) standard() bool { return o.Suite != SuiteBasic }

// Request is generated locally; callers cannot supply arbitrary probe payloads.
type Request struct {
	Method string
	Path   string // e.g. /v1beta/models/x:generateContent; the transport prefixes the base URL
	Body   map[string]any
	Stream bool // the path carries ?alt=sse and the reply is read as SSE
}

type SSEEvent = openaicheck.SSEEvent

type Response struct {
	Status       int
	Header       http.Header
	Body         []byte
	Events       []SSEEvent
	Done         bool // an OpenAI-style "data: [DONE]" frame arrived (Gemini never sends one)
	FirstEventMS *int64
	ErrorCode    string
}

type Transport func(context.Context, Request) (Response, error)

// Event is the progress stream. Request bodies and credentials never enter it.
type Event struct {
	Type     string  `json:"type"`
	Probe    string  `json:"probe,omitempty"`
	Report   *Report `json:"report,omitempty"`
	Sample   *Sample `json:"sample,omitempty"`
	Check    *Check  `json:"check,omitempty"`
	Markdown string  `json:"markdown,omitempty"`
}

// Usage maps usageMetadata onto the field names shared with openaicheck:
// input is promptTokenCount, output is candidatesTokenCount plus
// thoughtsTokenCount, total is totalTokenCount.
type Usage struct {
	Input    *int64 `json:"input_tokens"`
	Output   *int64 `json:"output_tokens"`
	Total    *int64 `json:"total_tokens"`
	Thoughts *int64 `json:"thoughts_tokens,omitempty"`
	Cached   *int64 `json:"cached_tokens,omitempty"`
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
	ResponseModel    string            `json:"response_model,omitempty"` // modelVersion
	ResponseID       string            `json:"response_id,omitempty"`    // responseId
	FinishReason     string            `json:"finish_reason,omitempty"`
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
	Version      int            `json:"version"`
	ID           string         `json:"id"`
	Provider     string         `json:"provider"`
	Model        string         `json:"model"`
	Endpoint     string         `json:"endpoint,omitempty"`
	Options      *Options       `json:"options,omitempty"`
	Limits       *RunLimits     `json:"limits,omitempty"`
	Plan         []PlanItem     `json:"plan,omitempty"`
	StartedAt    string         `json:"started_at"`
	DurationMS   int64          `json:"duration_ms"`
	Checks       []Check        `json:"checks"`
	Samples      []Sample       `json:"samples"`
	Summary      map[string]int `json:"summary"`
	Score        *int           `json:"score,omitempty"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopProbe    string         `json:"stop_probe,omitempty"`
	Cancelled    bool           `json:"cancelled"`
	RequestsRun  int            `json:"requests_run"`
	Remark       string         `json:"remark,omitempty"`
	HistorySaved *bool          `json:"history_saved,omitempty"`
}
