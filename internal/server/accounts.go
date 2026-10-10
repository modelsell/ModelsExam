package server

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"model-check/common"
	"model-check/internal/auth"
	"model-check/internal/store"
)

const (
	sessionCookie = "mc_session"
	sessionTTL    = 7 * 24 * time.Hour
	// msgBadLogin is the same for an unknown user and a wrong password.
	msgBadLogin = "Incorrect username or password"
)

// ---- request facts ----

func parseNets(entries []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			if ip.To4() != nil {
				e += "/32"
			} else {
				e += "/128"
			}
		}
		if _, n, err := net.ParseCIDR(e); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func inNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if ip != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// fromTrustedProxy is true when the connection comes from TRUSTED_PROXIES.
func (s *Server) fromTrustedProxy(r *http.Request) bool {
	return inNets(remoteIP(r), s.trusted)
}

// isHTTPS trusts X-Forwarded-Proto only from a trusted proxy.
func (s *Server) isHTTPS(r *http.Request) bool {
	return r.TLS != nil || (s.fromTrustedProxy(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

// clientIP is the visitor's address. Behind Cloudflare the trusted proxy
// reports a Cloudflare edge address; CF-Connecting-IP is then believed only
// because that edge address really is Cloudflare's.
func (s *Server) clientIP(c *gin.Context) string {
	ip := c.ClientIP()
	if s.cfg.Cloudflare && inNets(net.ParseIP(ip), cloudflareNets) {
		if real := net.ParseIP(strings.TrimSpace(c.GetHeader("CF-Connecting-IP"))); real != nil {
			return real.String()
		}
	}
	return ip
}

// sameSite rejects a state-changing browser request sent from another site.
// Requests without Origin and Sec-Fetch-Site come from non-browser clients,
// which cannot ride on a visitor's cookies.
func (s *Server) sameSite() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m := c.Request.Method; m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions {
			c.Next()
			return
		}
		if !s.sameSiteRequest(c.Request) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "Cross-site request refused"})
			return
		}
		c.Next()
	}
}

func (s *Server) sameSiteRequest(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}
		if site, err := url.Parse(s.cfg.SiteURL); err == nil && site.Host != "" && strings.EqualFold(u.Host, site.Host) {
			return true
		}
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

// requireHTTPS refuses to take a password or an API key over plain HTTP.
func (s *Server) requireHTTPS(c *gin.Context) bool {
	if !s.cfg.RequireHTTPS || s.isHTTPS(c.Request) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"success": false, "code": "https_required", "message": "Sign-in and saved keys need HTTPS. Open this site over https://"})
	return false
}

// ---- sessions ----

// session loads the signed-in user, if any. It never rejects a request.
func (s *Server) session() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token, err := c.Cookie(sessionCookie); err == nil && token != "" && len(token) < 100 {
			if u, err := s.cfg.Store.SessionUser(c.Request.Context(), auth.HashToken(token), s.now()); err == nil {
				c.Set("user", u)
				c.Set("session", auth.HashToken(token))
			}
		}
		c.Next()
	}
}

func currentUser(c *gin.Context) *store.User {
	if v, ok := c.Get("user"); ok {
		return v.(*store.User)
	}
	return nil
}

func currentUserID(c *gin.Context) int64 {
	if u := currentUser(c); u != nil {
		return u.ID
	}
	return 0
}

// requireUser answers 401 when nobody is signed in.
func requireUser(c *gin.Context) (*store.User, bool) {
	u := currentUser(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "login_required", "message": "Sign in first"})
		return nil, false
	}
	return u, true
}

func (s *Server) setSessionCookie(c *gin.Context, token string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.isHTTPS(c.Request)})
}

func (s *Server) startSession(c *gin.Context, userID int64) error {
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	now := s.now()
	ua := c.GetHeader("User-Agent")
	if len(ua) > 200 {
		ua = ua[:200]
	}
	if err := s.cfg.Store.CreateSession(c.Request.Context(), &store.Session{TokenHash: hash, UserID: userID,
		CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(sessionTTL).UnixMilli(), IP: s.clientIP(c), UserAgent: ua}); err != nil {
		return err
	}
	s.setSessionCookie(c, token, int(sessionTTL/time.Second))
	return nil
}

// ---- handlers ----

type accountView struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"created_at"`
}

func viewOf(u *store.User) *accountView {
	if u == nil {
		return nil
	}
	return &accountView{ID: u.ID, Username: u.Username, CreatedAt: u.CreatedAt}
}

func (s *Server) authMe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u := currentUser(c)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user": viewOf(u), "https": s.isHTTPS(c.Request), "require_https": s.cfg.RequireHTTPS}})
}

type credentialsInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func readCredentialsInput(c *gin.Context) (credentialsInput, bool) {
	var in credentialsInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if common.DecodeJson(c.Request.Body, &in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return in, false
	}
	in.Username = strings.TrimSpace(in.Username)
	return in, true
}

func (s *Server) register(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !s.requireHTTPS(c) {
		return
	}
	in, ok := readCredentialsInput(c)
	if !ok {
		return
	}
	ctx, ip, now := c.Request.Context(), s.clientIP(c), s.now()
	if n, err := s.cfg.Store.CountAttempts(ctx, "register", ip, now.Add(-time.Hour), true); err != nil || n >= 3 {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Too many accounts were created from your address. Try again later."})
		return
	}
	if err := auth.ValidateUsername(in.Username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := auth.ValidatePassword(in.Username, in.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not create the account"})
		return
	}
	u := &store.User{Username: in.Username, PasswordHash: hash, CreatedAt: now.UnixMilli(), PasswordChangedAt: now.UnixMilli(), RegisterIP: ip}
	if err := s.cfg.Store.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "This username is already taken"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not create the account"})
		return
	}
	_ = s.cfg.Store.AddAttempt(ctx, &store.LoginAttempt{Kind: "register", IP: ip, Username: u.UsernameLower, Success: true, CreatedAt: now.UnixMilli()})
	if err := s.startSession(c, u.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not sign in"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user": viewOf(u)}})
}

func (s *Server) login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !s.requireHTTPS(c) {
		return
	}
	in, ok := readCredentialsInput(c)
	if !ok {
		return
	}
	ctx, ip, now := c.Request.Context(), s.clientIP(c), s.now()
	perMinute, err1 := s.cfg.Store.CountAttempts(ctx, "login", ip, now.Add(-time.Minute), false)
	perHour, err2 := s.cfg.Store.CountAttempts(ctx, "login", ip, now.Add(-time.Hour), false)
	if err1 != nil || err2 != nil || perMinute >= 10 || perHour >= 50 {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Too many sign-in attempts from your address. Try again later."})
		return
	}
	attempt := &store.LoginAttempt{Kind: "login", IP: ip, Username: strings.ToLower(in.Username), CreatedAt: now.UnixMilli()}
	if len(attempt.Username) > 32 {
		attempt.Username = attempt.Username[:32]
	}
	defer func() { _ = s.cfg.Store.AddAttempt(ctx, attempt) }()
	u, err := s.cfg.Store.UserByName(ctx, in.Username)
	if err != nil || auth.ValidateUsername(in.Username) != nil || len(in.Password) > 1024 {
		auth.DummyCheck(in.Password)
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": msgBadLogin})
		return
	}
	if u.LockedUntil > now.UnixMilli() {
		lockedResponse(c, u.LockedUntil, now)
		return
	}
	if !auth.CheckPassword(u.PasswordHash, in.Password) {
		lockedUntil, err := s.cfg.Store.RecordLoginFailure(ctx, u.ID, now)
		if err == nil && lockedUntil > now.UnixMilli() {
			lockedResponse(c, lockedUntil, now)
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": msgBadLogin})
		return
	}
	attempt.Success = true
	if err := s.cfg.Store.ResetLoginFailures(ctx, u.ID); err != nil || s.startSession(c, u.ID) != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not sign in"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user": viewOf(u)}})
}

func lockedResponse(c *gin.Context, until int64, now time.Time) {
	minutes := (until - now.UnixMilli() + 59_999) / 60_000
	c.JSON(http.StatusLocked, gin.H{"success": false, "code": "locked", "minutes": minutes,
		"message": "Too many failed sign-ins. This account is locked for {{minutes}} more minutes."})
}

func (s *Server) logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if hash := c.GetString("session"); hash != "" {
		_ = s.cfg.Store.DeleteSession(c.Request.Context(), hash)
	}
	s.setSessionCookie(c, "", -1)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) changePassword(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	u, ok := requireUser(c)
	if !ok || !s.requireHTTPS(c) {
		return
	}
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if common.DecodeJson(c.Request.Body, &in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	if !auth.CheckPassword(u.PasswordHash, in.OldPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "The current password is incorrect"})
		return
	}
	if err := auth.ValidatePassword(u.Username, in.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	hash, err := auth.HashPassword(in.NewPassword)
	if err != nil || s.cfg.Store.SetPassword(c.Request.Context(), u.ID, hash, c.GetString("session")) != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not change the password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
