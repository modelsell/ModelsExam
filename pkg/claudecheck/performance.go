package claudecheck

import "fmt"

const PerformanceRequests = 3

func performanceBody(model string) map[string]any {
	body := referenceBody(model, "Output the integers 1 through 200 in ascending order, separated by commas. Output no explanation.", 512)
	body["stream"] = true
	return body
}

func (r *runner) performance() {
	for i := 1; i <= PerformanceRequests && r.ctx.Err() == nil; i++ {
		id := fmt.Sprintf("performance_%d", i)
		exists := false
		for _, sample := range r.report.Samples {
			exists = exists || sample.Probe == id
		}
		if exists {
			continue
		}
		_, response, _ := r.probe(id, Request{Body: performanceBody(r.report.Model)})
		if response.ErrorCode == "access_denied" || response.Status == 429 {
			break
		}
	}
	r.check("performance_sampling", "inconclusive", "performance_collected", nil)
}
