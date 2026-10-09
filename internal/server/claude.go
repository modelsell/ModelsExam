package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"model-check/common"
	"model-check/internal/siteinfo"
	"model-check/internal/store"
	"model-check/pkg/claudecheck"
)

const claudeTransportName = "anthropic_proxy"

func normalizeClaudeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || len(raw) > 2048 || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", http.ErrNotSupported
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, "/v1/messages")
	u.Path = strings.TrimSuffix(u.Path, "/v1")
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// preflightMessage maps a URL-policy failure to a user-facing message.
func preflightMessage(err error) string {
	message := "Endpoint is blocked by the outbound URL policy"
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || strings.HasPrefix(err.Error(), "DNS resolution") {
		message = "Endpoint DNS resolution could not be completed"
	}
	return message
}

func (s *Server) checkClaude(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		claudecheck.Options
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
		if lease, ok = s.leaseFor(c, input.CredentialID, "claude", input.BaseURL); !ok {
			return
		}
		defer lease.finish(s.cfg.Store)
		input.BaseURL, input.Key = lease.endpoint, lease.secret
	}
	input.Model, input.Key = strings.TrimSpace(input.Model), strings.TrimSpace(input.Key)
	endpoint, err := normalizeClaudeURL(input.BaseURL)
	if err != nil || input.Model == "" || len(input.Model) > 200 || len(input.Key) < 4 || len(input.Key) > 8192 || strings.ContainsAny(input.Key, "\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Enter a valid Base URL, API key, and model"})
		return
	}
	input.Remark = strings.TrimSpace(input.Remark)
	if !utf8.ValidString(input.Remark) || utf8.RuneCountInString(input.Remark) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Remark must contain at most 200 characters"})
		return
	}
	options := input.Options
	if !claudecheck.ValidOptions(options) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid model check options"})
		return
	}
	baselines, ok := s.selectBaselines(c, &options)
	if !ok {
		return
	}
	client := s.newClient()
	defer client.CloseIdleConnections()
	if err := client.ValidateURL(c.Request.Context(), endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": preflightMessage(err), "detail": strings.ReplaceAll(err.Error(), input.Key, "[redacted]")})
		return
	}
	release, ok := s.acquire(c, "claude", s.claudeSlots)
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), claudecheck.RunTimeout)
	defer cancel()
	transport := newClaudeTransport(client.Client, endpoint, input.Key)
	decorate := func(report *claudecheck.Report) {
		report.Baselines = baselines
		report.Transport = claudeTransportName
		report.ChannelName = site.Name
		if input.Remark != "" {
			remark := input.Remark
			report.Remark = &remark
		}
		if u, err := url.Parse(endpoint); err == nil {
			u.User, u.RawQuery, u.Fragment = nil, "", ""
			report.Endpoint = u.String()
		}
	}
	history := &claudeHistory{store: s.cfg.Store, owner: ownerID(c), meta: requestMeta(c, lease), redact: transport.redactJSON, siteDesc: site.Description}
	defer history.finalizeInterrupted()
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
	report := claudecheck.RunWithObserver(ctx, options, transport.call, func(event claudecheck.Event) {
		if event.Report != nil {
			if event.Type == "done" && history.checkpointErr != nil {
				event.Report.StopReason = "history_unavailable"
			}
			decorate(event.Report)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			history.interrupted = true
		}
		if err := history.observe(event, event.Type == "start" || event.Type == "done" || ctx.Err() == nil); err != nil {
			cancel()
		}
		if history.createErr != nil || !stream {
			return
		}
		if event.Type == "start" {
			c.Header("Content-Type", "text/event-stream")
			c.Header("X-Accel-Buffering", "no")
		}
		data, err := common.Marshal(event)
		if err != nil {
			cancel()
			return
		}
		data = transport.redactJSON(data)
		if err = writeStream(append(append([]byte("data: "), data...), '\n', '\n')); err != nil {
			cancel()
			return
		}
		if event.Type == "start" {
			stopKeepalive = startKeepalive(ctx, 10*time.Second, writeStream)
		}
	})
	if history.createErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Could not save check history. No upstream requests were sent."})
		return
	}
	if stream {
		return
	}
	data, err := common.Marshal(report)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to encode report"})
		return
	}
	data = transport.redactJSON(data)
	c.Data(http.StatusOK, "application/json", append(append([]byte(`{"success":true,"data":`), data...), '}'))
}

// claudeHistory stores a Claude check: a row on the first event, checkpoints
// while running, then the final state. Snapshots are redacted before writing.
type claudeHistory struct {
	store         *store.Store
	owner         string
	meta          runMeta
	redact        func([]byte) []byte
	snapshot      claudecheck.Report
	activeProbe   string
	siteDesc      string
	started       time.Time
	created       bool
	finished      bool
	createErr     error
	checkpointErr error
	interrupted   bool
}

func (h *claudeHistory) observe(event claudecheck.Event, persist bool) error {
	switch event.Type {
	case "start":
		h.snapshot, h.started = *event.Report, time.Now()
	case "probe_start":
		h.activeProbe = event.Probe
	case "sample":
		h.snapshot.Samples = append(h.snapshot.Samples, *event.Sample)
	case "check":
		h.snapshot.Checks = append(h.snapshot.Checks, *event.Check)
	case "fingerprint":
		h.snapshot.Fingerprint = event.Fingerprint
	case "token_audit":
		h.snapshot.TokenAudit = event.TokenAudit
	case "benchmark":
		h.snapshot.Benchmark = event.Benchmark
	case "done":
		h.snapshot, h.activeProbe = *event.Report, ""
	}
	if h.createErr != nil {
		return h.createErr
	}
	if !persist || (h.checkpointErr != nil && event.Type != "done") {
		return nil
	}
	state := "running"
	if event.Type == "done" {
		state = finalState(h.snapshot.Summary["fail"], h.snapshot.StopReason, h.snapshot.Cancelled)
		if h.interrupted {
			state = "interrupted"
		}
	}
	err := h.save(state)
	if err != nil {
		if !h.created {
			h.createErr = err
		} else {
			h.checkpointErr, h.interrupted = err, true
		}
	}
	if event.Type == "done" {
		h.finished = true
		saved := err == nil
		event.Report.HistorySaved = &saved
	}
	return err
}

func finalState(fails int, stopReason string, cancelled bool) string {
	switch {
	case cancelled:
		return "cancelled"
	case stopReason != "":
		return "stopped"
	case fails > 0:
		return "failed"
	}
	return "completed"
}

func (h *claudeHistory) save(state string) error {
	saved := true
	h.snapshot.HistorySaved = &saved
	h.snapshot.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, check := range h.snapshot.Checks {
		h.snapshot.Summary[check.Status]++
	}
	if state == "running" {
		h.snapshot.DurationMS = time.Since(h.started).Milliseconds()
	}
	data, err := common.Marshal(h.snapshot)
	if err != nil {
		return err
	}
	data = h.redact(data)
	var safe claudecheck.Report
	if err := common.Unmarshal(data, &safe); err != nil {
		return err
	}
	run := &store.Run{
		ID: h.snapshot.ID, OwnerID: h.owner, ModelName: safe.Model, ChannelID: safe.ChannelID,
		ChannelName: safe.ChannelName, SiteDescription: h.siteDesc, Endpoint: safe.Endpoint, Transport: safe.Transport,
		Remark: safe.Remark, Score: claudecheck.ReportScore(safe), ScoreVersion: store.ScoreVersion,
		Status: state, StartedAt: h.started.UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
		DurationMS: safe.DurationMS, RequestCount: len(safe.Samples), PassCount: safe.Summary["pass"], FailCount: safe.Summary["fail"],
		ActiveProbe: h.activeProbe, ReportJSON: string(data),
	}
	// Client cancellation must not cancel the final database write.
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

func (h *claudeHistory) finalizeInterrupted() {
	if !h.created || h.finished {
		return
	}
	h.snapshot.Cancelled = true
	h.snapshot.DurationMS = time.Since(h.started).Milliseconds()
	h.activeProbe = ""
	_ = h.save("interrupted")
}
