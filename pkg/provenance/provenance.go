// Package provenance is a client for OpenAI's content provenance check
// (POST /v1/content_provenance_checks), which reports whether an uploaded
// image carries OpenAI's C2PA Content Credentials or SynthID watermark.
//
// A missing signal never proves an image is not from OpenAI, and a present
// signal proves only that OpenAI tooling produced some image; callers must
// treat the verdict as evidence, not as an identity or billing proof.
package provenance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"time"

	"model-check/common"
)

const (
	// DefaultBaseURL is the only host a production deployment talks to.
	DefaultBaseURL = "https://api.openai.com"
	Path           = "/v1/content_provenance_checks"
	MaxFileBytes   = 50 << 20 // documented per-file limit
	maxResponse    = 1 << 20
	Timeout        = 60 * time.Second
)

// C2PA is one c2pa entry of a check response.
type C2PA struct {
	Outcome         string `json:"outcome"`          // detected | not_detected
	ValidationState string `json:"validation_state"` // trusted | valid | invalid | not_present
	Issuer          string `json:"issuer,omitempty"`
	Model           string `json:"model,omitempty"`
	GeneratedAt     string `json:"generated_at,omitempty"`
}

// SynthID is the watermark entry.
type SynthID struct {
	Outcome string `json:"outcome"` // detected | not_detected
}

// Result is a parsed check response. A signal the response does not mention
// stays nil rather than being assumed absent.
type Result struct {
	CreatedAt int64    `json:"created_at,omitempty"`
	C2PA      *C2PA    `json:"c2pa,omitempty"`
	SynthID   *SynthID `json:"synthid,omitempty"`
}

// APIError is a non-2xx answer from the verification service.
type APIError struct {
	Status     int
	Code       string
	Message    string
	RetryAfter string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("content provenance check failed with HTTP %d", e.Status)
	if e.Code != "" {
		msg += " (" + e.Code + ")"
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// Kind maps the status onto the failure vocabulary the reports use.
func (e *APIError) Kind() string {
	switch {
	case e.Status == http.StatusBadRequest:
		return "rejected"
	case e.Status == http.StatusUnauthorized:
		return "unauthorized"
	case e.Status == http.StatusForbidden, e.Status == http.StatusNotFound:
		return "no_access" // 404 means the organization lacks access
	case e.Status == http.StatusTooManyRequests:
		return "rate_limited"
	case e.Status >= 500:
		return "upstream_error"
	}
	return "request_rejected"
}

// Client uploads one file per call. HTTP decides network policy (use the SSRF
// client); redirects are never followed so the key cannot be replayed.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Key     string
}

func NewClient(h *http.Client, key string) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL, Key: key}
}

var allowedMIME = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// Check uploads data (an image of the given media type) and parses the result.
func (c *Client) Check(ctx context.Context, data []byte, mime string) (*Result, error) {
	if c.HTTP == nil || c.Key == "" {
		return nil, errors.New("provenance client is not configured")
	}
	if !allowedMIME[mime] {
		return nil, errors.New("unsupported media type for provenance check")
	}
	if len(data) == 0 || len(data) > MaxFileBytes {
		return nil, errors.New("file size is outside the provenance check limit")
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" {
		return nil, errors.New("invalid provenance base URL")
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="image`+extFor(mime)+`"`)
	hdr.Set("Content-Type", mime)
	part, err := w.CreatePart(hdr)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(data); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+Path, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	client := *c.HTTP
	client.Timeout = Timeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New(strings.ReplaceAll(err.Error(), c.Key, "[REDACTED]"))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponse {
		return nil, errors.New("provenance response exceeds size limit")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp, raw)
	}
	return parse(raw)
}

func extFor(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	}
	return ".png"
}

func apiError(resp *http.Response, raw []byte) *APIError {
	e := &APIError{Status: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	var doc struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if common.Unmarshal(raw, &doc) == nil {
		if s, ok := doc.Error.Code.(string); ok {
			e.Code = s
		}
		e.Message = truncate(doc.Error.Message, 240)
	}
	return e
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func parse(raw []byte) (*Result, error) {
	var doc struct {
		Object    string `json:"object"`
		CreatedAt int64  `json:"created_at"`
		Results   []struct {
			Type            string  `json:"type"`
			Outcome         string  `json:"outcome"`
			ValidationState string  `json:"validation_state"`
			Issuer          *string `json:"issuer"`
			Model           *string `json:"model"`
			GeneratedAt     *string `json:"generated_at"`
		} `json:"results"`
	}
	if err := common.Unmarshal(raw, &doc); err != nil {
		return nil, errors.New("provenance response is not valid JSON")
	}
	if doc.Object != "content_provenance_check" || doc.Results == nil {
		return nil, errors.New("provenance response has an unexpected shape")
	}
	res := &Result{CreatedAt: doc.CreatedAt}
	for _, r := range doc.Results {
		if r.Outcome != "detected" && r.Outcome != "not_detected" {
			return nil, errors.New("provenance response has an unknown outcome")
		}
		switch r.Type {
		case "c2pa":
			res.C2PA = &C2PA{Outcome: r.Outcome, ValidationState: r.ValidationState, Issuer: deref(r.Issuer), Model: deref(r.Model), GeneratedAt: deref(r.GeneratedAt)}
		case "synthid":
			res.SynthID = &SynthID{Outcome: r.Outcome}
		}
	}
	return res, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Verdict levels, strongest evidence first.
const (
	LevelTrusted   = "trusted"   // trusted C2PA credential from OpenAI
	LevelSynthID   = "synthid"   // watermark found; credentials absent or untrusted
	LevelUntrusted = "untrusted" // credential present but not trusted
	LevelNone      = "none"      // no OpenAI signal detected (proves nothing)
)

// Verdict is the interpretation of one Result.
type Verdict struct {
	Level       string `json:"level"`
	C2PAState   string `json:"c2pa_state"` // trusted | valid | invalid | not_present | unknown
	Issuer      string `json:"issuer,omitempty"`
	Model       string `json:"model,omitempty"`
	GeneratedAt string `json:"generated_at,omitempty"`
	SynthID     bool   `json:"synthid"`
	// ModelMatch is nil unless a credential named a model.
	ModelMatch *bool `json:"model_match,omitempty"`
}

// Classify applies the interpretation rules. Only a trusted credential issued
// by OpenAI (or with no issuer string) counts as trusted.
func Classify(r *Result, requestedModel string) Verdict {
	v := Verdict{Level: LevelNone, C2PAState: "unknown"}
	if r == nil {
		return v
	}
	if r.SynthID != nil && r.SynthID.Outcome == "detected" {
		v.SynthID = true
	}
	detected := false
	if c := r.C2PA; c != nil {
		v.Issuer, v.Model, v.GeneratedAt = c.Issuer, c.Model, c.GeneratedAt
		detected = c.Outcome == "detected"
		v.C2PAState = c.ValidationState
		if v.C2PAState == "" {
			v.C2PAState = "unknown"
		}
		if v.Model != "" {
			m := ModelsMatch(requestedModel, v.Model)
			v.ModelMatch = &m
		}
	}
	trusted := detected && v.C2PAState == "trusted" && (v.Issuer == "" || strings.Contains(strings.ToLower(v.Issuer), "openai"))
	switch {
	case trusted:
		v.Level = LevelTrusted
	case v.SynthID:
		v.Level = LevelSynthID
	case detected:
		v.Level = LevelUntrusted
	}
	return v
}

var snapshotSuffix = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{8})$`)

// NormalizeModel lowercases a model name and drops a provider prefix and a
// trailing date snapshot, so "openai/GPT-Image-2-2026-04-21" matches
// "gpt-image-2". Variants such as "-mini" are kept: they are different models.
func NormalizeModel(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndex(n, "/"); i >= 0 {
		n = n[i+1:]
	}
	return snapshotSuffix.ReplaceAllString(n, "")
}

func ModelsMatch(requested, claimed string) bool {
	a, b := NormalizeModel(requested), NormalizeModel(claimed)
	return a != "" && a == b
}
