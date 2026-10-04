package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"model-check/common"
	"model-check/internal/siteinfo"
	"model-check/internal/store"
	"model-check/pkg/imagecheck"
	"model-check/pkg/openaicheck"
)

// imageTransportName is the history transport label for image checks.
const imageTransportName = "image_api"

// checkImage runs the image suite. The endpoint key and the optional official
// OpenAI key (provenance verification) are both redacted from every report,
// stream event, stored row and error before it leaves this handler.
func (s *Server) checkImage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input imagecheck.Input
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if common.DecodeJson(c.Request.Body, &input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	endpoint, err := input.Normalize()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	redactor := openaicheck.NewRedactor(input.Key, input.VerifyKey)
	client := s.newClient()
	defer client.CloseIdleConnections()
	if err := client.ValidateURL(c.Request.Context(), endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": preflightMessage(err), "detail": redactor.String(err.Error())})
		return
	}
	release, ok := s.acquire(c, "image", s.imageSlots)
	if !ok {
		return
	}
	defer release()
	// Name and description of the tested site, read from its home page. A
	// failure leaves them empty; the check never depends on it.
	site := siteinfo.Fetch(c.Request.Context(), client.Client, client.ValidateURL, endpoint)

	ctx, cancel := context.WithTimeout(c.Request.Context(), imagecheck.RunTimeout)
	defer cancel()
	transport, deps := imagecheck.Build(imagecheck.Plumbing{
		Client:       client.Client,
		ValidateURL:  client.ValidateURL,
		Endpoint:     endpoint,
		OfficialBase: s.cfg.VerifyBaseURL,
		Input:        input,
		Redactor:     redactor,
	})

	redactReport := func(report *imagecheck.Report) (data []byte, safe imagecheck.Report, err error) {
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

	history := &imageHistory{store: s.cfg.Store, owner: ownerID(c), redactor: redactor, siteName: site.Name, siteDesc: site.Description}
	defer history.finalizeInterrupted()
	report := imagecheck.Run(ctx, input.Options, transport, deps, func(event imagecheck.Event) {
		if event.Report != nil {
			event.Report.Endpoint = endpoint
			if input.Remark != "" {
				event.Report.Remark = input.Remark
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
				event.Markdown = imagecheck.Markdown(safe)
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
	markdown, err := common.Marshal(imagecheck.Markdown(safe))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to encode report"})
		return
	}
	body := append([]byte(`{"success":true,"data":`), data...)
	body = append(append(body, []byte(`,"markdown":`)...), markdown...)
	c.Data(http.StatusOK, "application/json", append(body, '}'))
}

// imageHistory mirrors openAIHistory for image reports. Generated images stay
// private: every stored snapshot goes through imagecheck.Snapshot, which drops
// the thumbnails that only the live stream carries.
type imageHistory struct {
	store         *store.Store
	owner         string
	redactor      *openaicheck.Redactor
	snapshot      imagecheck.Report
	activeProbe   string
	siteName      string
	siteDesc      string
	started       time.Time
	created       bool
	finished      bool
	createErr     error
	checkpointErr error
}

func (h *imageHistory) observe(event imagecheck.Event) error {
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

func (h *imageHistory) save(state string) error {
	saved := true
	h.snapshot.HistorySaved = &saved
	snap := imagecheck.Snapshot(h.snapshot)
	if state == "running" {
		snap.DurationMS = time.Since(h.started).Milliseconds()
	}
	data, err := common.Marshal(snap)
	if err != nil {
		return err
	}
	data = h.redactor.JSON(data)
	var safe imagecheck.Report
	if err := common.Unmarshal(data, &safe); err != nil {
		return err
	}
	var remark *string
	if safe.Remark != "" {
		remark = &safe.Remark
	}
	run := &store.Run{
		ID: h.snapshot.ID, OwnerID: h.owner, ModelName: safe.Model, ChannelName: h.siteName, SiteDescription: h.siteDesc, Endpoint: safe.Endpoint, Remark: remark,
		Transport: imageTransportName, Score: imagecheck.Score(safe), ScoreVersion: store.ScoreVersion,
		Status: state, StartedAt: h.started.UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
		DurationMS: safe.DurationMS, RequestCount: len(safe.Samples), PassCount: safe.Summary["pass"], FailCount: safe.Summary["fail"],
		ActiveProbe: h.activeProbe, ReportJSON: string(data),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !h.created {
		err = h.store.CreateRun(ctx, run)
		h.created = err == nil
		return err
	}
	return h.store.UpdateRun(ctx, run)
}

func (h *imageHistory) finalizeInterrupted() {
	if !h.created || h.finished {
		return
	}
	h.snapshot.Cancelled = true
	h.snapshot.DurationMS = time.Since(h.started).Milliseconds()
	h.activeProbe = ""
	_ = h.save("interrupted")
}
