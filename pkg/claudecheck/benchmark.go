package claudecheck

const BenchmarkRequests = CapabilityRequests

type BenchmarkItem struct {
	ID         string               `json:"id"`
	Category   string               `json:"category"`
	Profile    string               `json:"profile"`
	Expected   string               `json:"expected"`
	Correct    *bool                `json:"correct"`
	Score      *int                 `json:"score,omitempty"`
	Actual     string               `json:"actual,omitempty"`
	Grader     string               `json:"grader,omitempty"`
	Code       string               `json:"code,omitempty"`
	Assertions []BenchmarkAssertion `json:"assertions,omitempty"`
}

type BenchmarkAssertion struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
}

type CapabilityBenchmark struct {
	Version int             `json:"version"`
	Items   []BenchmarkItem `json:"items"`
}

// Every newly executed benchmark uses the current capability tasks. Historical
// v1/v2 reports retain their stored evidence through the report readers.
func (r *runner) benchmark() {
	r.capability()
}
