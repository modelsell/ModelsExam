package claudecheck

import (
	"math"
	"sort"
	"strings"
)

const UsageTokenRequests = 4
const usageTokensMaxSafeInteger int64 = 1<<53 - 1
const usageTokensSystem = "The user message is synthetic input data for a usage accounting check. Do not analyze or quote it. Reply with exactly PONG."

var usageTokenTasks = []struct {
	probe      string
	words      int
	characters int
}{
	{"usage_tokens_en_30", 30, 0},
	{"usage_tokens_en_300", 300, 0},
	{"usage_tokens_en_1500", 1500, 0},
	{"usage_tokens_zh_1000", 0, 1000},
}

type usageTokenFixture struct {
	probe string
	body  map[string]any
}

// Fixed original data, shared prefixes and identical settings isolate changes
// in input length. Word counts are not claimed to equal provider token counts.
func usageTokenFixtures(model string) []usageTokenFixture {
	vocabulary := strings.Fields("apple bridge cloud garden river stone window paper green blue warm quiet small bright path field maple pine lake bird music light book desk chair glass water bread home road wind rain sun moon star tree leaf flower boat sand hill gate cup bowl seed grass farm door coat shoe wool silk bell clock house table plate spoon fork lamp roof wall floor room train plane wheel box rope ring flag")
	words := make([]string, 1500)
	for i := range words {
		words[i] = vocabulary[(i*17+i/11)%len(vocabulary)]
	}
	characters := []rune("春风吹过山间小路清水流向远方田野农人整理种子林中鸟儿迎着日光飞向天空")
	chinese := make([]rune, 1000)
	for i := range chinese {
		chinese[i] = characters[i%len(characters)]
	}
	fixtures := make([]usageTokenFixture, 0, UsageTokenRequests)
	for _, task := range usageTokenTasks {
		content := string(chinese)
		if task.words > 0 {
			content = strings.Join(words[:task.words], " ")
		}
		fixtures = append(fixtures, usageTokenFixture{task.probe, map[string]any{
			"model": model, "max_tokens": 32, "system": usageTokensSystem,
			"thinking": map[string]any{"type": "disabled"},
			"messages": []any{map[string]any{"role": "user", "content": content}},
		}})
	}
	return fixtures
}

type UsageTokenRow struct {
	Probe          string   `json:"probe"`
	Words          int      `json:"words,omitempty"`
	Characters     int      `json:"characters,omitempty"`
	TotalInput     *int64   `json:"total_input"`
	Code           string   `json:"code"`
	ReferenceInput *float64 `json:"reference_input"`
	Diff           *float64 `json:"diff"`
	BaselineScore  *int     `json:"baseline_score"`
}

type UsageTokenAssessment struct {
	Active         bool            `json:"active"`
	Code           string          `json:"code"`
	Score          *int            `json:"score"`
	GrowthScore    *int            `json:"growth_score"`
	BaselineScore  *int            `json:"baseline_score"`
	Measured       int             `json:"measured"`
	Total          int             `json:"total"`
	SlopeRatio     *float64        `json:"slope_ratio"`
	LongShortRatio *float64        `json:"long_short_ratio"`
	BaselineCode   string          `json:"baseline_code"`
	Rows           []UsageTokenRow `json:"rows"`
}

// AssessUsageTokens derives the score from immutable report samples, including
// a selected baseline attached after collection. Check status/evidence is only
// a collection-time observation and cannot override this assessment.
// Growth measures plausibility, not token accuracy or model authenticity.
func AssessUsageTokens(report Report) UsageTokenAssessment {
	assessment := UsageTokenAssessment{Code: "not_applicable", BaselineCode: "not_selected", Total: UsageTokenRequests, Rows: []UsageTokenRow{}}
	if report.Version < 19 || report.Options == nil || report.Options.Suite != "focused" {
		return assessment
	}
	for _, item := range report.Plan {
		if item.ID == "usage_token_integrity" && item.Selected {
			assessment.Active = true
		}
	}
	for _, sample := range report.Samples {
		if usageTokenProbe(sample.Probe) {
			assessment.Active = true
		}
	}
	if !assessment.Active {
		return assessment
	}
	assessment.Code = "usage_incomplete"
	baseline := usageTokenBaseline(report)
	if report.Options.BaselineID != "" {
		assessment.BaselineCode = "baseline_unavailable"
		if baseline != nil {
			assessment.BaselineCode = "baseline_incomplete"
		}
	}
	for _, task := range usageTokenTasks {
		row := UsageTokenRow{Probe: task.probe, Words: task.words, Characters: task.characters, Code: "not_collected"}
		var sample *Sample
		count := 0
		for i := range report.Samples {
			if report.Samples[i].Probe == task.probe {
				sample = &report.Samples[i]
				count++
			}
		}
		if count > 1 {
			row.Code = "ambiguous_sample"
		} else if sample != nil {
			row.TotalInput, row.Code = usageTokenTotal(*sample)
			if row.TotalInput != nil {
				assessment.Measured++
				if baseline != nil && strings.TrimSpace(sample.RequestProfile) != "" {
					var totals []float64
					for _, reference := range baseline.Samples {
						if reference.Probe != task.probe || reference.RequestProfile != sample.RequestProfile {
							continue
						}
						if total, _ := usageTokenTotal(reference); total != nil {
							totals = append(totals, float64(*total))
						}
					}
					if len(totals) > 0 {
						sort.Float64s(totals)
						middle := len(totals) / 2
						value := totals[middle]
						if len(totals)%2 == 0 {
							value = (totals[middle-1] + value) / 2
						}
						actual := float64(*row.TotalInput)
						diff := actual - value
						score := 100
						if math.Abs(diff) > math.Max(4, value*0.05) {
							score = int(math.Round(100 * math.Min(actual, value) / math.Max(actual, value)))
						}
						row.ReferenceInput, row.Diff, row.BaselineScore = &value, &diff, &score
					}
				}
			}
		}
		assessment.Rows = append(assessment.Rows, row)
	}
	if assessment.Measured != UsageTokenRequests {
		return assessment
	}
	short, medium, long := float64(*assessment.Rows[0].TotalInput), float64(*assessment.Rows[1].TotalInput), float64(*assessment.Rows[2].TotalInput)
	if short > 0 {
		ratio := long / short
		assessment.LongShortRatio = &ratio
	}
	growth := 0
	if short < medium && medium < long {
		first, second := (medium-short)/270, (long-medium)/1200
		ratio := math.Min(first, second) / math.Max(first, second)
		assessment.SlopeRatio = &ratio
		growth = 100
		if ratio < 0.8 {
			growth = int(math.Round(100 * ratio / 0.8))
		}
	}
	assessment.GrowthScore = &growth
	score := growth
	var baselineScores []*int
	for _, row := range assessment.Rows {
		if row.BaselineScore != nil {
			baselineScores = append(baselineScores, row.BaselineScore)
		}
	}
	if len(baselineScores) == UsageTokenRequests {
		assessment.BaselineCode = "baseline_complete"
		assessment.BaselineScore = reportScoreMean(baselineScores)
		if *assessment.BaselineScore < score {
			score = *assessment.BaselineScore
		}
	}
	assessment.Score = &score
	assessment.Code = "usage_consistent"
	if score < 100 {
		assessment.Code = "usage_anomaly"
	}
	return assessment
}

func usageTokenProbe(probe string) bool {
	for _, task := range usageTokenTasks {
		if task.probe == probe {
			return true
		}
	}
	return false
}

func usageTokenTotal(sample Sample) (*int64, string) {
	if sample.Status != 200 || sample.Valid == nil || !*sample.Valid {
		return nil, "invalid_response"
	}
	if sample.Usage.Input == nil {
		return nil, "missing_input"
	}
	var total int64
	for _, value := range []*int64{sample.Usage.Input, sample.Usage.CacheWrite, sample.Usage.CacheRead} {
		// Providers omit optional cache counters when no cache was used.
		if value == nil {
			continue
		}
		if *value < 0 || *value > usageTokensMaxSafeInteger {
			return nil, "invalid_usage"
		}
		if *value > usageTokensMaxSafeInteger-total {
			return nil, "usage_overflow"
		}
		total += *value
	}
	if total == 0 {
		return nil, "nonpositive_input"
	}
	return &total, "observed"
}

func usageTokenBaseline(report Report) *ComparisonBaseline {
	if report.Options.BaselineID == "" || strings.TrimSpace(report.Options.BaselineType) == "" {
		return nil
	}
	var selected *ComparisonBaseline
	for i := range report.Baselines {
		baseline := &report.Baselines[i]
		if baseline.ID == report.Options.BaselineID {
			if selected != nil {
				return nil // Ambiguous stored references must not be merged.
			}
			selected = baseline
		}
	}
	if selected != nil && (!strings.EqualFold(strings.TrimSpace(selected.Type), strings.TrimSpace(report.Options.BaselineType)) || !ComparisonModelsMatch(report.Model, selected.Model)) {
		return nil
	}
	return selected
}

func (r *runner) usageTokens() {
	for _, fixture := range usageTokenFixtures(r.report.Model) {
		if r.ctx.Err() != nil || r.focusedStop() {
			break
		}
		r.probe(fixture.probe, Request{Body: fixture.body})
	}
	assessment := AssessUsageTokens(r.report)
	state := "inconclusive"
	if assessment.Score != nil {
		state = status(*assessment.Score == 100)
	}
	r.check("usage_token_integrity", state, assessment.Code, map[string]any{
		"growth_score": assessment.GrowthScore, "measured": assessment.Measured, "total": assessment.Total,
		"slope_ratio": assessment.SlopeRatio, "long_short_ratio": assessment.LongShortRatio, "identity_verified": false,
	})
}
