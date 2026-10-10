package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newUser(t *testing.T, s *Store, name string) *User {
	t.Helper()
	u := &User{Username: name, PasswordHash: "x", CreatedAt: time.Now().UnixMilli()}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func newCredential(t *testing.T, s *Store, userID int64, expires time.Time) *Credential {
	t.Helper()
	c := &Credential{ID: uuid.NewString(), UserID: userID, Name: "k", Provider: "claude", BaseURL: "https://relay.example",
		Secret: "sk-secret-value-123", Hint: "sk-s••••-123", ExpiresAt: expires.UnixMilli(), CreatedAt: time.Now().UnixMilli()}
	if err := s.CreateCredential(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUsernamesAreCaseInsensitiveUnique(t *testing.T) {
	s, ctx := open(t), context.Background()
	newUser(t, s, "Alice")
	if err := s.CreateUser(ctx, &User{Username: "alice", PasswordHash: "x"}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: %v", err)
	}
	if u, err := s.UserByName(ctx, "ALICE"); err != nil || u.Username != "Alice" {
		t.Fatalf("lookup: %v %v", u, err)
	}
}

func TestLockDurationDoublesUpToADay(t *testing.T) {
	cases := map[int]time.Duration{4: 0, 5: 15 * time.Minute, 6: 30 * time.Minute, 7: time.Hour, 11: 16 * time.Hour, 12: 24 * time.Hour, 40: 24 * time.Hour}
	for failures, want := range cases {
		if got := LockDuration(failures); got != want {
			t.Errorf("LockDuration(%d) = %v, want %v", failures, got, want)
		}
	}
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "bob")
	now := time.Now()
	for i := 1; i <= 5; i++ {
		until, err := s.RecordLoginFailure(ctx, u.ID, now)
		if err != nil {
			t.Fatal(err)
		}
		if (i < 5) != (until == 0) {
			t.Fatalf("failure %d: locked until %d", i, until)
		}
	}
	if err := s.ResetLoginFailures(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UserByID(ctx, u.ID); got.FailedLogins != 0 || got.LockedUntil != 0 {
		t.Fatalf("reset: %+v", got)
	}
}

func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "carol")
	now := time.Now()
	for _, h := range []string{"keep", "other"} {
		if err := s.CreateSession(ctx, &Session{TokenHash: h, UserID: u.ID, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetPassword(ctx, u.ID, "new-hash", "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, "keep", now); err != nil {
		t.Fatalf("current session must survive: %v", err)
	}
	if _, err := s.SessionUser(ctx, "other", now); err == nil {
		t.Fatal("other sessions must end")
	}
	// Expired sessions never authenticate.
	if err := s.CreateSession(ctx, &Session{TokenHash: "old", UserID: u.ID, ExpiresAt: now.Add(-time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, "old", now); err == nil {
		t.Fatal("expired session accepted")
	}
}

func TestCredentialSecretIsWriteOnly(t *testing.T) {
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "dave")
	c := newCredential(t, s, u.ID, time.Now().Add(time.Hour))
	rows, err := s.ListCredentials(ctx, u.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v %d", err, len(rows))
	}
	one, err := s.GetCredential(ctx, u.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Secret != "" || one.Secret != "" {
		t.Fatal("list and detail queries must not read the secret")
	}
	full := *c // even a struct that holds the secret must not serialize it
	data, _ := json.Marshal([]any{rows, one, full})
	if strings.Contains(string(data), "sk-secret-value-123") || strings.Contains(string(data), `"secret"`) {
		t.Fatalf("secret leaked into JSON: %s", data)
	}
	if secret, err := s.CredentialSecret(ctx, u.ID, c.ID); err != nil || secret != "sk-secret-value-123" {
		t.Fatalf("runner read: %q %v", secret, err)
	}
	other := newUser(t, s, "eve")
	if _, err := s.CredentialSecret(ctx, other.ID, c.ID); err == nil {
		t.Fatal("another user read the secret")
	}
	if _, err := s.GetCredential(ctx, other.ID, c.ID); err == nil {
		t.Fatal("another user saw the key")
	}
}

func TestCredentialLimitsAndExpiry(t *testing.T) {
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "frank")
	now := time.Now()
	expired := newCredential(t, s, u.ID, now.Add(-time.Minute))
	live := newCredential(t, s, u.ID, now.Add(time.Hour))
	sc := &Schedule{ID: uuid.NewString(), UserID: u.ID, CredentialID: expired.ID, Provider: "claude", Model: "m", IntervalMinutes: 60, Enabled: true}
	if err := s.CreateSchedule(ctx, sc); err != nil {
		t.Fatal(err)
	}
	n, err := s.DeleteExpiredCredentials(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("expired deletion: %d %v", n, err)
	}
	if _, err := s.GetCredential(ctx, u.ID, expired.ID); err == nil {
		t.Fatal("expired key still present")
	}
	if _, err := s.GetSchedule(ctx, sc.ID); err == nil {
		t.Fatal("schedule of an expired key must stop")
	}
	if _, err := s.GetCredential(ctx, u.ID, live.ID); err != nil {
		t.Fatal("live key deleted")
	}
	for i := 1; i < MaxCredentialsPerUser; i++ {
		newCredential(t, s, u.ID, now.Add(time.Hour))
	}
	extra := &Credential{ID: uuid.NewString(), UserID: u.ID, ExpiresAt: now.Add(time.Hour).UnixMilli()}
	if err := s.CreateCredential(ctx, extra); !errors.Is(err, ErrLimit) {
		t.Fatalf("21st key: %v", err)
	}
	// Three consecutive auth failures pause a key.
	if err := s.RecordAuthResult(ctx, live.ID, 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetCredential(ctx, u.ID, live.ID); got.PausedReason != "auth_failed" {
		t.Fatalf("not paused: %+v", got)
	}
	if err := s.ResumeCredential(ctx, u.ID, live.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetCredential(ctx, u.ID, live.ID); got.PausedReason != "" || got.AuthFailures != 0 {
		t.Fatalf("not resumed: %+v", got)
	}
}

func TestDailyKeyCountAndPurge(t *testing.T) {
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "gina")
	c := newCredential(t, s, u.ID, time.Now().Add(time.Hour))
	for i := 0; i < 3; i++ {
		r := newRun("o", "completed")
		r.CredentialID = &c.ID
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.CredentialUsedToday(ctx, c.ID, time.Now()); err != nil || n != 3 {
		t.Fatalf("used today: %d %v", n, err)
	}
	if n, err := s.DeleteAllCredentials(ctx); err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
}

func TestScheduleClaimRunsOnce(t *testing.T) {
	s, ctx := open(t), context.Background()
	u := newUser(t, s, "hank")
	now := time.Now()
	sc := &Schedule{ID: uuid.NewString(), UserID: u.ID, CredentialID: "c", Provider: "claude", Model: "m", IntervalMinutes: 30, Enabled: true, NextRunAt: now.Add(-time.Second).UnixMilli()}
	if err := s.CreateSchedule(ctx, sc); err != nil {
		t.Fatal(err)
	}
	due, err := s.DueSchedules(ctx, now, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due: %d %v", len(due), err)
	}
	first, _ := s.ClaimSchedule(ctx, sc.ID, due[0].NextRunAt, now.Add(30*time.Minute).UnixMilli())
	second, _ := s.ClaimSchedule(ctx, sc.ID, due[0].NextRunAt, now.Add(30*time.Minute).UnixMilli())
	if !first || second {
		t.Fatalf("claims: first=%v second=%v", first, second)
	}
	if due, _ := s.DueSchedules(ctx, now, 10); len(due) != 0 {
		t.Fatal("claimed schedule still due")
	}
	for i := 1; i < MaxSchedulesPerUser; i++ {
		if err := s.CreateSchedule(ctx, &Schedule{ID: uuid.NewString(), UserID: u.ID, IntervalMinutes: 60}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateSchedule(ctx, &Schedule{ID: uuid.NewString(), UserID: u.ID, IntervalMinutes: 60}); !errors.Is(err, ErrLimit) {
		t.Fatalf("11th schedule: %v", err)
	}
}

func TestOnlyAccountsHaveRecordLists(t *testing.T) {
	s, ctx := open(t), context.Background()
	a, b := newUser(t, s, "ivan"), newUser(t, s, "judy")
	guest := newRun("browser-1", "completed") // started without an account
	mine := newRun("browser-1", "completed")
	mine.UserID = &a.ID
	theirs := newRun("browser-2", "completed")
	theirs.UserID = &b.ID
	for _, r := range []*Run{guest, mine, theirs} {
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err := s.ListRuns(ctx, ListQuery{UserID: a.ID, Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != mine.ID {
		t.Fatalf("account list must hold only its own runs: %d %v", total, err)
	}
	if rows, total, _ := s.ListRuns(ctx, ListQuery{Page: 1, PageSize: 20}); total != 0 || len(rows) != 0 {
		t.Fatal("without an account there is no list")
	}
	// The account may edit its run's remark; another account may not.
	if _, err := s.UpdateRemark(ctx, "", a.ID, mine.ID, "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateRemark(ctx, "", b.ID, mine.ID, "theirs"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other account edit: %v", err)
	}
	// A guest run stays a guest run: still openable by its link.
	if got, err := s.GetRun(ctx, guest.ID); err != nil || got.UserID != nil {
		t.Fatalf("guest run: %+v %v", got, err)
	}
}

func TestPreviousScoreOfSameConfig(t *testing.T) {
	s, ctx := open(t), context.Background()
	key := ConfigKey("openai_api", "https://x.example", "gpt", json.RawMessage(`{"suite":"full","vision":false}`))
	if key != ConfigKey("openai_api", "https://x.example/", "gpt", json.RawMessage(`{"vision":false,"suite":"full","model":"gpt"}`)) {
		t.Fatal("config key must ignore false options, key order and the model field")
	}
	other := ConfigKey("openai_api", "https://x.example", "gpt", json.RawMessage(`{"suite":"basic"}`))
	u := newUser(t, s, "kim")
	scores := []int{90, 80}
	var runs []*Run
	for i, score := range scores {
		r := newRun("o", "completed")
		r.StartedAt += int64(i * 1000)
		r.Score, r.ConfigKey, r.UserID = &score, key, &u.ID
		runs = append(runs, r)
	}
	unrelated := newRun("o", "completed")
	unrelated.StartedAt += 500
	unrelatedScore := 10
	unrelated.Score, unrelated.ConfigKey, unrelated.UserID = &unrelatedScore, other, &u.ID
	for _, r := range append(runs, unrelated) {
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	q := ListQuery{UserID: u.ID, Page: 1, PageSize: 20}
	rows, _, err := s.ListRuns(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	prev, err := s.PreviousScores(ctx, q, rows)
	if err != nil {
		t.Fatal(err)
	}
	if p := prev[runs[1].ID]; p == nil || *p != 90 {
		t.Fatalf("previous of the newer run: %v", p)
	}
	if prev[runs[0].ID] != nil || prev[unrelated.ID] != nil {
		t.Fatal("first runs of a configuration have no previous score")
	}
}
