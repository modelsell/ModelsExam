package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"model-check/common"
	"model-check/internal/store"
)

var scheduleIntervals = map[int]bool{30: true, 60: true, 180: true, 360: true, 720: true, 1440: true}

// jitter spreads runs by ±10% of the interval so schedules made at the same
// time do not hit an endpoint together.
func jitter(interval time.Duration) time.Duration {
	return time.Duration(float64(interval) * (0.9 + 0.2*rand.Float64()))
}

type scheduleView struct {
	store.Schedule
	Options        json.RawMessage `json:"options"`
	CredentialHint string          `json:"credential_hint"`
	CredentialName string          `json:"credential_name"`
	BaseURL        string          `json:"base_url"`
}

func (s *Server) scheduleViews(ctx context.Context, userID int64, rows []store.Schedule) ([]scheduleView, error) {
	creds, err := s.cfg.Store.ListCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	byID := map[string]store.Credential{}
	for _, c := range creds {
		byID[c.ID] = c
	}
	out := make([]scheduleView, 0, len(rows))
	for _, r := range rows {
		v := scheduleView{Schedule: r, Options: json.RawMessage(r.OptionsJSON)}
		if len(v.Options) == 0 {
			v.Options = json.RawMessage("{}")
		}
		if c, ok := byID[r.CredentialID]; ok {
			v.CredentialHint, v.CredentialName, v.BaseURL = c.Hint, c.Name, c.BaseURL
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Server) listSchedules(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	rows, err := s.cfg.Store.ListSchedules(c.Request.Context(), u.ID)
	var views []scheduleView
	if err == nil {
		views, err = s.scheduleViews(c.Request.Context(), u.ID, rows)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not load schedules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": views})
}

func (s *Server) createSchedule(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	var in struct {
		CredentialID    string          `json:"credential_id"`
		Model           string          `json:"model"`
		Options         json.RawMessage `json:"options"`
		IntervalMinutes int             `json:"interval_minutes"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if common.DecodeJson(c.Request.Body, &in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	if !scheduleIntervals[in.IntervalMinutes] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Choose an interval of 30 minutes to 24 hours"})
		return
	}
	cred, err := s.cfg.Store.GetCredential(c.Request.Context(), u.ID, in.CredentialID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Saved key not found"})
		return
	}
	opts, normalized, err := parseJobOptions(cred.Provider, in.Model, in.Options)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if cred.Provider == "claude" {
		if _, lerr := s.resolveBaselines(c.Request.Context(), &opts.claude); lerr != nil {
			c.JSON(lerr.status, gin.H{"success": false, "message": lerr.message})
			return
		}
	}
	now := s.now()
	row := &store.Schedule{ID: uuid.NewString(), UserID: u.ID, CredentialID: cred.ID, Provider: cred.Provider, Model: in.Model,
		OptionsJSON: string(normalized), IntervalMinutes: in.IntervalMinutes, Enabled: true,
		// The first run starts about a minute later, so the user sees it work.
		NextRunAt: now.Add(time.Minute).UnixMilli(), CreatedAt: now.UnixMilli()}
	if err := s.cfg.Store.CreateSchedule(c.Request.Context(), row); err != nil {
		if errors.Is(err, store.ErrLimit) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "An account can have at most 10 schedules"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not save the schedule"})
		return
	}
	views, err := s.scheduleViews(c.Request.Context(), u.ID, []store.Schedule{*row})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not save the schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": views[0]})
}

func (s *Server) updateSchedule(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	var in struct {
		Enabled         bool `json:"enabled"`
		IntervalMinutes int  `json:"interval_minutes"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if common.DecodeJson(c.Request.Body, &in) != nil || !scheduleIntervals[in.IntervalMinutes] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Choose an interval of 30 minutes to 24 hours"})
		return
	}
	next := s.now().Add(jitter(time.Duration(in.IntervalMinutes) * time.Minute)).UnixMilli()
	err := s.cfg.Store.UpdateScheduleSettings(c.Request.Context(), u.ID, c.Param("id"), in.Enabled, in.IntervalMinutes, next)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Schedule not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not save the schedule"})
		return
	}
	row, err := s.cfg.Store.GetSchedule(c.Request.Context(), c.Param("id"))
	var views []scheduleView
	if err == nil {
		views, err = s.scheduleViews(c.Request.Context(), u.ID, []store.Schedule{*row})
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not save the schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": views[0]})
}

func (s *Server) deleteSchedule(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	err := s.cfg.Store.DeleteSchedule(c.Request.Context(), u.ID, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Schedule not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not delete the schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) listScheduleRuns(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	row, err := s.cfg.Store.GetSchedule(c.Request.Context(), c.Param("id"))
	if err != nil || row.UserID != u.ID {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Schedule not found"})
		return
	}
	runs, err := s.cfg.Store.ListScheduleRuns(c.Request.Context(), row.ID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not load schedules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": runs})
}

// ---- background loop ----

// Start runs the scheduler and the cleanup of expired keys until Close.
func (s *Server) Start() {
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		lastCleanup := time.Time{}
		for {
			if time.Since(lastCleanup) >= time.Hour {
				s.cleanup()
				lastCleanup = time.Now()
			}
			s.runDueSchedules()
			select {
			case <-s.bgCtx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

// Close stops background work and waits (bounded) for running checks to
// record their interrupted state.
func (s *Server) Close() {
	s.bgCancel()
	finished := make(chan struct{})
	go func() { s.bgWG.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
	}
}

func (s *Server) cleanup() {
	ctx, cancel := context.WithTimeout(s.bgCtx, time.Minute)
	defer cancel()
	if n, err := s.cfg.Store.DeleteExpiredCredentials(ctx, s.now()); err != nil {
		log.Printf("delete expired keys: %v", err)
	} else if n > 0 {
		log.Printf("deleted %d expired saved keys", n)
	}
	if err := s.cfg.Store.PruneAttempts(ctx, s.now()); err != nil {
		log.Printf("prune login attempts: %v", err)
	}
}

// runDueSchedules starts every due schedule that has a free background slot
// and whose key is not busy. A schedule that cannot start now stays due and
// is retried on the next tick.
func (s *Server) runDueSchedules() {
	ctx, cancel := context.WithTimeout(s.bgCtx, 30*time.Second)
	defer cancel()
	now := s.now()
	due, err := s.cfg.Store.DueSchedules(ctx, now, 20)
	if err != nil {
		log.Printf("load due schedules: %v", err)
		return
	}
	for _, sc := range due {
		if _, busy := s.credBusy.Load(sc.CredentialID); busy || len(s.bgSlots[sc.Provider]) == cap(s.bgSlots[sc.Provider]) {
			continue
		}
		next := now.Add(jitter(time.Duration(sc.IntervalMinutes) * time.Minute)).UnixMilli()
		claimed, err := s.cfg.Store.ClaimSchedule(ctx, sc.ID, sc.NextRunAt, next)
		if err != nil || !claimed {
			continue
		}
		s.startScheduled(sc, now)
	}
}

func (s *Server) startScheduled(sc store.Schedule, now time.Time) {
	record := func(runID, status, message string, disable bool) {
		row := &store.ScheduleRun{ScheduleID: sc.ID, RunID: runID, StartedAt: now.UnixMilli(), FinishedAt: s.now().UnixMilli(), Status: status, Error: message}
		if runID != "" {
			if run, err := s.cfg.Store.GetRun(context.Background(), runID); err == nil {
				row.Status, row.Score = run.Status, run.Score
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.cfg.Store.FinishScheduleRun(ctx, row, disable); err != nil {
			log.Printf("record schedule run: %v", err)
		}
	}
	_, err := s.startJob(checkJob{Provider: sc.Provider, Model: sc.Model, Options: json.RawMessage(sc.OptionsJSON),
		UserID: sc.UserID, CredentialID: sc.CredentialID, ScheduleID: sc.ID}, func(runID string) {
		if runID == "" {
			record("", "failed", "The check could not start", false)
			return
		}
		record(runID, "", "", false)
	})
	if err != nil {
		var le *leaseError
		message := err.Error()
		disable := false
		if errors.As(err, &le) {
			message = le.message
			// A key that is gone, expired or paused will not recover by itself.
			disable = le.status == http.StatusNotFound || le.status == http.StatusGone || le.status == http.StatusConflict && le.message != "A check is already running with this saved key"
		}
		record("", "skipped", message, disable)
	}
}
