// Package imagecheck runs bounded, synthetic probes against an OpenAI-style
// image API (/v1/images/generations and /v1/images/edits).
//
// Every content check is decided by arithmetic over the returned pixels, and
// prompts carry per-run random parameters so an upstream cannot recognise a
// probe and route it differently. An optional provenance stage uploads the
// original bytes to OpenAI's content provenance check. Results are
// observations of API behavior, never proof of the model behind an endpoint.
package imagecheck

import (
	"context"
	"net/http"
	"time"

	"model-check/pkg/openaicheck"
	"model-check/pkg/provenance"
)

const (
	ReportVersion    = 1
	Provider         = "openai-image"
	ProbeTimeout     = 180 * time.Second
	RunTimeout       = 15 * time.Minute
	MaxResponseBytes = 48 << 20
	MaxRequestBytes  = 12 << 20
	MaxImageBytes    = 20 << 20
	maxThumbs        = 24
)

// Check kinds. "provenance" results are independent evidence: they are
// reported beside the score and never move it.
const (
	KindAssertion   = "assertion"
	KindObservation = "observation"
	KindProvenance  = "provenance"
)

// Shared with the text engines so reports and the UI use one vocabulary.
type (
	Check    = openaicheck.Check
	Sample   = openaicheck.Sample
	PlanItem = openaicheck.PlanItem
	Usage    = openaicheck.Usage
)

// Options never carries a credential: the whole struct is stored in the report.
type Options struct {
	Model string `json:"model"`
	// Suite is "basic", "standard" (default) or "full".
	Suite string `json:"suite,omitempty"`
	// Provenance uploads generated images to OpenAI's provenance check. The
	// server enables it only when an official OpenAI key was supplied.
	Provenance bool `json:"provenance,omitempty"`
	// Baseline additionally generates one image directly from OpenAI with the
	// same request and compares provenance. Implies Provenance.
	Baseline bool `json:"baseline,omitempty"`
	// Profile overrides the capability profile guessed from the model name.
	Profile *Profile `json:"profile,omitempty"`
}

const (
	SuiteBasic    = "basic"
	SuiteStandard = "standard"
	SuiteFull     = "full"
)

// Profile declares what the model is supposed to support. A parameter outside
// the profile is never probed, so an old model is not failed for a feature it
// was never meant to have.
type Profile struct {
	Family      string   `json:"family"`
	FixedSizes  []string `json:"fixed_sizes,omitempty"`
	CustomSizes bool     `json:"custom_sizes,omitempty"`
	Qualities   []string `json:"qualities"`
	Formats     []string `json:"formats"`
	Transparent bool     `json:"transparent,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	Edits       bool     `json:"edits,omitempty"`
	Mask        bool     `json:"mask,omitempty"`
	NMax        int      `json:"n_max"`
}

// Part is one multipart file.
type Part struct {
	Field, Filename, Mime string
	Data                  []byte
}

// Request is generated locally; callers cannot supply arbitrary payloads.
type Request struct {
	Method string
	Path   string // /v1/images/generations or /v1/images/edits
	Body   map[string]any
	Fields map[string]string // multipart text fields (edits)
	Files  []Part            // multipart files (edits)
	Stream bool
	// ExpectError marks a probe whose correct answer is a 4xx: it is not
	// recorded as an invalid response.
	ExpectError bool
}

type Response struct {
	Status       int
	Header       http.Header
	Body         []byte
	Events       []openaicheck.SSEEvent
	FirstEventMS *int64
	ErrorCode    string
}

type Transport func(context.Context, Request) (Response, error)

// Verifier uploads one image to the provenance check.
type Verifier func(ctx context.Context, data []byte, mime string) (*provenance.Result, error)

// Fetcher downloads an upstream-returned image URL. The server supplies one
// that runs through the SSRF client and never forwards credentials.
type Fetcher func(ctx context.Context, url string) ([]byte, error)

// Deps are the optional collaborators of a run.
type Deps struct {
	Verify   Verifier
	Fetch    Fetcher
	Baseline Transport // direct-to-OpenAI transport for the baseline comparison
}

type Event struct {
	Type     string  `json:"type"`
	Probe    string  `json:"probe,omitempty"`
	Report   *Report `json:"report,omitempty"`
	Sample   *Sample `json:"sample,omitempty"`
	Check    *Check  `json:"check,omitempty"`
	Markdown string  `json:"markdown,omitempty"`
}

// ImageRef describes one image the run received. Thumb (a small JPEG data URL)
// is for live display only; the server strips it before anything is stored.
type ImageRef struct {
	Probe  string `json:"probe"`
	Index  int    `json:"index"`
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
	Thumb  string `json:"thumb,omitempty"`
}

type Limits struct {
	ProbeTimeoutSeconds int `json:"probe_timeout_seconds"`
	RunTimeoutSeconds   int `json:"run_timeout_seconds"`
	MaxRequests         int `json:"max_requests"`
	MaxImages           int `json:"max_images"`
	MaxVerifyCalls      int `json:"max_verify_calls"`
}

// BaselineSummary compares the endpoint under test with a direct OpenAI call.
type BaselineSummary struct {
	Level   string              `json:"level"`
	Verdict *provenance.Verdict `json:"verdict,omitempty"`
}

// ProvenanceSummary is the headline of the provenance stage.
type ProvenanceSummary struct {
	Enabled bool `json:"enabled"`
	// Level is trusted | synthid | untrusted | none | unavailable | off.
	Level           string              `json:"level"`
	UnavailableCode string              `json:"unavailable_code,omitempty"`
	ControlOK       *bool               `json:"control_ok,omitempty"`
	Verdict         *provenance.Verdict `json:"verdict,omitempty"`
	Baseline        *BaselineSummary    `json:"baseline,omitempty"`
	Formats         map[string]string   `json:"formats,omitempty"`
	VerifyCalls     int                 `json:"verify_calls"`
}

type Report struct {
	Version      int                `json:"version"`
	ID           string             `json:"id"`
	Provider     string             `json:"provider"`
	Model        string             `json:"model"`
	Endpoint     string             `json:"endpoint,omitempty"`
	Options      *Options           `json:"options,omitempty"`
	Profile      *Profile           `json:"profile,omitempty"`
	Limits       *Limits            `json:"limits,omitempty"`
	Plan         []PlanItem         `json:"plan,omitempty"`
	StartedAt    string             `json:"started_at"`
	DurationMS   int64              `json:"duration_ms"`
	Checks       []Check            `json:"checks"`
	Samples      []Sample           `json:"samples"`
	Images       []ImageRef         `json:"images"`
	Summary      map[string]int     `json:"summary"`
	Score        *int               `json:"score,omitempty"`
	Provenance   *ProvenanceSummary `json:"provenance,omitempty"`
	StopReason   string             `json:"stop_reason,omitempty"`
	StopProbe    string             `json:"stop_probe,omitempty"`
	Cancelled    bool               `json:"cancelled"`
	RequestsRun  int                `json:"requests_run"`
	ImagesBilled int                `json:"images_requested"`
	Remark       string             `json:"remark,omitempty"`
	HistorySaved *bool              `json:"history_saved,omitempty"`
}
