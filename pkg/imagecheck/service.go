package imagecheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"model-check/pkg/openaicheck"
	"model-check/pkg/provenance"
)

// Input is the request body of an image check. Key goes only to the endpoint
// under test; VerifyKey is an official OpenAI key that goes only to the
// official host. Neither is ever stored: Options is the persisted part.
type Input struct {
	Options
	BaseURL   string `json:"base_url"`
	Key       string `json:"key"`
	VerifyKey string `json:"verify_key"`
	Remark    string `json:"remark"`
}

func validKey(k string) bool {
	return len(k) >= 4 && len(k) <= 8192 && !strings.ContainsAny(k, "\r\n")
}

// Normalize trims and validates the input, returning the normalized endpoint.
// The error text is safe to show to the caller.
func (in *Input) Normalize() (endpoint string, err error) {
	in.Model, in.Key, in.VerifyKey = strings.TrimSpace(in.Model), strings.TrimSpace(in.Key), strings.TrimSpace(in.VerifyKey)
	endpoint, urlErr := openaicheck.NormalizeBaseURL(in.BaseURL)
	if urlErr != nil || !validKey(in.Key) {
		return "", errors.New("Enter a valid Base URL, API key, and model")
	}
	if !ValidOptions(in.Options) {
		return "", errors.New("Invalid model check options")
	}
	in.Remark = strings.TrimSpace(in.Remark)
	if !utf8.ValidString(in.Remark) || utf8.RuneCountInString(in.Remark) > 200 {
		return "", errors.New("Remark must contain at most 200 characters")
	}
	if in.VerifyKey != "" && !validKey(in.VerifyKey) {
		return "", errors.New("Enter a valid official OpenAI API key")
	}
	if (in.Provenance || in.Baseline) && in.VerifyKey == "" {
		return "", errors.New("Enter an official OpenAI API key to use provenance verification")
	}
	in.Provenance = in.Provenance || in.Baseline
	if in.VerifyKey == "" {
		in.Provenance, in.Baseline = false, false
	}
	return endpoint, nil
}

// Plumbing wires a run to the network.
type Plumbing struct {
	Client       *http.Client                        // applies the SSRF policy at dial time
	ValidateURL  func(context.Context, string) error // preflight for URLs the upstream hands back
	Endpoint     string                              // normalized endpoint under test
	OfficialBase string                              // origin of the official OpenAI API; empty means the real one
	Input        Input                               // already normalized
	Redactor     *openaicheck.Redactor               // must know both keys
}

// maxRedirects bounds the hops taken to fetch an image URL.
const maxRedirects = 3

// Build returns the transport for the endpoint under test and the optional
// collaborators of a run.
func Build(p Plumbing) (Transport, Deps) {
	in := p.Input
	official := p.OfficialBase
	if official == "" {
		official = provenance.DefaultBaseURL
	}
	transport := NewHTTPTransport(p.Client, p.Endpoint, in.Key, p.Redactor)
	deps := Deps{Fetch: p.fetcher()}
	if in.Provenance && in.VerifyKey != "" {
		pc := provenance.NewClient(p.Client, in.VerifyKey)
		pc.BaseURL = official
		deps.Verify = func(ctx context.Context, data []byte, mime string) (*provenance.Result, error) {
			res, err := pc.Check(ctx, data, mime)
			if err != nil && p.Redactor != nil {
				var api *provenance.APIError
				if !errors.As(err, &api) {
					err = errors.New(p.Redactor.String(err.Error()))
				}
			}
			return res, err
		}
	}
	if in.Baseline && in.VerifyKey != "" {
		deps.Baseline = NewHTTPTransport(p.Client, official, in.VerifyKey, p.Redactor)
	}
	return transport, deps
}

// fetcher downloads an image URL returned by the upstream. It sends no
// credentials, follows at most maxRedirects hops, and re-validates every hop.
func (p Plumbing) fetcher() Fetcher {
	return func(ctx context.Context, raw string) ([]byte, error) {
		current := raw
		for hop := 0; hop <= maxRedirects; hop++ {
			u, err := url.Parse(current)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
				return nil, errors.New("invalid image URL")
			}
			if p.ValidateURL != nil {
				if err := p.ValidateURL(ctx, current); err != nil {
					return nil, errors.New("image URL blocked by the outbound policy")
				}
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
			if err != nil {
				return nil, err
			}
			resp, err := p.Client.Do(req)
			if err != nil {
				return nil, errors.New("image download failed")
			}
			switch {
			case resp.StatusCode >= 300 && resp.StatusCode < 400:
				loc, perr := u.Parse(resp.Header.Get("Location"))
				resp.Body.Close()
				if perr != nil || resp.Header.Get("Location") == "" {
					return nil, errors.New("image redirect without a valid location")
				}
				current = loc.String()
				continue
			case resp.StatusCode != http.StatusOK:
				resp.Body.Close()
				return nil, errors.New("image download returned a non-200 status")
			}
			data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
			resp.Body.Close()
			if err != nil {
				return nil, errors.New("image download failed")
			}
			if len(data) > MaxImageBytes {
				return nil, errors.New("image exceeds size limit")
			}
			return data, nil
		}
		return nil, errors.New("too many image redirects")
	}
}

// Snapshot is the form of a report that may be stored or published: no
// thumbnails, and a summary and score recomputed from the checks so far.
func Snapshot(r Report) Report {
	out := r
	out.Images = append([]ImageRef(nil), r.Images...)
	StripThumbs(&out)
	out.RequestsRun = len(out.Samples)
	out.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, c := range out.Checks {
		out.Summary[c.Status]++
	}
	out.Score = Score(out)
	return out
}
