package claudecheck

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Compare declared model identifiers, not model authenticity. Only documented
// model-ID shapes and Bedrock wrappers are normalized. Arbitrary aliases and
// application inference profile IDs require external configuration evidence.
// https://platform.claude.com/docs/en/about-claude/models/model-ids-and-versions
var namedModelID = regexp.MustCompile(`^claude-(opus|sonnet|haiku|fable|mythos)-([0-9]+)(?:-([0-9]{1,2}))?(?:-([0-9]{8}))?$`)
var legacyModelID = regexp.MustCompile(`^claude-([0-9]+)(?:-([0-9]{1,2}))?-(opus|sonnet|haiku)(?:-([0-9]{8}))?$`)
var bedrockRevision = regexp.MustCompile(`-v([0-9]+(?::[0-9]+)?)$`)

type declaredModel struct {
	name, major, minor, date, revision string
	known                              bool
}

func parseDeclaredModel(raw string) declaredModel {
	value := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(value, "arn:") {
		parts := strings.SplitN(value, ":", 6)
		if len(parts) != 6 || parts[2] != "bedrock" {
			return declaredModel{}
		}
		resource := strings.SplitN(parts[5], "/", 2)
		if len(resource) != 2 || (resource[0] != "foundation-model" && resource[0] != "inference-profile") {
			return declaredModel{}
		}
		value = resource[1]
	}
	for _, region := range []string{"global", "us", "eu", "apac", "au", "jp"} {
		if strings.HasPrefix(value, region+".anthropic.") {
			value = strings.TrimPrefix(value, region+".")
			break
		}
	}
	revision := ""
	if strings.HasPrefix(value, "anthropic.") {
		value = strings.TrimPrefix(value, "anthropic.")
		if match := bedrockRevision.FindStringSubmatch(value); match != nil {
			revision, value = match[1], value[:len(value)-len(match[0])]
		}
	}
	match := namedModelID.FindStringSubmatch(value)
	if match == nil {
		if old := legacyModelID.FindStringSubmatch(value); old != nil {
			match = []string{old[0], old[3], old[1], old[2], old[4]}
		}
	}
	if match == nil {
		return declaredModel{}
	}
	if match[4] != "" {
		if _, err := time.Parse("20060102", match[4]); err != nil {
			return declaredModel{}
		}
	}
	return declaredModel{name: match[1], major: match[2], minor: match[3], date: match[4], revision: revision, known: true}
}

// 1 = compatible declared IDs, -1 = explicitly different, 0 = unresolved.
func declaredModelRelation(left, right string) int {
	a, b := parseDeclaredModel(left), parseDeclaredModel(right)
	if !a.known || !b.known {
		return 0
	}
	if a.name != b.name || a.major != b.major || a.minor != b.minor {
		return -1
	}
	if a.revision != "" && b.revision != "" && a.revision != b.revision {
		return -1
	}
	if a.date != b.date {
		if a.date != "" && b.date != "" {
			return -1
		}
		minor, _ := strconv.Atoi(a.minor)
		if a.major != "3" && !(a.major == "4" && a.minor != "" && minor <= 5) {
			return 0
		}
	}
	return 1
}

type modelConflict struct {
	Probe    string `json:"probe"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
}

func declaredModelCheck(requested string, sample Sample) (bool, []modelConflict) {
	var conflicts []modelConflict
	compare := func(field, expected, observed string) int {
		relation := declaredModelRelation(expected, observed)
		if relation < 0 {
			conflicts = append(conflicts, modelConflict{Probe: sample.Probe, Field: field, Expected: expected, Observed: observed})
		}
		return relation
	}
	returned := compare("returned_model", requested, sample.ResponseModel)
	mapped := 1
	if sample.UpstreamModel != "" {
		mapped = compare("configured_mapping", requested, sample.UpstreamModel)
		compare("mapping_response", sample.UpstreamModel, sample.ResponseModel)
	}
	return returned != 0 && mapped != 0, conflicts
}

func (r *runner) modelEcho(m message, response Response) {
	comparable, conflicts := declaredModelCheck(r.report.Model, Sample{Probe: r.report.BaselineProbe, UpstreamModel: response.Model, ResponseModel: m.Model})
	state, code := modelDeclarationResult(comparable, len(conflicts))
	r.check("model_echo", state, code, map[string]any{"baseline_probe": r.report.BaselineProbe, "requested": r.report.Model, "upstream": response.Model, "returned": m.Model, "conflicts": conflicts, "identity_verified": false})
}

func modelDeclarationResult(comparable bool, conflicts int) (string, string) {
	if conflicts > 0 {
		return "fail", "model_declaration_mismatch"
	}
	if comparable {
		return "pass", "model_declaration_match"
	}
	return "inconclusive", "model_declaration_unknown"
}

func (r *runner) modelConsistency() {
	inspected, compared := 0, 0
	mapped, returned := map[string]bool{}, map[string]bool{}
	first := map[string]string{}
	var conflicts []modelConflict
	for _, sample := range r.report.Samples {
		if IsCountProbe(sample.Probe) || sample.Status != 200 || sample.Valid == nil || !*sample.Valid {
			continue
		}
		inspected++
		comparable, differences := declaredModelCheck(r.report.Model, sample)
		if comparable {
			compared++
		}
		conflicts = append(conflicts, differences...)
		if sample.UpstreamModel != "" {
			mapped[sample.UpstreamModel] = true
		}
		if sample.ResponseModel != "" {
			returned[sample.ResponseModel] = true
		}
		for _, entry := range []struct{ field, value string }{{"mapped_across_requests", sample.UpstreamModel}, {"returned_across_requests", sample.ResponseModel}} {
			if !parseDeclaredModel(entry.value).known {
				continue
			}
			if first[entry.field] == "" {
				first[entry.field] = entry.value
				continue
			}
			if declaredModelRelation(first[entry.field], entry.value) < 0 {
				conflicts = append(conflicts, modelConflict{Probe: sample.Probe, Field: entry.field, Expected: first[entry.field], Observed: entry.value})
			}
		}
	}
	state, code := modelDeclarationResult(inspected > 0 && inspected == compared, len(conflicts))
	if inspected == 0 {
		state, code = "skipped", "baseline_unavailable"
	}
	evidence := map[string]any{"requested": r.report.Model, "inspected_samples": inspected, "compared_samples": compared, "unresolved_samples": inspected - compared, "mapped_models": sortedModelNames(mapped), "returned_models": sortedModelNames(returned), "conflicts": conflicts, "identity_verified": false, "evidence_type": "declared_model_ids"}
	// Preserve observed mismatches even if a later probe was cancelled. The
	// cancellation policy for active requests must not erase earlier evidence.
	check := Check{ID: "model_consistency", Status: state, Code: code, Evidence: evidence}
	r.report.Checks = append(r.report.Checks, check)
	r.emit(Event{Type: "check", Check: &check})
}

func sortedModelNames(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
