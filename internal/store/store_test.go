package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newRun(owner, status string) *Run {
	now := time.Now().UnixMilli()
	return &Run{ID: uuid.NewString(), OwnerID: owner, ModelName: "claude-x", Status: status, StartedAt: now, UpdatedAt: now,
		ReportJSON: `{"id":"x","remark":"old"}`}
}

func TestRunLifecycleAndOwnerList(t *testing.T) {
	s, ctx := open(t), context.Background()
	a, b := newRun("owner-a", "running"), newRun("owner-b", "completed")
	for _, r := range []*Run{a, b} {
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	// Each browser lists only its own reports; no owner lists nothing.
	rows, total, err := s.ListRuns(ctx, ListQuery{OwnerID: "owner-a", Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != a.ID {
		t.Fatalf("list: %v total=%d rows=%d", err, total, len(rows))
	}
	if rows, total, err := s.ListRuns(ctx, ListQuery{Page: 1, PageSize: 20}); err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("list without owner: %v total=%d rows=%d", err, total, len(rows))
	}
	// Finalize the running row; a late checkpoint afterwards must be rejected.
	a.Status = "completed"
	if err := s.UpdateRun(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.Status = "running"
	if err := s.UpdateRun(ctx, a); err == nil {
		t.Fatal("a finished run must not be overwritten")
	}
}

func TestStaleRunsBecomeInterrupted(t *testing.T) {
	s, ctx := open(t), context.Background()
	r := newRun("o", "running")
	r.UpdatedAt = time.Now().Add(-10 * time.Minute).UnixMilli()
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, r.ID)
	if err != nil || got.Status != "interrupted" {
		t.Fatalf("status=%v err=%v", got, err)
	}
}

func TestRemarkOwnership(t *testing.T) {
	s, ctx := open(t), context.Background()
	r := newRun("owner-a", "completed")
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateRemark(ctx, "owner-b", r.ID, "hijack"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other browsers must not edit: %v", err)
	}
	got, err := s.UpdateRemark(ctx, "owner-a", r.ID, "new")
	if err != nil || got.Remark == nil || *got.Remark != "new" {
		t.Fatalf("owner edit failed: %v", err)
	}
	if _, err := s.UpdateRemark(ctx, "owner-a", uuid.NewString(), "x"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing report: %v", err)
	}
	running := newRun("owner-a", "running")
	_ = s.CreateRun(ctx, running)
	if _, err := s.UpdateRemark(ctx, "owner-a", running.ID, "x"); !errors.Is(err, ErrRunning) {
		t.Fatalf("running report: %v", err)
	}
}

func TestBaselineIsIdempotentPerReport(t *testing.T) {
	s, ctx := open(t), context.Background()
	first := &Baseline{ID: uuid.NewString(), ReportID: "r1", Type: "official", SnapshotJSON: `{}`, CreatedAt: 1}
	if err := s.CreateBaseline(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := &Baseline{ID: uuid.NewString(), ReportID: "r1", Type: "other", SnapshotJSON: `{}`, CreatedAt: 2}
	if err := s.CreateBaseline(ctx, second); err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Type != "official" {
		t.Fatal("the original baseline must be returned for the same report")
	}
	rows, _ := s.ListBaselines(ctx)
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
}

func TestListRunsShowsOnlyLatest100(t *testing.T) {
	s, ctx := open(t), context.Background()
	base := time.Now().UnixMilli()
	for i := 0; i < 120; i++ {
		r := newRun("o", "completed")
		r.StartedAt = base + int64(i)
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	seen, newest := 0, int64(0)
	for page := 1; page <= 6; page++ {
		rows, total, err := s.ListRuns(ctx, ListQuery{OwnerID: "o", Page: page, PageSize: 20})
		if err != nil || total != 100 {
			t.Fatalf("page %d: total=%d err=%v", page, total, err)
		}
		for _, r := range rows {
			if page == 1 && newest == 0 {
				newest = r.StartedAt
			}
			if r.StartedAt < base+20 {
				t.Fatalf("a record outside the latest 100 was listed: %d", r.StartedAt-base)
			}
		}
		seen += len(rows)
	}
	if seen != 100 || newest != base+119 {
		t.Fatalf("seen=%d newest=%d", seen, newest-base)
	}
}
