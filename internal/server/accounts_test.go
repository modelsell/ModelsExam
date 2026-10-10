package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"model-check/internal/auth"
	"model-check/internal/secretbox"
	"model-check/internal/store"
)

const strongPassword = "Tr4in-Cactus-Lamp"

type harness struct {
	t     *testing.T
	srv   *Server
	st    *store.Store
	h     http.Handler
	proxy string // RemoteAddr of the trusted proxy
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	auth.BcryptCost = 4
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := secretbox.NewKey()
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UseSecretBox(context.Background(), box); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Store: st, AllowPrivate: true, Trusted: []string{"10.0.0.1"}, RequireHTTPS: true, SiteURL: "https://exam.test"})
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, st: st, h: srv.Handler(), proxy: "10.0.0.1:5555"}
}

type client struct {
	h       *harness
	cookies map[string]string
	ip      string // client address reported by the proxy
	https   bool
	origin  string
}

func (h *harness) client(ip string) *client {
	return &client{h: h, cookies: map[string]string{}, ip: ip, https: true, origin: "https://exam.test"}
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.h.t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		data, _ := json.Marshal(body)
		reader = strings.NewReader(string(data))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "exam.test"
	req.RemoteAddr = c.h.proxy
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.https {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	if c.origin != "" && method != http.MethodGet {
		req.Header.Set("Origin", c.origin)
	}
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	rec := httptest.NewRecorder()
	c.h.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck.Value
		}
	}
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	out["_raw"] = rec.Body.String()
	return rec.Code, out
}

func (c *client) register(name string) {
	c.h.t.Helper()
	if code, out := c.do("POST", "/api/auth/register", map[string]string{"username": name, "password": strongPassword}); code != 200 {
		c.h.t.Fatalf("register %s: %d %v", name, code, out)
	}
}

func TestHTTPSGate(t *testing.T) {
	h := newHarness(t)
	c := h.client("1.1.1.1")
	c.https = false
	if code, out := c.do("POST", "/api/auth/register", map[string]string{"username": "alice", "password": strongPassword}); code != 403 || out["code"] != "https_required" {
		t.Fatalf("plain HTTP register: %d %v", code, out)
	}
	// X-Forwarded-Proto from an untrusted peer is ignored.
	c.https = true
	c.h.proxy = "192.0.2.9:1234"
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": strongPassword}); code != 403 {
		t.Fatalf("spoofed proto: %d", code)
	}
	c.h.proxy = "10.0.0.1:5555"
	c.register("alice")
	if _, out := c.do("GET", "/api/auth/me", nil); out["data"].(map[string]any)["user"] == nil {
		t.Fatalf("not signed in after register: %v", out)
	}
}

func TestCrossSiteRequestsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.client("1.1.1.2")
	c.origin = "https://evil.example"
	if code, _ := c.do("POST", "/api/auth/register", map[string]string{"username": "bob", "password": strongPassword}); code != 403 {
		t.Fatalf("cross-site register: %d", code)
	}
}

func TestLoginLockoutAndRateLimits(t *testing.T) {
	h := newHarness(t)
	c := h.client("2.2.2.2")
	c.register("carol")
	attacker := h.client("3.3.3.3")
	_, unknown := attacker.do("POST", "/api/auth/login", map[string]string{"username": "nobody", "password": "x"})
	_, wrong := attacker.do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": "wrong"})
	if unknown["message"] != msgBadLogin || wrong["message"] != msgBadLogin {
		t.Fatalf("unknown user and wrong password must look the same: %v / %v", unknown["message"], wrong["message"])
	}
	var code int
	var out map[string]any
	for i := 0; i < 4; i++ { // failures 2..5
		code, out = attacker.do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": "wrong"})
	}
	if code != http.StatusLocked || out["minutes"].(float64) != 15 {
		t.Fatalf("fifth failure must lock for 15 minutes: %d %v", code, out)
	}
	// Even the right password is refused while locked.
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": strongPassword}); code != http.StatusLocked {
		t.Fatalf("locked account accepted a login: %d", code)
	}
	// After the lock, a success resets the counter.
	h.srv.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": strongPassword}); code != 200 {
		t.Fatalf("login after lock: %d", code)
	}
	h.srv.now = time.Now
	// Per IP: 10 attempts a minute.
	flood := h.client("4.4.4.4")
	for i := 0; i < 10; i++ {
		flood.do("POST", "/api/auth/login", map[string]string{"username": fmt.Sprintf("u%d", i), "password": "x"})
	}
	if code, _ := flood.do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": strongPassword}); code != 429 {
		t.Fatalf("11th attempt a minute: %d", code)
	}
	// Registration: 3 accounts per IP per hour.
	reg := h.client("5.5.5.5")
	for i := 0; i < 3; i++ {
		reg.register(fmt.Sprintf("user%d", i))
	}
	if code, _ := reg.do("POST", "/api/auth/register", map[string]string{"username": "user9", "password": strongPassword}); code != 429 {
		t.Fatalf("4th registration: %d", code)
	}
}

func TestPasswordChangeAndLogoutEndSessions(t *testing.T) {
	h := newHarness(t)
	a := h.client("6.6.6.6")
	a.register("dave")
	b := h.client("6.6.6.7")
	if code, _ := b.do("POST", "/api/auth/login", map[string]string{"username": "dave", "password": strongPassword}); code != 200 {
		t.Fatal("second login failed")
	}
	if code, _ := a.do("POST", "/api/auth/password", map[string]string{"old_password": "wrong", "new_password": "New-Cactus-Lamp-9"}); code != 400 {
		t.Fatalf("password change without the old password: %d", code)
	}
	if code, _ := a.do("POST", "/api/auth/password", map[string]string{"old_password": strongPassword, "new_password": "New-Cactus-Lamp-9"}); code != 200 {
		t.Fatalf("password change: %d", code)
	}
	if _, out := b.do("GET", "/api/auth/me", nil); out["data"].(map[string]any)["user"] != nil {
		t.Fatal("other session survived a password change")
	}
	if _, out := a.do("GET", "/api/auth/me", nil); out["data"].(map[string]any)["user"] == nil {
		t.Fatal("current session must survive its own password change")
	}
	token := a.cookies[sessionCookie]
	a.do("POST", "/api/auth/logout", nil)
	a.cookies[sessionCookie] = token // a stolen copy of the cookie
	if _, out := a.do("GET", "/api/auth/me", nil); out["data"].(map[string]any)["user"] != nil {
		t.Fatal("session usable after logout")
	}
}

func saveKey(t *testing.T, c *client, provider, base, secret string) string {
	t.Helper()
	code, out := c.do("POST", "/api/credentials", map[string]any{"provider": provider, "base_url": base, "secret": secret, "expires_days": 7,
		"ack_test_key": true, "ack_quota": true, "ack_plaintext": true})
	if code != 200 {
		t.Fatalf("save key: %d %v", code, out)
	}
	return out["data"].(map[string]any)["id"].(string)
}

func TestSavedKeyIsWriteOnly(t *testing.T) {
	h := newHarness(t)
	c := h.client("7.7.7.7")
	c.register("erin")
	if code, _ := c.do("POST", "/api/credentials", map[string]any{"provider": "openai", "base_url": "http://127.0.0.1:1", "secret": "sk-abcd-secret-wxyz", "expires_days": 7, "ack_test_key": true, "ack_quota": true}); code != 400 {
		t.Fatalf("missing acknowledgement accepted: %d", code)
	}
	if code, _ := c.do("POST", "/api/credentials", map[string]any{"provider": "openai", "base_url": "http://127.0.0.1:1", "secret": "sk-abcd-secret-wxyz", "expires_days": 365, "ack_test_key": true, "ack_quota": true, "ack_plaintext": true}); code != 400 {
		t.Fatalf("expiry beyond 90 days accepted: %d", code)
	}
	id := saveKey(t, c, "openai", "http://127.0.0.1:1/v1", "sk-abcd-secret-wxyz")
	_, list := c.do("GET", "/api/credentials", nil)
	raw := list["_raw"].(string)
	if strings.Contains(raw, "secret-wxyz") || strings.Contains(raw, `"secret"`) || !strings.Contains(raw, id) || !strings.Contains(raw, "sk-a••••wxyz") {
		t.Fatalf("list must show only the mask: %s", raw)
	}
	_, renewed := c.do("POST", "/api/credentials/"+id+"/renew", map[string]int{"days": 30})
	if strings.Contains(renewed["_raw"].(string), "secret-wxyz") {
		t.Fatal("renew leaked the secret")
	}
	// Another account cannot see or use it.
	other := h.client("7.7.7.8")
	other.register("frank")
	if code, _ := other.do("POST", "/api/model_check/openai", map[string]any{"credential_id": id, "model": "gpt"}); code != 404 {
		t.Fatalf("other account used the key: %d", code)
	}
	// Anonymous visitors cannot use saved keys.
	anon := h.client("7.7.7.9")
	if code, _ := anon.do("POST", "/api/model_check/openai", map[string]any{"credential_id": id, "model": "gpt"}); code != 401 {
		t.Fatalf("anonymous use: %d", code)
	}
}

// fakeUpstream answers every request with status and counts requests and
// the keys it received.
func fakeUpstream(t *testing.T, status int) (*httptest.Server, *atomic.Int64, *atomic.Value) {
	var hits atomic.Int64
	var lastKey atomic.Value
	lastKey.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		key := r.Header.Get("Authorization") + r.Header.Get("x-api-key")
		if key != "" {
			lastKey.Store(key)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid key"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &lastKey
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSavedKeyBindingUseAndAutoPause(t *testing.T) {
	h := newHarness(t)
	upstream, hits, lastKey := fakeUpstream(t, http.StatusUnauthorized)
	c := h.client("8.8.8.8")
	c.register("gina")
	id := saveKey(t, c, "openai", upstream.URL, "sk-bound-key-1234")
	// The key is never sent to another address.
	if code, out := c.do("POST", "/api/model_check/openai", map[string]any{"credential_id": id, "base_url": "http://127.0.0.2:9", "model": "gpt", "suite": "basic"}); code != 400 {
		t.Fatalf("other base URL accepted: %d %v", code, out)
	}
	if hits.Load() != 0 {
		t.Fatal("upstream contacted on a refused request")
	}
	// Each run stops early on 401; the consecutive count carries over runs.
	runs := 0
	for ; runs < 3; runs++ {
		code, out := c.do("POST", "/api/model_check/openai", map[string]any{"credential_id": id, "model": "gpt", "suite": "basic"})
		if code == http.StatusConflict {
			break
		}
		if code != 200 || strings.Contains(out["_raw"].(string), "sk-bound-key-1234") {
			t.Fatalf("check with saved key: %d %.300s", code, out["_raw"])
		}
	}
	if !strings.Contains(lastKey.Load().(string), "sk-bound-key-1234") {
		t.Fatal("upstream did not receive the saved key")
	}
	// Every answer was 401: the key is paused and further use refused.
	cred, err := h.st.GetCredential(context.Background(), currentUserIDFor(t, h, "gina"), id)
	if err != nil || cred.PausedReason != "auth_failed" {
		t.Fatalf("key not paused after repeated 401s: %+v %v", cred, err)
	}
	if code, _ := c.do("POST", "/api/model_check/openai", map[string]any{"credential_id": id, "model": "gpt", "suite": "basic"}); code != http.StatusConflict {
		t.Fatalf("paused key used: %d", code)
	}
	// The run belongs to the account and counts toward the key's daily limit.
	if n, _ := h.st.CredentialUsedToday(context.Background(), id, time.Now()); n != int64(runs) {
		t.Fatalf("daily count %d", n)
	}
	_, list := c.do("GET", "/api/model_check/history", nil)
	if items := list["data"].(map[string]any)["items"].([]any); len(items) != runs || !items[0].(map[string]any)["mine"].(bool) {
		t.Fatalf("account history: %v", list["data"])
	}
}

func currentUserIDFor(t *testing.T, h *harness, name string) int64 {
	u, err := h.st.UserByName(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestRetestAndScheduleRunInBackground(t *testing.T) {
	h := newHarness(t)
	upstream, _, _ := fakeUpstream(t, http.StatusForbidden)
	c := h.client("9.9.9.9")
	c.register("hank")
	id := saveKey(t, c, "gemini", upstream.URL, "AIza-test-key-5678")
	code, out := c.do("POST", "/api/jobs/retest", map[string]any{"provider": "gemini", "credential_id": id, "model": "gemini-x", "options": map[string]any{"suite": "basic", "unknown": true}})
	if code != 200 {
		t.Fatalf("retest: %d %v", code, out)
	}
	runID := out["data"].(map[string]any)["run_id"].(string)
	waitFor(t, "background run to finish", func() bool {
		run, err := h.st.GetRun(context.Background(), runID)
		return err == nil && run.Status != "running"
	})
	run, _ := h.st.GetRun(context.Background(), runID)
	if run.UserID == nil || run.CredentialID == nil || *run.CredentialID != id || run.ConfigKey == "" || strings.Contains(run.ReportJSON, "AIza-test-key-5678") {
		t.Fatalf("background run metadata or redaction: %+v", run)
	}
	_ = h.st.ResumeCredential(context.Background(), currentUserIDFor(t, h, "hank"), id)

	// A schedule: interval limits, then one due run even when ticks overlap.
	if code, _ := c.do("POST", "/api/schedules", map[string]any{"credential_id": id, "model": "gemini-x", "options": map[string]any{"suite": "basic"}, "interval_minutes": 10}); code != 400 {
		t.Fatalf("10-minute interval accepted: %d", code)
	}
	code, out = c.do("POST", "/api/schedules", map[string]any{"credential_id": id, "model": "gemini-x", "options": map[string]any{"suite": "basic"}, "interval_minutes": 30})
	if code != 200 {
		t.Fatalf("create schedule: %d %v", code, out)
	}
	scheduleID := out["data"].(map[string]any)["id"].(string)
	h.srv.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	go h.srv.runDueSchedules()
	go h.srv.runDueSchedules()
	h.srv.runDueSchedules()
	waitFor(t, "scheduled run to be recorded", func() bool {
		runs, _ := h.st.ListScheduleRuns(context.Background(), scheduleID, 10)
		return len(runs) > 0 && runs[0].FinishedAt > 0
	})
	time.Sleep(300 * time.Millisecond)
	runs, _ := h.st.ListScheduleRuns(context.Background(), scheduleID, 10)
	if len(runs) != 1 || runs[0].RunID == "" {
		t.Fatalf("want exactly one scheduled run, got %+v", runs)
	}
	sc, _ := h.st.GetSchedule(context.Background(), scheduleID)
	if next := time.UnixMilli(sc.NextRunAt).Sub(h.srv.now()); next < 26*time.Minute || next > 34*time.Minute {
		t.Fatalf("next run in %v, want 30m ±10%%", next)
	}
	_, list := c.do("GET", "/api/model_check/history", nil)
	found := false
	for _, item := range list["data"].(map[string]any)["items"].([]any) {
		found = found || item.(map[string]any)["scheduled"].(bool)
	}
	if !found {
		t.Fatal("scheduled run not marked in the records list")
	}
	// Deleting the key stops its schedules.
	c.do("DELETE", "/api/credentials/"+id, nil)
	if _, err := h.st.GetSchedule(context.Background(), scheduleID); err == nil {
		t.Fatal("schedule survived its key")
	}
}

func TestMaskKey(t *testing.T) {
	for in, want := range map[string]string{"sk-abcdefghijklmnwxyz": "sk-ab••••wxyz", "short": "•••••", "abc": "••••"} {
		if got := maskKey(in); got != want {
			t.Errorf("maskKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCloudflareClientIP(t *testing.T) {
	h := newHarness(t)
	h.srv.cfg.Cloudflare = true
	c := h.client("173.245.48.10") // a Cloudflare edge address
	var seen string
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = h.proxy
	req.Header.Set("X-Forwarded-For", c.ip)
	req.Header.Set("CF-Connecting-IP", "203.0.113.7")
	gc, _ := ginTestContext(req)
	seen = h.srv.clientIP(gc)
	if seen != "203.0.113.7" {
		t.Fatalf("via Cloudflare: %s", seen)
	}
	// A direct hit that claims a CF header is not believed.
	req.Header.Set("X-Forwarded-For", "198.51.100.4")
	gc, _ = ginTestContext(req)
	if seen = h.srv.clientIP(gc); seen != "198.51.100.4" {
		t.Fatalf("spoofed CF header believed: %s", seen)
	}
}

func ginTestContext(req *http.Request) (*gin.Context, *gin.Engine) {
	c, engine := gin.CreateTestContext(httptest.NewRecorder())
	_ = engine.SetTrustedProxies([]string{"10.0.0.1"})
	c.Request = req
	return c, engine
}

func TestRecordsAreForAccountsOnly(t *testing.T) {
	h := newHarness(t)
	upstream, _, _ := fakeUpstream(t, http.StatusUnauthorized)
	guest := h.client("11.0.0.1")
	// A guest can still run a check and open its report by link...
	code, out := guest.do("POST", "/api/model_check/openai", map[string]any{"base_url": upstream.URL, "key": "sk-guest-key-0000", "model": "gpt", "suite": "basic"})
	if code != 200 {
		t.Fatalf("guest check: %d %.200s", code, out["_raw"])
	}
	guestRun := out["data"].(map[string]any)["id"].(string)
	if code, _ := guest.do("GET", "/api/model_check/history/"+guestRun, nil); code != 200 {
		t.Fatalf("report link: %d", code)
	}
	// ...but has no record list.
	if code, out := guest.do("GET", "/api/model_check/history", nil); code != 401 || out["code"] != "login_required" {
		t.Fatalf("guest list: %d %v", code, out)
	}
	// Signing in in the same browser does not bring the guest run along.
	guest.register("lena")
	_, list := guest.do("GET", "/api/model_check/history", nil)
	if items := list["data"].(map[string]any)["items"].([]any); len(items) != 0 {
		t.Fatalf("guest run listed in the account: %v", items)
	}
	if code, _ := guest.do("POST", "/api/auth/claim", map[string]any{}); code != 404 {
		t.Fatalf("claim endpoint still exists: %d", code)
	}
	// Runs started while signed in are listed, and only for that account.
	if code, _ := guest.do("POST", "/api/model_check/openai", map[string]any{"base_url": upstream.URL, "key": "sk-lena-key-0000", "model": "gpt", "suite": "basic"}); code != 200 {
		t.Fatal("signed-in check failed")
	}
	_, list = guest.do("GET", "/api/model_check/history", nil)
	if items := list["data"].(map[string]any)["items"].([]any); len(items) != 1 {
		t.Fatalf("account list: %v", items)
	}
	other := h.client("11.0.0.2")
	other.register("mike")
	_, list = other.do("GET", "/api/model_check/history", nil)
	if items := list["data"].(map[string]any)["items"].([]any); len(items) != 0 {
		t.Fatalf("another account sees runs: %v", items)
	}
}
