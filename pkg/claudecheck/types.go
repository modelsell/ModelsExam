// Package claudecheck runs bounded, synthetic Claude compatibility probes.
// Fingerprints and reported usage are observations, never proof of authenticity.
package claudecheck

import (
	"context"
	"net/http"
	"time"
)

const MaxResponseBytes = 2 << 20
const ProbeTimeout = 90 * time.Second
const RunTimeout = 10 * time.Minute

type Options struct {
	Suite                string   `json:"suite,omitempty"`
	PerformanceTolerance *float64 `json:"performance_tolerance,omitempty"`
	Model                string   `json:"model"`
	Cache                bool     `json:"cache"`
	Thinking             bool     `json:"thinking"`
	Repeat               bool     `json:"repeat"`
	Vision               bool     `json:"vision"`
	PDF                  bool     `json:"pdf"`
	StreamComparison     bool     `json:"stream_comparison"`
	Fingerprint          bool     `json:"fingerprint,omitempty"`
	Benchmark            bool     `json:"benchmark,omitempty"`
	Performance          bool     `json:"performance,omitempty"`
	PromptAudit          bool     `json:"prompt_audit,omitempty"`
	CompareBaselines     bool     `json:"compare_baselines,omitempty"`
	BaselineID           string   `json:"baseline_id,omitempty"`
	BaselineType         string   `json:"baseline_type,omitempty"`
	Bedrock              bool     `json:"bedrock,omitempty"`
	// LegacySuite accepts saved clients; it is normalized before creating a new report.
	LegacySuite bool `json:"veridrop,omitempty"`
}

func (o Options) normalized() Options {
	// The retired preference switch remains readable in saved reports, but cannot
	// schedule samples in a new run, including requests from old clients.
	o.Fingerprint = false
	if o.Suite == "focused" {
		o.Thinking, o.Repeat, o.StreamComparison, o.LegacySuite = false, false, false, false
		o.Benchmark, o.Performance = true, true
		if o.PerformanceTolerance == nil {
			value := float64(25)
			o.PerformanceTolerance = &value
		}
		return o
	}
	if o.LegacySuite {
		o.PDF, o.StreamComparison, o.LegacySuite = true, true, false
	}
	return o
}

// Request is generated locally; callers cannot supply arbitrary probe payloads.
type Request struct {
	Body  map[string]any
	Count bool
	Beta  string
}

type Response struct {
	PromptProfile      string
	Bedrock            bool
	Diagnostic         *BedrockDiagnostic
	RequestProfile     string
	ErrorCode          string
	Status             int
	Header             http.Header
	Body               []byte
	Events             [][]byte
	FirstEventMS       *int64
	Stream             *StreamMetrics
	Model              string
	EffectiveMaxTokens *int64
}

type Transport func(context.Context, Request) (Response, error)

// Event contains only report evidence; request bodies and credentials never
// enter the progress stream. Observers run synchronously on the runner goroutine.
type Event struct {
	TokenAudit  *TokenAuditReport    `json:"token_audit,omitempty"`
	Benchmark   *CapabilityBenchmark `json:"benchmark,omitempty"`
	Fingerprint *BehaviorFingerprint `json:"fingerprint,omitempty"`
	Type        string               `json:"type"`
	Probe       string               `json:"probe,omitempty"`
	Report      *Report              `json:"report,omitempty"`
	Sample      *Sample              `json:"sample,omitempty"`
	Check       *Check               `json:"check,omitempty"`
}

type Usage struct {
	Input      *int64 `json:"input_tokens"`
	Output     *int64 `json:"output_tokens"`
	CacheWrite *int64 `json:"cache_creation_input_tokens"`
	CacheRead  *int64 `json:"cache_read_input_tokens"`
}

type Sample struct {
	Diagnostic         *BedrockDiagnostic `json:"diagnostic,omitempty"`
	RequestProfile     string             `json:"request_profile,omitempty"`
	ErrorCode          string             `json:"error_code,omitempty"`
	Probe              string             `json:"probe"`
	Status             int                `json:"http_status"`
	DurationMS         int64              `json:"duration_ms"`
	FirstEventMS       *int64             `json:"first_event_ms,omitempty"`
	Stream             *StreamMetrics     `json:"stream,omitempty"`
	Valid              *bool              `json:"valid_response,omitempty"`
	RequestedMaxTokens *int64             `json:"requested_max_tokens,omitempty"`
	EffectiveMaxTokens *int64             `json:"effective_max_tokens,omitempty"`
	ContentBlocks      *int               `json:"content_blocks,omitempty"`
	ValidationErrors   []string           `json:"validation_errors,omitempty"`
	UpstreamModel      string             `json:"upstream_model,omitempty"`
	ResponseModel      string             `json:"response_model,omitempty"`
	MessageID          string             `json:"message_id,omitempty"`
	RequestID          string             `json:"request_id,omitempty"`
	StopReason         string             `json:"stop_reason,omitempty"`
	Usage              Usage              `json:"usage"`
	Headers            map[string]string  `json:"headers,omitempty"`
	Error              string             `json:"error,omitempty"`
}

type Check struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Code     string         `json:"code"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

type Report struct {
	TokenAudit    *TokenAuditReport    `json:"token_audit,omitempty"`
	Benchmark     *CapabilityBenchmark `json:"benchmark,omitempty"`
	Baselines     []ComparisonBaseline `json:"baselines,omitempty"`
	Fingerprint   *BehaviorFingerprint `json:"fingerprint,omitempty"`
	StopReason    string               `json:"stop_reason,omitempty"`
	StopProbe     string               `json:"stop_probe,omitempty"`
	BaselineProbe string               `json:"baseline_probe,omitempty"`
	Limits        *RunLimits           `json:"limits,omitempty"`
	Reference     *BaselineReference   `json:"reference,omitempty"`
	Version       int                  `json:"version"`
	ID            string               `json:"id"`
	Model         string               `json:"model"`
	ChannelID     int                  `json:"channel_id"`
	ChannelName   string               `json:"channel_name"`
	Remark        *string              `json:"remark,omitempty"`
	Transport     string               `json:"transport"`
	Endpoint      string               `json:"endpoint,omitempty"`
	Options       *Options             `json:"options,omitempty"`
	Plan          []PlanItem           `json:"plan,omitempty"`
	HistorySaved  *bool                `json:"history_saved,omitempty"`
	StartedAt     string               `json:"started_at"`
	DurationMS    int64                `json:"duration_ms"`
	Checks        []Check              `json:"checks"`
	Samples       []Sample             `json:"samples"`
	Summary       map[string]int       `json:"summary"`
	Cancelled     bool                 `json:"cancelled"`
}

type RunLimits struct {
	ProbeTimeoutSeconds int `json:"probe_timeout_seconds"`
	RunTimeoutSeconds   int `json:"run_timeout_seconds"`
	MaxRequests         int `json:"max_requests"`
}
