package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Accounts are optional. A visitor without one keeps the per-browser owner ID
// described in store.go; an account adds a login, saved keys and schedules.

type User struct {
	ID                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Username          string `json:"username" gorm:"size:32"`
	UsernameLower     string `json:"-" gorm:"size:32;uniqueIndex"`
	PasswordHash      string `json:"-" gorm:"size:100"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime:false"`
	PasswordChangedAt int64  `json:"password_changed_at"`
	FailedLogins      int    `json:"-"`
	LockedUntil       int64  `json:"-"`
	RegisterIP        string `json:"-" gorm:"size:64;index"`
}

func (User) TableName() string { return "users" }

// Session stores only the SHA-256 of the cookie token.
type Session struct {
	TokenHash string `gorm:"primaryKey;size:64"`
	UserID    int64  `gorm:"index"`
	CreatedAt int64  `gorm:"autoCreateTime:false"`
	ExpiresAt int64  `gorm:"index"`
	IP        string `gorm:"size:64"`
	UserAgent string `gorm:"size:200"`
}

func (Session) TableName() string { return "sessions" }

// LoginAttempt records logins and registrations for rate limiting; rows are
// kept for 30 days.
type LoginAttempt struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Kind      string `gorm:"size:16;index:idx_attempt_ip,priority:1"` // login | register
	IP        string `gorm:"size:64;index:idx_attempt_ip,priority:2"`
	Username  string `gorm:"size:32"`
	Success   bool
	CreatedAt int64 `gorm:"autoCreateTime:false;index:idx_attempt_ip,priority:3;index"`
}

func (LoginAttempt) TableName() string { return "login_attempts" }

// OwnerClaim recorded browser owner IDs whose anonymous runs were moved into
// an account. Moving guest runs into accounts was withdrawn; the table is kept
// (schema changes are add-only) and no longer written.
type OwnerClaim struct {
	OwnerID   string `gorm:"primaryKey;size:36"`
	UserID    int64  `gorm:"index"`
	ClaimedAt int64  `gorm:"autoCreateTime:false"`
}

func (OwnerClaim) TableName() string { return "owner_claims" }

// Credential is an API key saved by a user. Secret is encrypted at rest
// (AES-256-GCM, see internal/secretbox; the master key is not in the
// database) and is write-only: it has no JSON name, list and detail queries
// omit the column, and only CredentialSecret decrypts it, for the check runner.
type Credential struct {
	ID             string `json:"id" gorm:"primaryKey;size:36"`
	UserID         int64  `json:"-" gorm:"index"`
	Name           string `json:"name" gorm:"size:64"`
	Provider       string `json:"provider" gorm:"size:16"`
	BaseURL        string `json:"base_url" gorm:"type:text"`
	Secret         string `json:"-" gorm:"type:text"`
	Hint           string `json:"hint" gorm:"size:64"`
	ExpiresAt      int64  `json:"expires_at" gorm:"index"`
	AcknowledgedAt int64  `json:"acknowledged_at"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime:false"`
	LastUsedAt     int64  `json:"last_used_at"`
	UseCount       int    `json:"use_count"`
	PausedReason   string `json:"paused_reason" gorm:"size:64"`
	AuthFailures   int    `json:"-"`
}

func (Credential) TableName() string { return "credentials" }

// Schedule runs one check configuration with a saved key at a fixed interval.
type Schedule struct {
	ID              string `json:"id" gorm:"primaryKey;size:36"`
	UserID          int64  `json:"-" gorm:"index"`
	CredentialID    string `json:"credential_id" gorm:"size:36;index"`
	Provider        string `json:"provider" gorm:"size:16"`
	Model           string `json:"model" gorm:"size:200"`
	OptionsJSON     string `json:"-" gorm:"type:text"`
	IntervalMinutes int    `json:"interval_minutes"`
	Enabled         bool   `json:"enabled"`
	NextRunAt       int64  `json:"next_run_at" gorm:"index"`
	LastRunAt       int64  `json:"last_run_at"`
	LastRunID       string `json:"last_run_id" gorm:"size:36"`
	LastStatus      string `json:"last_status" gorm:"size:20"`
	LastScore       *int   `json:"last_score"`
	LastError       string `json:"last_error" gorm:"size:200"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime:false"`
}

func (Schedule) TableName() string { return "schedules" }

type ScheduleRun struct {
	ID         int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	ScheduleID string `json:"schedule_id" gorm:"size:36;index"`
	RunID      string `json:"run_id" gorm:"size:36"`
	StartedAt  int64  `json:"started_at" gorm:"autoCreateTime:false"`
	FinishedAt int64  `json:"finished_at"`
	Status     string `json:"status" gorm:"size:20"`
	Score      *int   `json:"score"`
	Error      string `json:"error" gorm:"size:200"`
}

func (ScheduleRun) TableName() string { return "schedule_runs" }

var (
	ErrUsernameTaken = errors.New("username taken")
	ErrLimit         = errors.New("limit reached")
	ErrNotFound      = gorm.ErrRecordNotFound
)

const (
	MaxCredentialsPerUser = 20
	MaxSchedulesPerUser   = 10
	MaxChecksPerKeyPerDay = 50
	// UserRecordLimit caps an account's record list, which grows with schedules.
	UserRecordLimit  = 1000
	attemptRetention = 30 * 24 * time.Hour
)

// ---- users ----

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	u.UsernameLower = strings.ToLower(u.Username)
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(u)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrUsernameTaken
	}
	return nil
}

func (s *Store) UserByName(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("username_lower = ?", strings.ToLower(username)).First(&u).Error
	return &u, err
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	return &u, err
}

// RecordLoginFailure counts a wrong password and returns the new lock time
// (0 when not locked). Five consecutive failures lock the account for 15
// minutes; every further failure doubles it, up to 24 hours.
func (s *Store) RecordLoginFailure(ctx context.Context, userID int64, now time.Time) (int64, error) {
	var lockedUntil int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u User
		if err := tx.Where("id = ?", userID).First(&u).Error; err != nil {
			return err
		}
		u.FailedLogins++
		if d := LockDuration(u.FailedLogins); d > 0 {
			lockedUntil = now.Add(d).UnixMilli()
		}
		return tx.Model(&User{}).Where("id = ?", userID).
			Updates(map[string]any{"failed_logins": u.FailedLogins, "locked_until": lockedUntil}).Error
	})
	return lockedUntil, err
}

// LockDuration is the lock applied after the given number of consecutive failures.
func LockDuration(failures int) time.Duration {
	if failures < 5 {
		return 0
	}
	d := 15 * time.Minute
	for i := 5; i < failures && d < 24*time.Hour; i++ {
		d *= 2
	}
	return min(d, 24*time.Hour)
}

func (s *Store) ResetLoginFailures(ctx context.Context, userID int64) error {
	return s.db.WithContext(ctx).Model(&User{}).Where("id = ?", userID).
		Updates(map[string]any{"failed_logins": 0, "locked_until": 0}).Error
}

// SetPassword replaces the hash and ends every session except keep (may be "").
func (s *Store) SetPassword(ctx context.Context, userID int64, hash, keep string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
			"password_hash": hash, "password_changed_at": time.Now().UnixMilli(), "failed_logins": 0, "locked_until": 0})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Where("user_id = ? AND token_hash <> ?", userID, keep).Delete(&Session{}).Error
	})
}

// ---- attempts ----

func (s *Store) AddAttempt(ctx context.Context, a *LoginAttempt) error {
	return s.db.WithContext(ctx).Create(a).Error
}

func (s *Store) CountAttempts(ctx context.Context, kind, ip string, since time.Time, successOnly bool) (int64, error) {
	var n int64
	tx := s.db.WithContext(ctx).Model(&LoginAttempt{}).Where("kind = ? AND ip = ? AND created_at >= ?", kind, ip, since.UnixMilli())
	if successOnly {
		tx = tx.Where("success = ?", true)
	}
	err := tx.Count(&n).Error
	return n, err
}

// ---- sessions ----

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	return s.db.WithContext(ctx).Create(sess).Error
}

// SessionUser returns the user of a live session.
func (s *Store) SessionUser(ctx context.Context, tokenHash string, now time.Time) (*User, error) {
	var sess Session
	if err := s.db.WithContext(ctx).Where("token_hash = ? AND expires_at > ?", tokenHash, now.UnixMilli()).First(&sess).Error; err != nil {
		return nil, err
	}
	return s.UserByID(ctx, sess.UserID)
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return s.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Delete(&Session{}).Error
}

// ---- credentials ----

// credentialColumns is every column except the secret.
var credentialColumns = []string{"id", "user_id", "name", "provider", "base_url", "hint", "expires_at", "acknowledged_at",
	"created_at", "last_used_at", "use_count", "paused_reason", "auth_failures"}

// CreateCredential encrypts c.Secret before it is written.
func (s *Store) CreateCredential(ctx context.Context, c *Credential) error {
	if s.box == nil {
		return ErrNoSecretKey
	}
	sealed, err := s.box.Seal(c.Secret, secretAAD(c.ID, c.UserID))
	if err != nil {
		return err
	}
	row := *c
	row.Secret = sealed
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&Credential{}).Where("user_id = ?", c.UserID).Count(&n).Error; err != nil {
			return err
		}
		if n >= MaxCredentialsPerUser {
			return ErrLimit
		}
		return tx.Create(&row).Error
	})
}

func (s *Store) ListCredentials(ctx context.Context, userID int64) ([]Credential, error) {
	rows := []Credential{}
	err := s.db.WithContext(ctx).Select(credentialColumns).Where("user_id = ?", userID).Order("created_at DESC").Find(&rows).Error
	return rows, err
}

// GetCredential returns a user's credential without its secret.
func (s *Store) GetCredential(ctx context.Context, userID int64, id string) (*Credential, error) {
	var c Credential
	err := s.db.WithContext(ctx).Select(credentialColumns).Where("id = ? AND user_id = ?", id, userID).First(&c).Error
	return &c, err
}

// CredentialSecret is the only read of a saved key, used by the check runner
// right before it sends the key upstream.
func (s *Store) CredentialSecret(ctx context.Context, userID int64, id string) (string, error) {
	if s.box == nil {
		return "", ErrNoSecretKey
	}
	var c Credential
	if err := s.db.WithContext(ctx).Select("secret").Where("id = ? AND user_id = ?", id, userID).First(&c).Error; err != nil {
		return "", err
	}
	return s.box.Open(c.Secret, secretAAD(id, userID))
}

func (s *Store) RenewCredential(ctx context.Context, userID int64, id string, expiresAt int64) error {
	res := s.db.WithContext(ctx).Model(&Credential{}).Where("id = ? AND user_id = ?", id, userID).Update("expires_at", expiresAt)
	if res.Error == nil && res.RowsAffected != 1 {
		return ErrNotFound
	}
	return res.Error
}

// ResumeCredential clears an automatic pause after the user confirmed the key works again.
func (s *Store) ResumeCredential(ctx context.Context, userID int64, id string) error {
	res := s.db.WithContext(ctx).Model(&Credential{}).Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"paused_reason": "", "auth_failures": 0})
	if res.Error == nil && res.RowsAffected != 1 {
		return ErrNotFound
	}
	return res.Error
}

// DeleteCredential hard-deletes a key and every schedule that used it.
func (s *Store) DeleteCredential(ctx context.Context, userID int64, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&Credential{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Where("credential_id = ?", id).Delete(&Schedule{}).Error
	})
}

// DeleteUserCredentials removes all keys (and their schedules) of one user.
func (s *Store) DeleteUserCredentials(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&Schedule{}).Error; err != nil {
			return err
		}
		res := tx.Where("user_id = ?", userID).Delete(&Credential{})
		n = res.RowsAffected
		return res.Error
	})
	return n, err
}

// DeleteAllCredentials is the incident-response purge: every saved key and schedule.
func (s *Store) DeleteAllCredentials(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&Schedule{}).Error; err != nil {
			return err
		}
		res := tx.Where("1 = 1").Delete(&Credential{})
		n = res.RowsAffected
		return res.Error
	})
	return n, err
}

// CredentialUsedToday counts the checks a key started since the UTC day began.
func (s *Store) CredentialUsedToday(ctx context.Context, id string, now time.Time) (int64, error) {
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	var n int64
	err := s.db.WithContext(ctx).Model(&Run{}).Where("credential_id = ? AND started_at >= ?", id, day.UnixMilli()).Count(&n).Error
	return n, err
}

func (s *Store) TouchCredential(ctx context.Context, id string, now time.Time) error {
	return s.db.WithContext(ctx).Model(&Credential{}).Where("id = ?", id).
		Updates(map[string]any{"last_used_at": now.UnixMilli(), "use_count": gorm.Expr("use_count + 1")}).Error
}

// RecordAuthResult stores the consecutive upstream 401/403 count of a key and
// pauses it at three.
func (s *Store) RecordAuthResult(ctx context.Context, id string, failures int) error {
	updates := map[string]any{"auth_failures": failures}
	if failures >= 3 {
		updates["paused_reason"] = "auth_failed"
	}
	return s.db.WithContext(ctx).Model(&Credential{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteExpiredCredentials hard-deletes expired keys and their schedules.
func (s *Store) DeleteExpiredCredentials(ctx context.Context, now time.Time) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := []string{}
		if err := tx.Model(&Credential{}).Where("expires_at <= ?", now.UnixMilli()).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
			return err
		}
		if err := tx.Where("credential_id IN ?", ids).Delete(&Schedule{}).Error; err != nil {
			return err
		}
		res := tx.Where("id IN ?", ids).Delete(&Credential{})
		n = res.RowsAffected
		return res.Error
	})
	return n, err
}

// PruneAttempts drops rate-limit rows older than 30 days and dead sessions.
func (s *Store) PruneAttempts(ctx context.Context, now time.Time) error {
	if err := s.db.WithContext(ctx).Where("created_at < ?", now.Add(-attemptRetention).UnixMilli()).Delete(&LoginAttempt{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("expires_at < ?", now.UnixMilli()).Delete(&Session{}).Error
}

// ---- schedules ----

func (s *Store) CreateSchedule(ctx context.Context, sc *Schedule) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&Schedule{}).Where("user_id = ?", sc.UserID).Count(&n).Error; err != nil {
			return err
		}
		if n >= MaxSchedulesPerUser {
			return ErrLimit
		}
		return tx.Create(sc).Error
	})
}

func (s *Store) ListSchedules(ctx context.Context, userID int64) ([]Schedule, error) {
	rows := []Schedule{}
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (s *Store) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	var sc Schedule
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&sc).Error
	return &sc, err
}

func (s *Store) UpdateScheduleSettings(ctx context.Context, userID int64, id string, enabled bool, interval int, nextRunAt int64) error {
	res := s.db.WithContext(ctx).Model(&Schedule{}).Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"enabled": enabled, "interval_minutes": interval, "next_run_at": nextRunAt})
	if res.Error == nil && res.RowsAffected != 1 {
		return ErrNotFound
	}
	return res.Error
}

func (s *Store) DeleteSchedule(ctx context.Context, userID int64, id string) error {
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&Schedule{})
	if res.Error == nil && res.RowsAffected != 1 {
		return ErrNotFound
	}
	return res.Error
}

// DueSchedules lists enabled schedules whose next run is due.
func (s *Store) DueSchedules(ctx context.Context, now time.Time, limit int) ([]Schedule, error) {
	rows := []Schedule{}
	err := s.db.WithContext(ctx).Where("enabled = ? AND next_run_at <= ?", true, now.UnixMilli()).
		Order("next_run_at ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

// ClaimSchedule moves a due schedule's next run forward. Only the caller whose
// update matched the old time may start the run, so a slow tick can never
// start the same run twice.
func (s *Store) ClaimSchedule(ctx context.Context, id string, oldNext, newNext int64) (bool, error) {
	res := s.db.WithContext(ctx).Model(&Schedule{}).Where("id = ? AND enabled = ? AND next_run_at = ?", id, true, oldNext).
		Update("next_run_at", newNext)
	return res.RowsAffected == 1, res.Error
}

// FinishScheduleRun records the outcome of one scheduled run.
func (s *Store) FinishScheduleRun(ctx context.Context, row *ScheduleRun, disable bool) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		updates := map[string]any{"last_run_at": row.StartedAt, "last_status": row.Status, "last_error": row.Error}
		if row.RunID != "" {
			updates["last_run_id"], updates["last_score"] = row.RunID, row.Score
		}
		if disable {
			updates["enabled"] = false
		}
		return tx.Model(&Schedule{}).Where("id = ?", row.ScheduleID).Updates(updates).Error
	})
}

func (s *Store) ListScheduleRuns(ctx context.Context, scheduleID string, limit int) ([]ScheduleRun, error) {
	rows := []ScheduleRun{}
	err := s.db.WithContext(ctx).Where("schedule_id = ?", scheduleID).Order("started_at DESC, id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// RunningRuns lists an account's checks that are still running, newest first.
func (s *Store) RunningRuns(ctx context.Context, userID int64) ([]Run, error) {
	if err := s.recoverStale(ctx); err != nil {
		return nil, err
	}
	runs := []Run{}
	err := s.db.WithContext(ctx).Omit("report_json").Where("user_id = ? AND status = ?", userID, "running").
		Order("started_at DESC").Limit(20).Find(&runs).Error
	return runs, err
}

// PreviousScores returns, for each run, the score of the newest earlier
// scored run with the same configuration in the same list.
func (s *Store) PreviousScores(ctx context.Context, q ListQuery, runs []Run) (map[string]*int, error) {
	out := map[string]*int{}
	keys := []string{}
	for _, r := range runs {
		if r.ConfigKey != "" {
			keys = append(keys, r.ConfigKey)
		}
	}
	if len(keys) == 0 {
		return out, nil
	}
	tx := s.db.WithContext(ctx).Model(&Run{}).Select("id", "config_key", "score", "started_at").
		Where("config_key IN ? AND score IS NOT NULL AND status IN ?", keys, []string{"completed", "failed"})
	tx = tx.Where("user_id = ?", q.UserID)
	history := []Run{}
	if err := tx.Order("started_at DESC").Limit(2000).Find(&history).Error; err != nil {
		return nil, err
	}
	for _, r := range runs {
		for _, h := range history {
			if h.ConfigKey == r.ConfigKey && h.ID != r.ID && h.StartedAt < r.StartedAt {
				out[r.ID] = h.Score
				break
			}
		}
	}
	return out, nil
}
