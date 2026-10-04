package claudecheck

import (
	"os"
	"testing"

	"model-check/common"
)

func TestReportScoreMatchesSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/report_scores.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name   string `json:"name"`
		Report Report `json:"report"`
		Score  *int   `json:"score"`
	}
	if err := common.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("missing shared report score cases")
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			got := ReportScore(fixture.Report)
			if fixture.Score == nil {
				if got != nil {
					t.Fatalf("score = %d; want unscored", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("unscored; want %d", *fixture.Score)
			}
			if *got != *fixture.Score {
				t.Fatalf("score = %d; want %d", *got, *fixture.Score)
			}
		})
	}
}
