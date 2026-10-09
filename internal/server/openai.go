package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"model-check/common"
	"model-check/internal/siteinfo"
	"model-check/internal/store"
	"model-check/pkg/openaicheck"
)

// openAITransportName is the history transport label for OpenAI checks.
const openAITransportName = "openai_api"

func (s *Server) checkOpenAI(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		openaicheck.Options
		BaseURL string `json:"base_url"`
		Key     string `json:"key"`
		Remark  string `json:"remark"`
		// CredentialID uses a saved key instead of Key (signed-in accounts only).
		CredentialID string `json:"credential_id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if common.DecodeJson(c.Request.Body, &input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	var lease *keyLease
	if input.CredentialID != "" {
		var ok bool
		if lease, ok = s.leaseFor(c, input.CredentialID, "openai", input.BaseURL); !ok {
			return
		}
		defer lease.finish(s.cfg.Store)
		input.BaseURL, input.Key = lease.endpoint, lease.secret
	}
	input.Model, input.Key = strings.TrimSpace(input.Model), strings.TrimSpace(input.Key)
	endpoint, err := openaicheck.NormalizeBaseURL(input.BaseURL)
	if err != nil || len(input.Key) < 4 || len(input.Key) > 8192 || strings.ContainsAny(input.Key, "\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Enter a valid Base URL, API key, and model"})
		return
	}
	if !openaicheck.ValidOptions(input.Options) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid model check options"})
		return
	}
	input.Remark = strings.TrimSpace(input.Remark)
	if !utf8.ValidString(input.Remark) || utf8.RuneCountInString(input.Remark) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Remark must contain at most 200 characters"})
		return
	}
	client := s.newClient()
	defer client.CloseIdleConnections()
	if err := client.ValidateURL(c.Request.Context(), endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": preflightMessage(err), "detail": strings.ReplaceAll(err.Error(), input.Key, "[redacted]")})
		return
	}
	release, ok := s.acquire(c, "openai", s.openaiSlots)
	if !ok {
		return
	}
	defer release()
	// Name and description of the tested site, read from its home page. A
	// failure leaves them empty; the check never depends on it.
	site := siteinfo.Fetch(c.Request.Context(), client.Client, client.ValidateURL, endpoint)
	if lease != nil {
		lease.watch.wrap(client.Client)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), openaicheck.RunTimeout)
	defer cancel()
	redactor := openaicheck.NewRedactor(input.Key)
	transport := openaicheck.NewHTTPTransport(client.Client, endpoint, input.Key, redactor)

	// Markdown is rendered from the redacted copy so the credential can never
	// reach the exported report through evidence or upstream error text.
	redactReport := func(report *openaicheck.Report) (data []byte, safe openaicheck.Report, err error) {
		if data, err = common.Marshal(report); err != nil {
			return nil, safe, err
		}
		data = redactor.JSON(data)
		err = common.Unmarshal(data, &safe)
		return data, safe, err
	}

	stream := strings.Contains(c.GetHeader("Accept"), "text/event-stream")
	var writeMu sync.Mutex
	writeStream := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if c.Request.Context().Err() != nil {
			return c.Request.Context().Err()
		}
		if _, err := c.Writer.Write(data); err != nil {
			cancel()
			return err
		}
		c.Writer.Flush()
		return nil
	}
	var stopKeepalive func()
	defer func() {
		if stopKeepalive != nil {
			stopKeepalive()
		}
	}()

	history := &openAIHistory{store: s.cfg.Store, owner: ownerID(c), meta: requestMeta(c, lease), redactor: redactor, siteName: site.Name, siteDesc: site.Description}
	defer history.finalizeInterrupted()
	report := openaicheck.RunWithObserver(ctx, input.Options, transport, func(event openaicheck.Event) {
		if event.Report != nil {
			event.Report.Endpoint = endpoint
			if input.Remark != "" {
				remark := input.Remark
				event.Report.Remark = remark
			}
		}
		// A failed history write never fails the check: the report still reaches
		// the caller, flagged with history_saved=false on "done".
		switch event.Type {
		case "start", "done", "sample", "check", "probe_start":
			_ = history.observe(event)
		}
		if !stream {
			return
		}
		if event.Type == "start" {
			c.Header("Content-Type", "text/event-stream")
			c.Header("X-Accel-Buffering", "no")
		}
		if event.Type == "done" && event.Report != nil {
			if _, safe, err := redactReport(event.Report); err == nil {
				event.Markdown = openaicheck.Markdown(safe)
			}
		}
		data, err := common.Marshal(event)
		if err != nil {
			cancel()
			return
		}
		data = redactor.JSON(data)
		if err = writeStream(append(append([]byte("data: "), data...), '\n', '\n')); err != nil {
			cancel()
			return
		}
		if event.Type == "start" {
			stopKeepalive = startKeepalive(ctx, 10*time.Second, writeStream)
		}
	})
	if stream {
		return
	}
	report.Endpoint = endpoint
	if report.Remark == "" {
		report.Remark = input.Remark
	}
	data, safe, err := redactReport(&report)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to encode report"})
		return
	}
	markdown, err := common.Marshal(openaicheck.Markdown(safe))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to encode report"})
		return
	}
	body := append([]byte(`{"success":true,"data":`), data...)
	body = append(append(body, []byte(`,"markdown":`)...), markdown...)
	c.Data(http.StatusOK, "application/json", append(body, '}'))
}

// openAIHistory mirrors claudeHistory for OpenAI reports.
type openAIHistory struct {
	store         *store.Store
	owner         string
	meta          runMeta
	redactor      *openaicheck.Redactor
	snapshot      openaicheck.Report
	activeProbe   string
	siteName      string
	siteDesc      string
	started       time.Time
	created       bool
	finished      bool
	createErr     error
	checkpointErr error
}

func (h *openAIHistory) observe(event openaicheck.Event) error {
	switch event.Type {
	case "start":
		h.snapshot, h.started = *event.Report, time.Now()
	case "probe_start":
		h.activeProbe = event.Probe
	case "sample":
		if event.Sample != nil {
			h.snapshot.Samples = append(h.snapshot.Samples, *event.Sample)
		}
	case "check":
		if event.Check != nil {
			h.snapshot.Checks = append(h.snapshot.Checks, *event.Check)
		}
	case "done":
		h.snapshot, h.activeProbe = *event.Report, ""
	}
	if h.createErr != nil {
		return h.createErr
	}
	if h.checkpointErr != nil && event.Type != "done" {
		return nil
	}
	state := "running"
	if event.Type == "done" {
		state = finalState(h.snapshot.Summary["fail"], h.snapshot.StopReason, h.snapshot.Cancelled)
	}
	err := h.save(state)
	if err != nil {
		if !h.created {
			h.createErr = err
		} else {
			h.checkpointErr = err
		}
	}
	if event.Type == "done" {
		h.finished = true
		saved := err == nil
		event.Report.HistorySaved = &saved
	}
	return err
}

func (h *openAIHistory) save(state string) error {
	saved := true
	h.snapshot.HistorySaved = &saved
	// The runner finalizes the summary only at the end; keep checkpoints
	// consistent with the checks collected so far.
	h.snapshot.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, check := range h.snapshot.Checks {
		h.snapshot.Summary[check.Status]++
	}
	h.snapshot.RequestsRun = len(h.snapshot.Samples)
	if state == "running" {
		h.snapshot.DurationMS = time.Since(h.started).Milliseconds()
		h.snapshot.Score = openaicheck.Score(h.snapshot)
	}
	data, err := common.Marshal(h.snapshot)
	if err != nil {
		return err
	}
	data = h.redactor.JSON(data)
	var safe openaicheck.Report
	if err := common.Unmarshal(data, &safe); err != nil {
		return err
	}
	var remark *string
	if safe.Remark != "" {
		remark = &safe.Remark
	}
	run := &store.Run{
		ID: h.snapshot.ID, OwnerID: h.owner, ModelName: safe.Model, ChannelName: h.siteName, SiteDescription: h.siteDesc, Endpoint: safe.Endpoint, Remark: remark,
		Transport: openAITransportName, Score: openaicheck.Score(safe), ScoreVersion: store.ScoreVersion,
		Status: state, StartedAt: h.started.UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
		DurationMS: safe.DurationMS, RequestCount: len(safe.Samples), PassCount: safe.Summary["pass"], FailCount: safe.Summary["fail"],
		ActiveProbe: h.activeProbe, ReportJSON: string(data),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !h.created {
		h.meta.apply(run)
		err = h.store.CreateRun(ctx, run)
		h.created = err == nil
		return err
	}
	return h.store.UpdateRun(ctx, run)
}

func (h *openAIHistory) finalizeInterrupted() {
	if !h.created || h.finished {
		return
	}
	h.snapshot.Cancelled = true
	h.snapshot.DurationMS = time.Since(h.started).Milliseconds()
	h.activeProbe = ""
	_ = h.save("interrupted")
}
