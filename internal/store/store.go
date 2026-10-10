// Package store persists check reports, comparison baselines and the optional
// accounts (accounts.go). Only an account has a record list: it holds the runs
// started while signed in. A guest run is in no list; the random per-browser
// owner ID only decides who may annotate it. Anyone with a report's ID (its
// share link) may open it, but reports are never listed publicly.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	// UserID is set for runs of a signed-in account (or claimed into one).
	UserID       *int64  `json:"-" gorm:"index"`
	CredentialID *string `json:"-" gorm:"size:36;index"`
	ScheduleID   *string `json:"-" gorm:"size:36;index"`
	// ConfigKey identifies provider, endpoint, model and options, so a run can
	// be compared with the previous run of the same configuration.
	ConfigKey string `json:"-" gorm:"size:64;index"`
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
	sqlitePath := ""
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
		sqlitePath = dsn
	}
	// A missing row (unknown user, expired session) is an answer, not an error to log.
	quiet := logger.New(log.New(os.Stderr, "\r\n", log.LstdFlags), logger.Config{SlowThreshold: 200 * time.Millisecond, LogLevel: logger.Warn, IgnoreRecordNotFoundError: true, Colorful: false})
	db, err := gorm.Open(dialector, &gorm.Config{Logger: quiet})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&Run{}, &Baseline{}, &User{}, &Session{}, &LoginAttempt{}, &OwnerClaim{}, &Credential{}, &Schedule{}, &ScheduleRun{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// The database holds saved API keys in plaintext: only the service user may read it.
	if sqlitePath != "" {
		for _, p := range []string{sqlitePath, sqlitePath + "-wal", sqlitePath + "-shm", sqlitePath + "-journal"} {
			if err := os.Chmod(p, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("chmod database: %w", err)
			}
		}
	}
	st := &Store{db: db}
	if err := st.backfillConfigKeys(context.Background()); err != nil {
		return nil, fmt.Errorf("backfill config keys: %w", err)
	}
	return st, nil
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
	UserID    int64 // required: only this account's runs are listed
	ModelName string
	Status    string
	Page      int
	PageSize  int
}

func (s *Store) ListRuns(ctx context.Context, q ListQuery) ([]Run, int64, error) {
	if err := s.recoverStale(ctx); err != nil {
		return nil, 0, err
	}
	// Only accounts have a record list; guest runs are never listed.
	if q.UserID == 0 {
		return []Run{}, 0, nil
	}
	limit := int64(UserRecordLimit)
	tx := s.db.WithContext(ctx).Model(&Run{}).Where("user_id = ?", q.UserID)
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
	// Only the most recent records of a browser (or account) are shown.
	if total > limit {
		total = limit
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
// the run (matching owner ID) or the account it belongs to may edit it.
func (s *Store) UpdateRemark(ctx context.Context, ownerID string, userID int64, id, remark string) (*Run, error) {
	if (ownerID == "" && userID == 0) || id == "" || !utf8.ValidString(remark) || utf8.RuneCountInString(remark) > 200 {
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
		if !RunOwnedBy(&run, ownerID, userID) {
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

// RunOwnedBy reports whether a browser owner ID or an account owns a run.
func RunOwnedBy(run *Run, ownerID string, userID int64) bool {
	return (ownerID != "" && run.OwnerID == ownerID) || (userID != 0 && run.UserID != nil && *run.UserID == userID)
}

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
