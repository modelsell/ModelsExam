// Package store persists check reports and comparison baselines. There are no
// user accounts: every report is public, and a random per-browser owner ID only
// gates remark edits.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"model-check/common"
)

const (
	MaxReportBytes   = 4 << 20
	MaxBaselineBytes = 32 << 10
	BaselineLimit    = 50
	ScoreVersion     = 1
	// PublicRecordLimit caps the public record list; older rows stay in the database.
	PublicRecordLimit = 100
)

var ErrRunning = errors.New("model check is still running")

type Run struct {
	ID          string `json:"id" gorm:"primaryKey;size:36"`
	OwnerID     string `json:"-" gorm:"size:36;index"`
	ModelName   string `json:"model" gorm:"size:200;index"`
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name" gorm:"type:text"`
	// SiteDescription and ChannelName hold the tested site's public description and
	// name, read from its home page when the check starts.
	SiteDescription string  `json:"site_description" gorm:"type:text"`
	Remark          *string `json:"remark,omitempty" gorm:"type:text"`
	Score           *int    `json:"score"`
	ScoreVersion    int     `json:"-" gorm:"not null;default:0"`
	Endpoint        string  `json:"endpoint" gorm:"type:text"`
	Transport       string  `json:"transport" gorm:"size:32"`
	Status          string  `json:"status" gorm:"size:20;index"`
	StartedAt       int64   `json:"started_at" gorm:"index"`
	UpdatedAt       int64   `json:"updated_at" gorm:"autoUpdateTime:false"`
	DurationMS      int64   `json:"duration_ms"`
	RequestCount    int     `json:"request_count"`
	PassCount       int     `json:"pass_count"`
	FailCount       int     `json:"fail_count"`
	ActiveProbe     string  `json:"active_probe" gorm:"size:64"`
	ReportJSON      string  `json:"-" gorm:"size:4194304"`
}

func (Run) TableName() string { return "model_check_runs" }

type Baseline struct {
	ID           string `json:"id" gorm:"primaryKey;size:36"`
	ReportID     string `json:"report_id" gorm:"size:36;uniqueIndex"`
	Type         string `json:"type" gorm:"size:40"`
	ModelName    string `json:"model" gorm:"size:200"`
	ChannelName  string `json:"channel_name" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime:false;index"`
	Fingerprint  bool   `json:"fingerprint"`
	SnapshotJSON string `json:"-" gorm:"type:text"`
}

func (Baseline) TableName() string { return "model_check_baselines" }

type Store struct{ db *gorm.DB }

// Open connects using DSN: empty or a file path selects SQLite, a
// "postgres://" / "postgresql://" URL selects PostgreSQL and anything with an
// "@tcp(" or "mysql://" prefix selects MySQL.
func Open(dsn string) (*Store, error) {
	var dialector gorm.Dialector
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		dialector = postgres.Open(dsn)
	case strings.HasPrefix(dsn, "mysql://"):
		dialector = mysql.Open(strings.TrimPrefix(dsn, "mysql://"))
	case strings.Contains(dsn, "@tcp("):
		dialector = mysql.Open(dsn)
	default:
		if dsn == "" {
			dsn = "data/model-check.db"
		}
		if dir := filepath.Dir(dsn); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		dialector = sqlite.Open(dsn + "?_busy_timeout=5000")
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&Run{}, &Baseline{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (s *Store) CreateRun(ctx context.Context, run *Run) error {
	if run.ID == "" || len(run.ReportJSON) > MaxReportBytes {
		return errors.New("invalid model check history record")
	}
	return s.db.WithContext(ctx).Create(run).Error
}

// UpdateRun applies a checkpoint or the final state. A finished run cannot be
// overwritten by a late checkpoint.
func (s *Store) UpdateRun(ctx context.Context, run *Run) error {
	if len(run.ReportJSON) > MaxReportBytes {
		return errors.New("model check report exceeds size limit")
	}
	res := s.db.WithContext(ctx).Model(&Run{}).Where("id = ? AND status = ?", run.ID, "running").
		Updates(map[string]any{
			"model_name": run.ModelName, "channel_name": run.ChannelName, "site_description": run.SiteDescription, "endpoint": run.Endpoint,
			"remark": run.Remark, "score": run.Score, "score_version": run.ScoreVersion,
			"status": run.Status, "updated_at": run.UpdatedAt, "duration_ms": run.DurationMS,
			"request_count": run.RequestCount, "pass_count": run.PassCount, "fail_count": run.FailCount,
			"active_probe": run.ActiveProbe, "report_json": run.ReportJSON,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("model check history record is no longer running")
	}
	return nil
}

func (s *Store) recoverStale(ctx context.Context) error {
	// A probe takes at most 90s; five minutes without a checkpoint means the
	// process died mid-run.
	return s.db.WithContext(ctx).Model(&Run{}).
		Where("status = ? AND updated_at < ?", "running", time.Now().Add(-5*time.Minute).UnixMilli()).
		Updates(map[string]any{"status": "interrupted", "active_probe": ""}).Error
}

type ListQuery struct {
	ModelName string
	Status    string
	Page      int
	PageSize  int
}

func (s *Store) ListRuns(ctx context.Context, q ListQuery) ([]Run, int64, error) {
	if err := s.recoverStale(ctx); err != nil {
		return nil, 0, err
	}
	tx := s.db.WithContext(ctx).Model(&Run{})
	if q.ModelName != "" {
		tx = tx.Where("model_name LIKE ?", "%"+q.ModelName+"%")
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// Only the most recent PublicRecordLimit records are ever shown.
	if total > PublicRecordLimit {
		total = PublicRecordLimit
	}
	page, size := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	if int64(offset) >= total {
		return []Run{}, total, nil
	}
	if remaining := int(total) - offset; size > remaining {
		size = remaining
	}
	runs := []Run{}
	err := tx.Omit("report_json").Order("started_at DESC, id DESC").Offset(offset).Limit(size).Find(&runs).Error
	return runs, total, err
}

func (s *Store) GetRun(ctx context.Context, id string) (*Run, error) {
	if err := s.recoverStale(ctx); err != nil {
		return nil, err
	}
	var run Run
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// UpdateRemark preserves unknown report fields. Only the browser that started
// the run (matching owner ID) may edit it.
func (s *Store) UpdateRemark(ctx context.Context, ownerID, id, remark string) (*Run, error) {
	if ownerID == "" || id == "" || !utf8.ValidString(remark) || utf8.RuneCountInString(remark) > 200 {
		return nil, errors.New("invalid model check remark")
	}
	var run Run
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx
		if tx.Dialector.Name() != "sqlite" {
			q = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Where("id = ?", id).First(&run).Error; err != nil {
			return err
		}
		if run.OwnerID != ownerID {
			return ErrForbidden
		}
		if run.Status == "running" {
			return ErrRunning
		}
		var doc map[string]json.RawMessage
		if err := common.UnmarshalJsonStr(run.ReportJSON, &doc); err != nil || doc == nil {
			return errors.New("invalid model check report")
		}
		value, err := common.Marshal(remark)
		if err != nil {
			return err
		}
		doc["remark"] = value
		data, err := common.Marshal(doc)
		if err != nil {
			return err
		}
		if len(data) > MaxReportBytes {
			return errors.New("model check report exceeds size limit")
		}
		res := tx.Model(&Run{}).Where("id = ? AND status <> ?", id, "running").
			Updates(map[string]any{"remark": remark, "report_json": string(data)})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrRunning
		}
		run.Remark, run.ReportJSON = &remark, string(data)
		return nil
	})
	return &run, err
}

var ErrForbidden = errors.New("not the owner of this report")

// CreateBaseline is idempotent per report: repeated clicks return the existing
// snapshot so a historical comparison cannot be silently replaced.
func (s *Store) CreateBaseline(ctx context.Context, b *Baseline) error {
	if b.ID == "" || b.ReportID == "" || len(b.SnapshotJSON) == 0 || len(b.SnapshotJSON) > MaxBaselineBytes {
		return errors.New("invalid model check baseline")
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(b).Error; err != nil {
		return err
	}
	var saved Baseline
	if err := s.db.WithContext(ctx).Where("report_id = ?", b.ReportID).First(&saved).Error; err != nil {
		return err
	}
	*b = saved
	return nil
}

func (s *Store) ListBaselines(ctx context.Context) ([]Baseline, error) {
	rows := []Baseline{}
	err := s.db.WithContext(ctx).Omit("snapshot_json").Order("created_at DESC, id DESC").Find(&rows).Error
	return rows, err
}

func (s *Store) LoadBaselines(ctx context.Context) ([]Baseline, error) {
	rows := []Baseline{}
	err := s.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(BaselineLimit).Find(&rows).Error
	return rows, err
}

func (s *Store) GetBaseline(ctx context.Context, id string) (*Baseline, error) {
	var row Baseline
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	return &row, err
}

// ListIndexableRuns returns completed runs that have a site name, newest
// first, for the sitemap and the public record lists. Only the columns those
// need are filled.
func (s *Store) ListIndexableRuns(ctx context.Context, limit int) ([]Run, error) {
	runs := []Run{}
	err := s.db.WithContext(ctx).Select("id", "updated_at", "started_at", "channel_name", "site_description", "model_name", "endpoint", "transport", "score").
		Where("status = ? AND channel_name <> ''", "completed").
		Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// ListCompletedByDomain returns the newest completed, scored runs whose
// endpoint mentions domain. The match is a coarse SQL filter; the caller
// checks the endpoint host exactly.
func (s *Store) ListCompletedByDomain(ctx context.Context, domain string, limit int) ([]Run, error) {
	runs := []Run{}
	err := s.db.WithContext(ctx).Select("id", "model_name", "transport", "endpoint", "score", "started_at").
		Where("status = ? AND score IS NOT NULL AND (endpoint LIKE ? OR endpoint LIKE ?)", "completed", "%://"+domain+"%", "%."+domain+"%").
		Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// indexable limits a query to runs that may be listed publicly and indexed:
// completed, with a site name that was read from the tested site.
func indexable(tx *gorm.DB) *gorm.DB {
	return tx.Where("status = ? AND channel_name <> ''", "completed")
}

// ListIndexablePage returns one page (1-based) of indexable runs, newest
// first, with the total count. Report bodies are not loaded.
func (s *Store) ListIndexablePage(ctx context.Context, page, size int) ([]Run, int64, error) {
	tx := indexable(s.db.WithContext(ctx).Model(&Run{}))
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	runs := []Run{}
	err := tx.Omit("report_json").Order("started_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&runs).Error
	return runs, total, err
}

// ListIndexableByModel returns indexable runs of exactly one model name.
func (s *Store) ListIndexableByModel(ctx context.Context, model string, limit int) ([]Run, error) {
	runs := []Run{}
	err := indexable(s.db.WithContext(ctx).Model(&Run{})).Where("model_name = ?", model).
		Omit("report_json").Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// ListIndexableBySite returns indexable runs whose endpoint mentions host. The
// match is a coarse SQL filter; the caller checks the endpoint host exactly.
func (s *Store) ListIndexableBySite(ctx context.Context, host string, limit int) ([]Run, error) {
	runs := []Run{}
	err := indexable(s.db.WithContext(ctx).Model(&Run{})).Where("endpoint LIKE ?", "%://"+host+"%").
		Omit("report_json").Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// ModelCount is a model name with how many indexable runs it has.
type ModelCount struct {
	Model string
	N     int64
}

// TopIndexableModels returns the model names with the most indexable runs.
func (s *Store) TopIndexableModels(ctx context.Context, limit int) ([]ModelCount, error) {
	out := []ModelCount{}
	err := indexable(s.db.WithContext(ctx).Model(&Run{})).Where("model_name <> ''").
		Select("model_name AS model, COUNT(*) AS n").Group("model_name").
		Order("n DESC, model_name ASC").Limit(limit).Scan(&out).Error
	return out, err
}

// ListScoredRuns returns the newest completed runs that have a score and a
// site name (the same public-listing rule as the record lists), with
// only the columns the boards and the site directory need.
func (s *Store) ListScoredRuns(ctx context.Context, limit int) ([]Run, error) {
	runs := []Run{}
	err := s.db.WithContext(ctx).Model(&Run{}).
		Select("id", "channel_name", "model_name", "endpoint", "transport", "score", "started_at").
		Where("status = ? AND score IS NOT NULL AND model_name <> '' AND channel_name <> ''", "completed").
		Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// CountCompleted returns how many runs completed.
func (s *Store) CountCompleted(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&Run{}).Where("status = ?", "completed").Count(&n).Error
	return n, err
}
