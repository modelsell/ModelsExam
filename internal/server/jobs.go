package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"model-check/common"
	"model-check/internal/siteinfo"
	"model-check/internal/ssrf"
	"model-check/pkg/claudecheck"
	"model-check/pkg/geminicheck"
	"model-check/pkg/imagecheck"
	"model-check/pkg/openaicheck"
)

// Background checks run without a browser request: a one-click retest with a
// saved key, or a scheduled run. They use the same engines, history writer
// and redaction as the streaming endpoints, but their own concurrency slots,
// and they keep running when the page that started them is closed.

// checkJob describes one background check.
type checkJob struct {
	Provider           string
	Model              string
	Options            json.RawMessage
	Remark             string
	OwnerID            string // browser that asked for a retest; empty for schedules
	UserID             int64
	CredentialID       string
	VerifyCredentialID string // image provenance key (retest only)
	ScheduleID         string
}

// jobOptions is a provider's options, decoded and validated.
type jobOptions struct {
	claude claudecheck.Options
	openai openaicheck.Options
	gemini geminicheck.Options
	image  imagecheck.Options
}

// parseJobOptions decodes options for provider, ignoring unknown fields, and
// returns the normalized JSON that is stored with a schedule.
func parseJobOptions(provider, model string, raw json.RawMessage) (jobOptions, json.RawMessage, error) {
	var o jobOptions
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 200 {
		return o, nil, errors.New("Enter a model name")
	}
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	var target any
	switch provider {
	case "claude":
		target = &o.claude
	case "openai":
		target = &o.openai
	case "gemini":
		target = &o.gemini
	case "image":
		target = &o.image
	default:
		return o, nil, errors.New("Unknown check type")
	}
	if common.Unmarshal(raw, target) != nil {
		return o, nil, errors.New("Invalid model check options")
	}
	valid := false
	switch provider {
	case "claude":
		o.claude.Model = model
		valid = claudecheck.ValidOptions(o.claude)
	case "openai":
		o.openai.Model = model
		valid = openaicheck.ValidOptions(o.openai)
	case "gemini":
		o.gemini.Model = model
		valid = geminicheck.ValidOptions(o.gemini)
	case "image":
		o.image.Model = model
		valid = imagecheck.ValidOptions(o.image)
	}
	if !valid {
		return o, nil, errors.New("Invalid model check options")
	}
	normalized, err := common.Marshal(target)
	return o, normalized, err
}

func (s *Server) verifyBase() string {
	if s.cfg.VerifyBaseURL != "" {
		return s.cfg.VerifyBaseURL
	}
	return officialOpenAIBase
}

// bgSlot takes one background slot for provider without waiting.
func (s *Server) bgSlot(provider string) (func(), bool) {
	slots := s.bgSlots[provider]
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	default:
		return nil, false
	}
}

// startJob checks out the keys and starts the run. It returns once the run
// has its report ID, or with the reason it could not start. done, if set, is
// called with the report ID when the run ends.
func (s *Server) startJob(job checkJob, done func(runID string)) (string, error) {
	ctx, cancel := context.WithTimeout(s.bgCtx, 15*time.Second)
	defer cancel()
	opts, _, err := parseJobOptions(job.Provider, job.Model, job.Options)
	if err != nil {
		return "", &leaseError{http.StatusBadRequest, err.Error()}
	}
	var baselines []claudecheck.ComparisonBaseline
	if job.Provider == "claude" {
		var lerr *leaseError
		baselines, lerr = s.resolveBaselines(ctx, &opts.claude)
		if lerr != nil && lerr.message == msgBaselineUnavailable {
			// A baseline deleted since the schedule was made: run without it.
			opts.claude.BaselineID, opts.claude.BaselineType, opts.claude.CompareBaselines = "", "", false
			baselines, lerr = nil, nil
		}
		if lerr != nil {
			return "", lerr
		}
	}
	if job.Provider == "image" && job.VerifyCredentialID == "" {
		opts.image.Provenance, opts.image.Baseline = false, false
	}
	release, ok := s.bgSlot(job.Provider)
	if !ok {
		return "", &leaseError{http.StatusTooManyRequests, "Background checks are busy. Try again in a few minutes"}
	}
	lease, err := s.leaseCredential(ctx, job.UserID, job.CredentialID, job.Provider, "")
	if err != nil {
		release()
		return "", err
	}
	var verify *keyLease
	if job.VerifyCredentialID != "" {
		if verify, err = s.leaseCredential(ctx, job.UserID, job.VerifyCredentialID, "openai", s.verifyBase()); err != nil {
			lease.finish(s.cfg.Store)
			release()
			return "", err
		}
	}
	client := s.newClient()
	if err := client.ValidateURL(ctx, lease.endpoint); err != nil {
		client.CloseIdleConnections()
		lease.finish(s.cfg.Store)
		verify.finish(s.cfg.Store)
		release()
		return "", &leaseError{http.StatusBadRequest, preflightMessage(err)}
	}
	userID, credID := job.UserID, job.CredentialID
	meta := runMeta{UserID: &userID, CredentialID: &credID}
	if job.ScheduleID != "" {
		scheduleID := job.ScheduleID
		meta.ScheduleID = &scheduleID
	}
	started := make(chan string, 1)
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		defer release()
		defer client.CloseIdleConnections()
		defer lease.finish(s.cfg.Store)
		defer verify.finish(s.cfg.Store)
		var runID string
		notify := func(id string) {
			if runID == "" {
				runID = id
				started <- id
			}
		}
		run := backgroundRun{s: s, job: job, opts: opts, baselines: baselines, client: client, lease: lease, verify: verify, meta: meta, notify: notify}
		run.exec()
		if runID == "" {
			close(started)
		}
		if done != nil {
			done(runID)
		}
	}()
	select {
	case id, ok := <-started:
		if !ok || id == "" {
			return "", &leaseError{http.StatusServiceUnavailable, "Could not save check history. No upstream requests were sent."}
		}
		return id, nil
	case <-time.After(30 * time.Second):
		return "", nil // still preparing; it will appear in the records list
	}
}

type backgroundRun struct {
	s         *Server
	job       checkJob
	opts      jobOptions
	baselines []claudecheck.ComparisonBaseline
	client    *ssrf.Client
	lease     *keyLease
	verify    *keyLease
	meta      runMeta
	notify    func(id string)
}

func (b *backgroundRun) exec() {
	s, endpoint, key := b.s, b.lease.endpoint, b.lease.secret
	site := siteinfo.Fetch(b.s.bgCtx, b.client.Client, b.client.ValidateURL, endpoint)
	b.lease.watch.wrap(b.client.Client)
	remark := strings.TrimSpace(b.job.Remark)
	if !utf8.ValidString(remark) || utf8.RuneCountInString(remark) > 200 {
		remark = ""
	}
	switch b.job.Provider {
	case "claude":
		ctx, cancel := context.WithTimeout(s.bgCtx, claudecheck.RunTimeout)
		defer cancel()
		transport := newClaudeTransport(b.client.Client, endpoint, key)
		history := &claudeHistory{store: s.cfg.Store, owner: b.job.OwnerID, meta: b.meta, redact: transport.redactJSON, siteDesc: site.Description}
		defer history.finalizeInterrupted()
		claudecheck.RunWithObserver(ctx, b.opts.claude, transport.call, func(event claudecheck.Event) {
			if event.Report != nil {
				if event.Type == "done" && history.checkpointErr != nil {
					event.Report.StopReason = "history_unavailable"
				}
				event.Report.Baselines = b.baselines
				event.Report.Transport = claudeTransportName
				event.Report.ChannelName = site.Name
				if remark != "" {
					r := remark
					event.Report.Remark = &r
				}
				if u, err := url.Parse(endpoint); err == nil {
					u.User, u.RawQuery, u.Fragment = nil, "", ""
					event.Report.Endpoint = u.String()
				}
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				history.interrupted = true
			}
			if err := history.observe(event, event.Type == "start" || event.Type == "done" || ctx.Err() == nil); err != nil {
				cancel()
			}
			if event.Type == "start" && history.created {
				b.notify(event.Report.ID)
			}
		})
	case "openai":
		ctx, cancel := context.WithTimeout(s.bgCtx, openaicheck.RunTimeout)
		defer cancel()
		redactor := openaicheck.NewRedactor(key)
		transport := openaicheck.NewHTTPTransport(b.client.Client, endpoint, key, redactor)
		history := &openAIHistory{store: s.cfg.Store, owner: b.job.OwnerID, meta: b.meta, redactor: redactor, siteName: site.Name, siteDesc: site.Description}
		defer history.finalizeInterrupted()
		openaicheck.RunWithObserver(ctx, b.opts.openai, transport, func(event openaicheck.Event) {
			if event.Report != nil {
				event.Report.Endpoint, event.Report.Remark = endpoint, remark
			}
			switch event.Type {
			case "start", "done", "sample", "check", "probe_start":
				_ = history.observe(event)
			}
			if event.Type == "start" {
				if history.createErr != nil {
					cancel()
				} else {
					b.notify(event.Report.ID)
				}
			}
		})
	case "gemini":
		ctx, cancel := context.WithTimeout(s.bgCtx, geminicheck.RunTimeout)
		defer cancel()
		redactor := openaicheck.NewRedactor(key)
		transport := geminicheck.NewHTTPTransport(b.client.Client, endpoint, key, redactor)
		history := &geminiHistory{store: s.cfg.Store, owner: b.job.OwnerID, meta: b.meta, redactor: redactor, siteName: site.Name, siteDesc: site.Description}
		defer history.finalizeInterrupted()
		geminicheck.RunWithObserver(ctx, b.opts.gemini, transport, func(event geminicheck.Event) {
			if event.Report != nil {
				event.Report.Endpoint, event.Report.Remark = endpoint, remark
			}
			switch event.Type {
			case "start", "done", "sample", "check", "probe_start":
				_ = history.observe(event)
			}
			if event.Type == "start" {
				if history.createErr != nil {
					cancel()
				} else {
					b.notify(event.Report.ID)
				}
			}
		})
	case "image":
		ctx, cancel := context.WithTimeout(s.bgCtx, imagecheck.RunTimeout)
		defer cancel()
		input := imagecheck.Input{Options: b.opts.image, BaseURL: endpoint, Key: key, Remark: remark}
		if b.verify != nil {
			input.VerifyKey = b.verify.secret
		}
		if _, err := input.Normalize(); err != nil {
			return
		}
		redactor := openaicheck.NewRedactor(input.Key, input.VerifyKey)
		transport, deps := imagecheck.Build(imagecheck.Plumbing{Client: b.client.Client, ValidateURL: b.client.ValidateURL,
			Endpoint: endpoint, OfficialBase: s.cfg.VerifyBaseURL, Input: input, Redactor: redactor})
		history := &imageHistory{store: s.cfg.Store, owner: b.job.OwnerID, meta: b.meta, redactor: redactor, siteName: site.Name, siteDesc: site.Description}
		defer history.finalizeInterrupted()
		imagecheck.Run(ctx, input.Options, transport, deps, func(event imagecheck.Event) {
			if event.Report != nil {
				event.Report.Endpoint, event.Report.Remark = endpoint, remark
			}
			switch event.Type {
			case "start", "done", "sample", "check", "probe_start":
				_ = history.observe(event)
			}
			if event.Type == "start" {
				if history.createErr != nil {
					cancel()
				} else {
					b.notify(event.Report.ID)
				}
			}
		})
	}
}

// ---- HTTP ----

// retest starts a background check with a saved key: the one-click retest.
func (s *Server) retest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	var in struct {
		Provider           string          `json:"provider"`
		CredentialID       string          `json:"credential_id"`
		VerifyCredentialID string          `json:"verify_credential_id"`
		Model              string          `json:"model"`
		Options            json.RawMessage `json:"options"`
		Remark             string          `json:"remark"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if common.DecodeJson(c.Request.Body, &in) != nil || in.CredentialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	id, err := s.startJob(checkJob{Provider: in.Provider, Model: in.Model, Options: in.Options, Remark: in.Remark,
		OwnerID: ownerID(c), UserID: u.ID, CredentialID: in.CredentialID, VerifyCredentialID: in.VerifyCredentialID}, nil)
	if err != nil {
		var le *leaseError
		if errors.As(err, &le) {
			c.JSON(le.status, gin.H{"success": false, "message": le.message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"run_id": id}})
}

// runningJobs lists the account's checks that are still running, including
// scheduled ones started while no page was open.
func (s *Server) runningJobs(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	runs, err := s.cfg.Store.RunningRuns(c.Request.Context(), u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to load check history"})
		return
	}
	type item struct {
		ID          string `json:"id"`
		Model       string `json:"model"`
		Transport   string `json:"transport"`
		Endpoint    string `json:"endpoint"`
		StartedAt   int64  `json:"started_at"`
		ActiveProbe string `json:"active_probe"`
		Scheduled   bool   `json:"scheduled"`
	}
	items := make([]item, 0, len(runs))
	for _, r := range runs {
		items = append(items, item{ID: r.ID, Model: r.ModelName, Transport: r.Transport, Endpoint: r.Endpoint,
			StartedAt: r.StartedAt, ActiveProbe: r.ActiveProbe, Scheduled: r.ScheduleID != nil})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}
