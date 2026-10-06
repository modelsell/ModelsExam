package server

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"model-check/common"
	"model-check/internal/store"
	"model-check/pkg/claudecheck"
	"model-check/pkg/geminicheck"
	"model-check/pkg/imagecheck"
	"model-check/pkg/openaicheck"
)

func (s *Server) listHistory(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, size := 1, 20
	for _, item := range []struct {
		name  string
		value *int
		max   int
	}{{"page", &page, 1000000}, {"page_size", &size, 100}} {
		if raw := c.Query(item.name); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 || v > item.max {
				c.JSON(400, gin.H{"success": false, "message": "Invalid pagination"})
				return
			}
			*item.value = v
		}
	}
	status := c.Query("status")
	switch status {
	case "", "running", "completed", "failed", "cancelled", "interrupted", "stopped":
	default:
		c.JSON(400, gin.H{"success": false, "message": "Invalid history status"})
		return
	}
	name := strings.TrimSpace(c.Query("model"))
	if len(name) > 200 {
		c.JSON(400, gin.H{"success": false, "message": "Invalid model name"})
		return
	}
	rows, total, err := s.cfg.Store.ListRuns(c.Request.Context(), store.ListQuery{OwnerID: ownerID(c), ModelName: name, Status: status, Page: page, PageSize: size})
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load check history"})
		return
	}
	// "mine" lets the UI show which rows this browser may annotate.
	owner := ownerID(c)
	type item struct {
		store.Run
		Mine bool `json:"mine"`
	}
	items := make([]item, len(rows))
	for i, r := range rows {
		items[i] = item{Run: r}
		items[i].Mine = r.OwnerID == owner
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page, "page_size": size}})
}

// historyDetail decodes a stored run. The report type depends on the
// transport; OpenAI runs also carry the Markdown export, rebuilt from the
// stored (already redacted) report.
func historyDetail(run *store.Run, owner string) (gin.H, bool) {
	out := gin.H{"run": run, "mine": run.OwnerID == owner}
	if run.Transport == openAITransportName {
		var report openaicheck.Report
		if common.UnmarshalJsonStr(run.ReportJSON, &report) != nil {
			return nil, false
		}
		if run.Status == "interrupted" {
			report.Cancelled = true
		}
		out["report"], out["markdown"] = report, openaicheck.Markdown(report)
		return out, true
	}
	if run.Transport == geminiTransportName {
		var report geminicheck.Report
		if common.UnmarshalJsonStr(run.ReportJSON, &report) != nil {
			return nil, false
		}
		if run.Status == "interrupted" {
			report.Cancelled = true
		}
		out["report"], out["markdown"] = report, geminicheck.Markdown(report)
		return out, true
	}
	if run.Transport == imageTransportName {
		var report imagecheck.Report
		if common.UnmarshalJsonStr(run.ReportJSON, &report) != nil {
			return nil, false
		}
		if run.Status == "interrupted" {
			report.Cancelled = true
		}
		out["report"], out["markdown"] = report, imagecheck.Markdown(report)
		return out, true
	}
	var report claudecheck.Report
	if common.UnmarshalJsonStr(run.ReportJSON, &report) != nil {
		return nil, false
	}
	if run.Status == "interrupted" {
		report.Cancelled = true
	}
	out["report"] = report
	return out, true
}

func (s *Server) getHistory(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(400, gin.H{"success": false, "message": "Invalid report ID"})
		return
	}
	run, err := s.cfg.Store.GetRun(c.Request.Context(), id)
	// Reports are listed only to the browser that ran them, but the random
	// report ID works as a share link: anyone who has it may open the report.
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Check report not found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load check report"})
		return
	}
	data, ok := historyDetail(run, ownerID(c))
	if !ok {
		c.JSON(500, gin.H{"success": false, "message": "Failed to load check report"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": data})
}

func (s *Server) updateRemark(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid report ID"})
		return
	}
	var body struct {
		Remark *string `json:"remark"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.Remark == nil || !utf8.ValidString(*body.Remark) || utf8.RuneCountInString(*body.Remark) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Remark must contain at most 200 characters"})
		return
	}
	run, err := s.cfg.Store.UpdateRemark(c.Request.Context(), ownerID(c), id, *body.Remark)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Check report not found"})
		return
	case errors.Is(err, store.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Only the browser that ran this check can edit its remark"})
		return
	case errors.Is(err, store.ErrRunning):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Wait for the model check to finish before editing its remark"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update check remark"})
		return
	}
	data, ok := historyDetail(run, ownerID(c))
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to load check report"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// ---- baselines ----

func (s *Server) listBaselines(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rows, err := s.cfg.Store.ListBaselines(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Could not load comparison baselines"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": rows})
}

var baselineTypePattern = regexp.MustCompile(`^[\p{L}\p{N} _-]+$`)

func (s *Server) createBaseline(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		ReportID string `json:"report_id"`
		Type     string `json:"type"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if common.DecodeJson(c.Request.Body, &input) != nil {
		c.JSON(400, gin.H{"success": false, "message": "Invalid report ID"})
		return
	}
	if _, err := uuid.Parse(input.ReportID); err != nil {
		c.JSON(400, gin.H{"success": false, "message": "Invalid report ID"})
		return
	}
	input.Type = strings.TrimSpace(input.Type)
	if utf8.RuneCountInString(input.Type) > 40 || !baselineTypePattern.MatchString(input.Type) {
		c.JSON(400, gin.H{"success": false, "message": "Enter a baseline type using letters, numbers, spaces, underscores or hyphens"})
		return
	}
	run, err := s.cfg.Store.GetRun(c.Request.Context(), input.ReportID)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && run.OwnerID != ownerID(c)) {
		c.JSON(404, gin.H{"success": false, "message": "Check report not found"})
		return
	}
	var report claudecheck.Report
	if err != nil || run.Transport != claudeTransportName || common.UnmarshalJsonStr(run.ReportJSON, &report) != nil {
		c.JSON(400, gin.H{"success": false, "message": "Only a Claude check report can become a baseline"})
		return
	}
	valid := false
	for _, sample := range report.Samples {
		valid = valid || (sample.Valid != nil && *sample.Valid && !claudecheck.IsCountProbe(sample.Probe))
	}
	if (run.Status != "completed" && run.Status != "failed") || report.Cancelled || report.StopReason != "" || !valid || report.Version < 6 {
		c.JSON(400, gin.H{"success": false, "message": "Run a complete current-version check before setting a baseline"})
		return
	}
	for _, check := range report.Checks {
		if (check.ID == "model_echo" || check.ID == "model_consistency") && check.Status == "fail" {
			c.JSON(400, gin.H{"success": false, "message": "Resolve model-name conflicts before setting a baseline"})
			return
		}
	}
	snapshot := claudecheck.SnapshotBaseline(report, uuid.NewString(), input.Type, time.Now().UnixMilli())
	data, err := common.Marshal(snapshot)
	if err != nil || len(data) > store.MaxBaselineBytes {
		c.JSON(400, gin.H{"success": false, "message": "The baseline snapshot exceeds the size limit"})
		return
	}
	row := store.Baseline{ID: snapshot.ID, Type: snapshot.Type, ReportID: report.ID, ModelName: report.Model,
		ChannelName: report.ChannelName, CreatedAt: snapshot.CreatedAt,
		Fingerprint: claudecheck.FingerprintComplete(report.Fingerprint), SnapshotJSON: string(data)}
	if s.cfg.Store.CreateBaseline(c.Request.Context(), &row) != nil {
		c.JSON(500, gin.H{"success": false, "message": "Could not save the comparison baseline"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": row})
}

func (s *Server) loadBaselines(c *gin.Context) ([]claudecheck.ComparisonBaseline, bool) {
	rows, err := s.cfg.Store.LoadBaselines(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Could not load comparison baselines"})
		return nil, false
	}
	snapshots := make([]claudecheck.ComparisonBaseline, 0, len(rows))
	for _, row := range rows {
		var snapshot claudecheck.ComparisonBaseline
		if common.UnmarshalJsonStr(row.SnapshotJSON, &snapshot) != nil {
			c.JSON(500, gin.H{"success": false, "message": "Could not load comparison baselines"})
			return nil, false
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, true
}

// selectBaselines resolves the chosen comparison before history creation or
// any model request. Clients omit comparison when no snapshot is selected;
// explicit legacy compare_baselines requests keep automatic matching.
func (s *Server) selectBaselines(c *gin.Context, options *claudecheck.Options) ([]claudecheck.ComparisonBaseline, bool) {
	options.BaselineID, options.BaselineType = strings.TrimSpace(options.BaselineID), strings.TrimSpace(options.BaselineType)
	if options.BaselineID == "" {
		if options.BaselineType != "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Select a saved baseline for the chosen type"})
			return nil, false
		}
		if options.CompareBaselines {
			return s.loadBaselines(c)
		}
		return nil, true
	}
	parsed, err := uuid.Parse(options.BaselineID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid comparison baseline"})
		return nil, false
	}
	row, err := s.cfg.Store.GetBaseline(c.Request.Context(), parsed.String())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "The selected comparison baseline is unavailable"})
		return nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not load comparison baselines"})
		return nil, false
	}
	if options.BaselineType != "" && !strings.EqualFold(options.BaselineType, row.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "The selected baseline does not match the chosen type"})
		return nil, false
	}
	var snapshot claudecheck.ComparisonBaseline
	if common.UnmarshalJsonStr(row.SnapshotJSON, &snapshot) != nil || snapshot.ID != row.ID || snapshot.ReportID != row.ReportID ||
		snapshot.Type != row.Type || snapshot.Version < 6 || !claudecheck.ComparisonModelsMatch(snapshot.Model, row.ModelName) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "The selected comparison baseline is unavailable"})
		return nil, false
	}
	if !claudecheck.ComparisonModelsMatch(options.Model, snapshot.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "The selected baseline does not match the requested model"})
		return nil, false
	}
	// Earlier snapshots omitted individual fingerprint usage rows; the source
	// report may supply them. Missing history never invalidates the choice.
	if source, err := s.cfg.Store.GetRun(c.Request.Context(), snapshot.ReportID); err == nil {
		var sourceReport claudecheck.Report
		if common.UnmarshalJsonStr(source.ReportJSON, &sourceReport) == nil {
			snapshot = claudecheck.RestoreBaselineFingerprintUsage(snapshot, sourceReport, store.MaxBaselineBytes)
		}
	}
	options.CompareBaselines = true
	options.BaselineID, options.BaselineType = snapshot.ID, snapshot.Type
	return []claudecheck.ComparisonBaseline{snapshot}, true
}
