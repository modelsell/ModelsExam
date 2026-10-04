package claudecheck

import (
	"fmt"
	"math"
	"strings"

	"model-check/common"
)

const PromptAuditRequests = 6

// Counter consistency is not proof that a proxy did not modify both requests
// or both counters. No response text is retained in this evidence.
type TokenComparison struct {
	Platform       string          `json:"platform,omitempty"`
	ResponseModel  string          `json:"response_model,omitempty"`
	ClientProfile  string          `json:"client_profile,omitempty"`
	PromptProfile  string          `json:"prompt_profile,omitempty"`
	PromptChanged  bool            `json:"prompt_changed,omitempty"`
	Model          string          `json:"model,omitempty"`
	Behavior       *PromptBehavior `json:"behavior,omitempty"`
	Tolerance      *int64          `json:"tolerance,omitempty"`
	ID             string          `json:"id"`
	Expected       *int64          `json:"expected"`
	Actual         *int64          `json:"actual"`
	Difference     *int64          `json:"difference"`
	Score          *int            `json:"score"`
	Code           string          `json:"code"`
	Profile        string          `json:"profile,omitempty"`
	RequestChanged bool            `json:"request_changed,omitempty"`
	RepeatedWrite  *int64          `json:"repeated_write,omitempty"`
}

type TokenAuditReport struct {
	Injection        *PromptInjectionReport `json:"injection,omitempty"`
	PromptAssessment *PromptAssessment      `json:"prompt_assessment,omitempty"`
	CacheAssessment  *CacheAssessment       `json:"cache_assessment,omitempty"`
	Version          int                    `json:"version"`
	Prompt           []TokenComparison      `json:"prompt"`
	Cache            []TokenComparison      `json:"cache"`
}

func compareTokenCounts(item *TokenComparison) {
	if item.Expected == nil || item.Actual == nil || *item.Expected < 0 || *item.Actual < 0 {
		item.Code = "missing_usage"
		return
	}
	delta := *item.Actual - *item.Expected
	item.Difference = &delta
	score := 100
	if delta != 0 {
		hi, lo := math.Max(float64(*item.Expected), float64(*item.Actual)), math.Min(float64(*item.Expected), float64(*item.Actual))
		score = int(math.Round(100 * lo / hi))
		if score == 100 {
			score = 99
		}
	}
	item.Score = &score
	item.Code = "tokens_equal"
	if delta != 0 {
		item.Code = "tokens_differ"
	}
}

type promptAuditFixture struct {
	id   string
	body map[string]any
}

func promptAuditFixtures(model string) []promptAuditFixture {
	body := func(prompt string) map[string]any {
		return map[string]any{"model": model, "max_tokens": 128, "thinking": map[string]any{"type": "disabled"},
			"messages": []any{map[string]any{"role": "user", "content": prompt}}}
	}
	short := body("Return only the word PONG.")
	system := body("What is the verification code? Return only the code.")
	system["system"] = "The verification code is MAPLE-7391. Return it without explanation."
	var reference strings.Builder
	for i := 0; i < 96; i++ {
		fmt.Fprintf(&reference, "Record %03d: code=%06d\n", i, (i*7919+123457)%1000000)
	}
	long := body(reference.String() + "\nReturn only the six-digit code for Record 073.")
	return []promptAuditFixture{{"short", short}, {"system", system}, {"long", long}}
}

func (r *runner) emitTokenAudit() {
	r.report.TokenAudit.assessCache()
	r.report.TokenAudit.assessPrompt()
	r.assessPromptInjection()
	r.emit(Event{Type: "token_audit", TokenAudit: r.report.TokenAudit})
}

func (r *runner) promptTokenAudit() {
	if r.report.TokenAudit.Version >= 6 {
		r.simplePromptAudit()
		return
	}
	fixtures := promptAuditFixtures(r.report.Model)
	modern := r.report.TokenAudit.Version >= 2
	if modern {
		fixtures = promptIntegrityFixtures(r.report.Model)
	}
	if r.report.TokenAudit.Version >= 3 {
		fixtures = promptChannelFixtures(r.report.Model)
	}
	if r.report.TokenAudit.Version >= 4 {
		fixtures = repeatedPromptFixtures(r.report.Model)
	}
	countUnavailable := false
	for i, fixture := range fixtures {
		if r.ctx.Err() != nil {
			break
		}
		item := &r.report.TokenAudit.Prompt[i]
		m, response, ok := r.probe("prompt_audit_"+fixture.id, Request{Body: fixture.body})
		item.Profile = response.RequestProfile
		if r.report.TokenAudit.Version >= 3 {
			item.ClientProfile = bodyProfile(fixture.body)
			item.PromptProfile = response.PromptProfile
			item.PromptChanged = response.PromptProfile != "" && response.PromptProfile != promptContentProfile(fixture.body)
			item.Model = response.Model
			item.ResponseModel = m.Model
			if item.Model == "" {
				item.Model = m.Model
			}
		}
		item.RequestChanged = response.RequestProfile != "" && response.RequestProfile != bodyProfile(fixture.body)
		item.Code = "probe_unavailable"
		if modern {
			item.Behavior = evaluatePromptBehavior(fixture.id, m, ok)
		}
		if !ok {
			r.emitTokenAudit()
			if response.ErrorCode == "access_denied" || response.Status == 429 {
				break
			}
			continue
		}
		// Count immediately after inference: transports retain and count that
		// exact effective body, including configured channel overrides.
		if actual, valid := totalInput(m.Usage); valid {
			item.Actual = &actual
		}
		if countUnavailable {
			item.Code = "count_unavailable"
			r.emitTokenAudit()
			continue
		}
		_, countResponse, countOK := r.probe("prompt_audit_"+fixture.id+"_count", Request{Body: fixture.body, Count: true})
		var count struct {
			Input *int64 `json:"input_tokens"`
		}
		item.Code = "count_unavailable"
		if countOK && common.Unmarshal(countResponse.Body, &count) == nil && count.Input != nil && *count.Input >= 0 {
			item.Expected = count.Input
			if item.Profile == "" || countResponse.RequestProfile != item.Profile {
				item.Code = "count_profile_mismatch"
			} else {
				compareTokenCounts(item)
				if modern {
					comparePromptTokens(item)
				}
			}
		}
		r.emitTokenAudit()
		if r.report.Version >= 9 && ((countResponse.ErrorCode == "access_denied" && !(modern && countResponse.Status == 403)) || countResponse.Status == 429) {
			break
		}
		if r.report.Version >= 9 && ((modern && countResponse.Status == 403) || countResponse.Status == 404 || countResponse.Status == 405 || countResponse.Status == 501 || countResponse.Status == 400 && countResponse.Diagnostic != nil && countResponse.Diagnostic.Code == "count_tokens") {
			if modern {
				countUnavailable = true
				continue
			}
			for j := i + 1; j < len(r.report.TokenAudit.Prompt); j++ {
				r.report.TokenAudit.Prompt[j].Code = "count_unavailable"
			}
			r.emitTokenAudit()
			break
		}
	}
	code := "token_audit_observed"
	if modern {
		code = "prompt_integrity_assessed"
	}
	r.check("prompt_integrity", "inconclusive", code, map[string]any{"count_source": "same_endpoint", "injection_verified": false})
	if r.report.TokenAudit.Version >= 4 {
		r.emitTokenAudit()
	}
}

func (r *runner) compareCacheTokens(index int, first Sample, current Sample) {
	item := &r.report.TokenAudit.Cache[index]
	item.Profile, item.RepeatedWrite = current.RequestProfile, current.Usage.CacheWrite
	item.Expected, item.Actual = first.Usage.CacheWrite, current.Usage.CacheRead
	item.Code = "cache_unconfirmed"
	if first.Valid == nil || !*first.Valid || current.Valid == nil || !*current.Valid {
		item.Code = "probe_unavailable"
	} else if first.RequestProfile == "" || current.RequestProfile != first.RequestProfile {
		item.Code = "count_profile_mismatch"
	} else if first.Usage.CacheRead == nil || *first.Usage.CacheRead != 0 || item.Expected == nil || *item.Expected <= 0 {
		item.Code = "cache_write_reference_missing"
	} else {
		compareTokenCounts(item)
		if r.report.TokenAudit.Version >= 4 {
			// Rewrites must not receive full credit even when read counters match.
			if item.RepeatedWrite == nil || *item.RepeatedWrite < 0 {
				item.Score, item.Code = nil, "missing_usage"
			} else if item.Score != nil && *item.RepeatedWrite > 0 {
				score := int(math.Round(float64(*item.Score) * float64(*item.Expected) / float64(*item.Expected+*item.RepeatedWrite)))
				item.Score, item.Code = &score, "cache_rewritten"
			}
		}
	}
	r.emitTokenAudit()
}

func IsCountProbe(id string) bool {
	return id == "token_count" || (strings.HasPrefix(id, "prompt_audit_") && strings.HasSuffix(id, "_count"))
}
