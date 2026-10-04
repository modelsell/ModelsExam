package claudecheck

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

const CacheRounds = 2
const CacheReadsPerRound = 3
const RepeatedCacheRequests = CacheRounds * (1 + CacheReadsPerRound)

type CacheAssessment struct {
	Score    *int `json:"score"`
	Measured int  `json:"measured"`
	Planned  int  `json:"planned"`
	Coverage int  `json:"coverage"`
	Rounds   int  `json:"rounds"`
}

func repeatedCacheReadIDs() []string {
	var ids []string
	for i := 1; i <= CacheRounds*CacheReadsPerRound; i++ {
		ids = append(ids, fmt.Sprintf("cache_read_%d", i))
	}
	return ids
}

func (audit *TokenAuditReport) assessCache() {
	if audit.Version < 4 || len(audit.Cache) == 0 {
		return
	}
	a := &CacheAssessment{Planned: CacheRounds * CacheReadsPerRound, Rounds: CacheRounds}
	sum, pending := 0, false
	for _, id := range repeatedCacheReadIDs() {
		item := promptItem(audit.Cache, id)
		if item == nil || item.Code == "pending" {
			pending = true
			continue
		}
		if item.Score != nil && *item.Score >= 0 && *item.Score <= 100 {
			a.Measured++
			sum += *item.Score
		}
	}
	a.Coverage = int(math.Round(float64(a.Measured) * 100 / float64(a.Planned)))
	if !pending && a.Measured > 0 {
		score := int(math.Round(float64(sum) / float64(a.Planned)))
		a.Score = &score
	}
	audit.CacheAssessment = a
}

func (r *runner) repeatedCache() {
	defer func() {
		r.check("cache_token_audit", "inconclusive", "cache_token_audit_observed", nil)
		r.emitTokenAudit()
	}()
	var written, read, total, repeatedWrite int64
	var requests, warm, hits, freshWrites int
	for round := 0; round < CacheRounds; round++ {
		var prefix strings.Builder
		// A distinct nonce starts each round cold; every read reuses its exact
		// write body, including generation parameters and the cache breakpoint.
		fmt.Fprintf(&prefix, "Synthetic cache test %s. Read this reference and answer the question below.\n", uuid.NewString())
		for i := 0; i < 384; i++ {
			fmt.Fprintf(&prefix, "Record %d: amber birch cedar delta elm frost green harbor iris jade kite lemon maple north oak pine.\n", i)
		}
		body := r.body("Return only the first word in Record 73.")
		body["system"] = []any{map[string]any{"type": "text", "text": prefix.String(), "cache_control": map[string]any{"type": "ephemeral"}}}
		var first Sample
		for step := 0; step <= CacheReadsPerRound; step++ {
			if r.ctx.Err() != nil || r.focusedStop() {
				return
			}
			id := fmt.Sprintf("cache_read_%d", round*CacheReadsPerRound+step)
			if step == 0 {
				id = "cache_write"
				if round > 0 {
					id = fmt.Sprintf("cache_write_%d", round+1)
				}
			}
			before := len(r.report.Samples)
			_, _, ok := r.probe(id, Request{Body: body})
			if len(r.report.Samples) == before {
				return
			}
			sample := r.report.Samples[len(r.report.Samples)-1]
			requests++
			if step == 0 {
				first = sample
				if ok && sample.Usage.CacheRead != nil && *sample.Usage.CacheRead == 0 && sample.Usage.CacheWrite != nil && *sample.Usage.CacheWrite > 0 {
					freshWrites++
				}
			} else {
				warm++
				r.compareCacheTokens(round*CacheReadsPerRound+step-1, first, sample)
				if ok && sample.Usage.CacheRead != nil && *sample.Usage.CacheRead > 0 {
					hits++
				}
				if ok && sample.Usage.CacheWrite != nil {
					repeatedWrite += *sample.Usage.CacheWrite
				}
			}
			if n, valid := totalInput(sample.Usage); ok && valid {
				total += n
				if sample.Usage.CacheRead != nil {
					read += *sample.Usage.CacheRead
				}
				if sample.Usage.CacheWrite != nil {
					written += *sample.Usage.CacheWrite
				}
			}
			// Transient errors do not discard the other round's evidence.
		}
	}
	state, code := "inconclusive", "cache_unconfirmed"
	if freshWrites == CacheRounds && hits == CacheRounds*CacheReadsPerRound {
		state, code = "pass", "cache_observed"
	}
	rate := float64(0)
	if total > 0 {
		rate = float64(read) / float64(total)
	}
	r.check("cache", state, code, map[string]any{"rounds": CacheRounds, "fresh_writes": freshWrites, "requests": requests,
		"warm_requests": warm, "warm_hits": hits, "cache_write_tokens": written, "cache_read_tokens": read,
		"input_plus_cache": total, "read_token_ratio": rate, "repeated_write_tokens": repeatedWrite, "ttl": "default"})
}
