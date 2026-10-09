package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"model-check/common"
	"model-check/internal/store"
	"model-check/pkg/geminicheck"
	"model-check/pkg/openaicheck"
)

// Saved keys are stored in plaintext (an explicit product decision) and are
// write-only: no endpoint returns the secret, and a key is only ever sent to
// the base URL it was saved with.

var expiryChoices = map[int]bool{1: true, 7: true, 30: true, 90: true}

// officialOpenAIBase is where an image check's provenance key may be sent.
const officialOpenAIBase = "https://api.openai.com"

func normalizeEndpoint(provider, raw string) (string, error) {
	switch provider {
	case "claude":
		return normalizeClaudeURL(raw)
	case "openai", "image":
		return openaicheck.NormalizeBaseURL(raw)
	case "gemini":
		return geminicheck.NormalizeBaseURL(raw)
	}
	return "", errors.New("unknown provider")
}

// maskKey matches the web app's maskKey: sk-abc••••wxyz.
func maskKey(key string) string {
	key = strings.TrimSpace(key)
	n := utf8.RuneCountInString(key)
	if n <= 8 {
		return strings.Repeat("•", max(n, 4))
	}
	runes := []rune(key)
	head := min(6, n/4)
	return string(runes[:head]) + "••••" + string(runes[n-4:])
}

func validSecret(key string) bool {
	return len(key) >= 4 && len(key) <= 8192 && !strings.ContainsAny(key, "\r\n")
}

func (s *Server) listCredentials(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	rows, err := s.cfg.Store.ListCredentials(c.Request.Context(), u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not load saved keys"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func (s *Server) createCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok || !s.requireHTTPS(c) {
		return
	}
	var in struct {
		Name         string `json:"name"`
		Provider     string `json:"provider"`
		BaseURL      string `json:"base_url"`
		Secret       string `json:"secret"`
		ExpiresDays  int    `json:"expires_days"`
		AckTestKey   bool   `json:"ack_test_key"`
		AckQuota     bool   `json:"ack_quota"`
		AckPlaintext bool   `json:"ack_plaintext"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if common.DecodeJson(c.Request.Body, &in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	if !in.AckTestKey || !in.AckQuota || !in.AckPlaintext {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Confirm all three statements before saving the key"})
		return
	}
	if !expiryChoices[in.ExpiresDays] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Choose an expiry of 1, 7, 30 or 90 days"})
		return
	}
	in.Secret, in.Name = strings.TrimSpace(in.Secret), strings.TrimSpace(in.Name)
	endpoint, err := normalizeEndpoint(in.Provider, in.BaseURL)
	if err != nil || !validSecret(in.Secret) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Enter a valid Base URL and API key"})
		return
	}
	if in.Name == "" {
		if parsed, err := url.Parse(endpoint); err == nil {
			in.Name = parsed.Hostname()
		}
	}
	if !utf8.ValidString(in.Name) || utf8.RuneCountInString(in.Name) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "The name must contain at most 64 characters"})
		return
	}
	client := s.newClient()
	defer client.CloseIdleConnections()
	if err := client.ValidateURL(c.Request.Context(), endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": preflightMessage(err)})
		return
	}
	now := s.now()
	row := &store.Credential{ID: uuid.NewString(), UserID: u.ID, Name: in.Name, Provider: in.Provider, BaseURL: endpoint,
		Secret: in.Secret, Hint: maskKey(in.Secret), ExpiresAt: now.Add(time.Duration(in.ExpiresDays) * 24 * time.Hour).UnixMilli(),
		AcknowledgedAt: now.UnixMilli(), CreatedAt: now.UnixMilli()}
	if err := s.cfg.Store.CreateCredential(c.Request.Context(), row); err != nil {
		if errors.Is(err, store.ErrLimit) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "An account can save at most 20 keys. Delete one first"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not save the key"})
		return
	}
	saved, err := s.cfg.Store.GetCredential(c.Request.Context(), u.ID, row.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not save the key"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": saved})
}

func (s *Server) renewCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	var in struct {
		Days int `json:"days"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if common.DecodeJson(c.Request.Body, &in) != nil || !expiryChoices[in.Days] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Choose an expiry of 1, 7, 30 or 90 days"})
		return
	}
	err := s.cfg.Store.RenewCredential(c.Request.Context(), u.ID, c.Param("id"), s.now().Add(time.Duration(in.Days)*24*time.Hour).UnixMilli())
	s.credentialResult(c, u.ID, err)
}

func (s *Server) resumeCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	s.credentialResult(c, u.ID, s.cfg.Store.ResumeCredential(c.Request.Context(), u.ID, c.Param("id")))
}

func (s *Server) credentialResult(c *gin.Context, userID int64, err error) {
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Saved key not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not update the key"})
		return
	}
	row, err := s.cfg.Store.GetCredential(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not update the key"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func (s *Server) deleteCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok {
		return
	}
	err := s.cfg.Store.DeleteCredential(c.Request.Context(), u.ID, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Saved key not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not delete the key"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---- using a saved key ----

// keyLease is a saved key checked out for one run. finish must be called.
type keyLease struct {
	cred     *store.Credential
	endpoint string
	secret   string
	watch    *authWatch
	release  func()
}

// leaseError is a refusal with the HTTP status a handler should answer with.
type leaseError struct {
	status  int
	message string
}

func (e *leaseError) Error() string { return e.message }

// leaseCredential applies every rule for using a saved key: owner, provider,
// base URL binding, expiry, pause, daily limit and one run per key at a time.
// rawBaseURL may be empty to use the saved one.
func (s *Server) leaseCredential(ctx context.Context, userID int64, id, provider, rawBaseURL string) (*keyLease, error) {
	if userID == 0 {
		return nil, &leaseError{http.StatusUnauthorized, "Sign in to use a saved key"}
	}
	cred, err := s.cfg.Store.GetCredential(ctx, userID, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, &leaseError{http.StatusNotFound, "Saved key not found"}
	}
	if err != nil {
		return nil, &leaseError{http.StatusInternalServerError, "Could not load the saved key"}
	}
	if cred.Provider != provider {
		return nil, &leaseError{http.StatusBadRequest, "This saved key is for a different kind of check"}
	}
	if strings.TrimSpace(rawBaseURL) != "" {
		endpoint, err := normalizeEndpoint(provider, rawBaseURL)
		if err != nil || endpoint != cred.BaseURL {
			return nil, &leaseError{http.StatusBadRequest, "A saved key can only be sent to the Base URL it was saved with. Enter the key again to use another address"}
		}
	}
	now := s.now()
	if cred.ExpiresAt <= now.UnixMilli() {
		return nil, &leaseError{http.StatusGone, "This saved key has expired"}
	}
	if cred.PausedReason != "" {
		return nil, &leaseError{http.StatusConflict, "This saved key is paused after repeated authentication failures"}
	}
	used, err := s.cfg.Store.CredentialUsedToday(ctx, cred.ID, now)
	if err != nil {
		return nil, &leaseError{http.StatusInternalServerError, "Could not load the saved key"}
	}
	if used >= store.MaxChecksPerKeyPerDay {
		return nil, &leaseError{http.StatusTooManyRequests, "This saved key has reached its limit of 50 checks today"}
	}
	if _, busy := s.credBusy.LoadOrStore(cred.ID, true); busy {
		return nil, &leaseError{http.StatusConflict, "A check is already running with this saved key"}
	}
	secret, err := s.cfg.Store.CredentialSecret(ctx, userID, cred.ID)
	if err != nil {
		s.credBusy.Delete(cred.ID)
		return nil, &leaseError{http.StatusInternalServerError, "Could not load the saved key"}
	}
	_ = s.cfg.Store.TouchCredential(ctx, cred.ID, now)
	host := ""
	if u, err := url.Parse(cred.BaseURL); err == nil {
		host = u.Host
	}
	return &keyLease{cred: cred, endpoint: cred.BaseURL, secret: secret,
		watch:   &authWatch{host: host, failures: cred.AuthFailures, start: cred.AuthFailures},
		release: func() { s.credBusy.Delete(cred.ID) }}, nil
}

// finish stores the key's authentication outcome and frees it for the next run.
func (l *keyLease) finish(st *store.Store) {
	if l == nil {
		return
	}
	if failures := l.watch.count(); failures != l.watch.start {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = st.RecordAuthResult(ctx, l.cred.ID, failures)
		cancel()
	}
	l.release()
}

// leaseFor is leaseCredential for a handler: it answers the refusal itself.
func (s *Server) leaseFor(c *gin.Context, id, provider, rawBaseURL string) (*keyLease, bool) {
	if currentUser(c) == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "login_required", "message": "Sign in to use a saved key"})
		return nil, false
	}
	if !s.sameSiteRequest(c.Request) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Cross-site request refused"})
		return nil, false
	}
	lease, err := s.leaseCredential(c.Request.Context(), currentUserID(c), id, provider, rawBaseURL)
	if err != nil {
		var le *leaseError
		if errors.As(err, &le) {
			c.JSON(le.status, gin.H{"success": false, "message": le.message})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		}
		return nil, false
	}
	return lease, true
}

// authWatch counts consecutive 401/403 answers from the key's own host. After
// three, the key is paused and no further request is sent with it.
type authWatch struct {
	next     http.RoundTripper
	host     string
	mu       sync.Mutex
	failures int
	start    int
}

var errKeyPaused = errors.New("saved key paused after repeated 401/403 responses")

func (w *authWatch) wrap(client *http.Client) {
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	w.next = next
	client.Transport = w
}

func (w *authWatch) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failures
}

func (w *authWatch) RoundTrip(r *http.Request) (*http.Response, error) {
	own := strings.EqualFold(r.URL.Host, w.host)
	if own && w.count() >= 3 {
		return nil, errKeyPaused
	}
	resp, err := w.next.RoundTrip(r)
	if err == nil && own {
		w.mu.Lock()
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			w.failures++
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			w.failures = 0
		}
		w.mu.Unlock()
	}
	return resp, err
}

// runMeta links a stored run to an account, a saved key and a schedule.
type runMeta struct {
	UserID       *int64
	CredentialID *string
	ScheduleID   *string
}

func (m runMeta) apply(run *store.Run) {
	run.UserID, run.CredentialID, run.ScheduleID = m.UserID, m.CredentialID, m.ScheduleID
	run.ConfigKey = store.ReportConfigKey(run.Transport, run.ReportJSON)
}

func requestMeta(c *gin.Context, lease *keyLease) runMeta {
	var m runMeta
	if id := currentUserID(c); id != 0 {
		m.UserID = &id
	}
	if lease != nil {
		m.CredentialID = &lease.cred.ID
	}
	return m
}
