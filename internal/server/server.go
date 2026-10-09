// Package server wires the HTTP API and the embedded web UI.
package server

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"model-check/internal/seo"
	"model-check/internal/ssrf"
	"model-check/internal/store"
)

type Config struct {
	Store        *store.Store
	AllowPrivate bool     // permit loopback/private upstreams (local development only)
	Web          fs.FS    // built frontend (index.html at the root), may be nil
	Trusted      []string // trusted proxy CIDRs for client IP detection
	// VerifyBaseURL is the origin of the official OpenAI API used for image
	// provenance checks. Empty means https://api.openai.com; set it only to
	// point tests at a fake service.
	VerifyBaseURL string
	// SiteURL is the public origin (https://example.com) used for canonical
	// links and the sitemap. Empty derives it from the request.
	SiteURL string
	// RequireHTTPS refuses sign-in, registration and saving keys over plain
	// HTTP. X-Forwarded-Proto is believed only from Trusted proxies.
	RequireHTTPS bool
	// Cloudflare believes CF-Connecting-IP when the trusted proxy reports a
	// Cloudflare edge address as the client.
	Cloudflare bool
}

type Server struct {
	cfg         Config
	active      sync.Map // per-client-IP running check
	claudeSlots chan struct{}
	openaiSlots chan struct{}
	imageSlots  chan struct{}
	geminiSlots chan struct{}
	trusted     []*net.IPNet
	// credBusy holds the saved keys in use: one check per key at a time.
	credBusy sync.Map
	// Background checks (retests with a saved key, schedules) have their own
	// slots, separate from the manual ones above.
	bgSlots  map[string]chan struct{}
	bgCtx    context.Context
	bgCancel context.CancelFunc
	bgWG     sync.WaitGroup
	now      func() time.Time
}

const ownerCookie = "mc_owner"

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, claudeSlots: make(chan struct{}, 2), openaiSlots: make(chan struct{}, 2), imageSlots: make(chan struct{}, 2), geminiSlots: make(chan struct{}, 2),
		trusted: parseNets(cfg.Trusted), now: time.Now, bgSlots: map[string]chan struct{}{}}
	for _, p := range []string{"claude", "openai", "gemini", "image"} {
		s.bgSlots[p] = make(chan struct{}, 2)
	}
	s.bgCtx, s.bgCancel = context.WithCancel(context.Background())
	return s
}

func (s *Server) newClient() *ssrf.Client { return ssrf.New(s.cfg.AllowPrivate) }

func (s *Server) Handler() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	_ = r.SetTrustedProxies(s.cfg.Trusted)
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/api/model_check"})))

	api := r.Group("/api", s.owner(), s.session())
	api.GET("/status", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if err := s.cfg.Store.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	checks := api.Group("/model_check")
	checks.POST("", s.checkClaude)
	checks.POST("/openai", s.checkOpenAI)
	checks.POST("/image", s.checkImage)
	checks.POST("/gemini", s.checkGemini)
	checks.POST("/models", s.listModels)
	checks.GET("/history", s.listHistory)
	checks.GET("/history/:id", s.getHistory)
	checks.PATCH("/history/:id/remark", s.updateRemark)
	checks.GET("/baselines", s.listBaselines)
	checks.GET("/stats", s.getStats)
	checks.GET("/boards", s.listBoards)
	checks.GET("/boards/:model", s.getBoard)
	checks.POST("/baselines", s.createBaseline)

	// Accounts, saved keys, schedules and background checks. Every
	// state-changing request here must come from this site.
	acct := api.Group("", s.sameSite())
	acct.GET("/auth/me", s.authMe)
	acct.POST("/auth/register", s.register)
	acct.POST("/auth/login", s.login)
	acct.POST("/auth/logout", s.logout)
	acct.POST("/auth/password", s.changePassword)
	acct.POST("/auth/claim", s.claimRuns)
	acct.GET("/credentials", s.listCredentials)
	acct.POST("/credentials", s.createCredential)
	acct.POST("/credentials/:id/renew", s.renewCredential)
	acct.POST("/credentials/:id/resume", s.resumeCredential)
	acct.DELETE("/credentials/:id", s.deleteCredential)
	acct.GET("/schedules", s.listSchedules)
	acct.POST("/schedules", s.createSchedule)
	acct.PATCH("/schedules/:id", s.updateSchedule)
	acct.DELETE("/schedules/:id", s.deleteSchedule)
	acct.GET("/schedules/:id/runs", s.listScheduleRuns)
	acct.POST("/jobs/retest", s.retest)
	acct.GET("/jobs/running", s.runningJobs)

	// Badge endpoints are public and carry no cookies. They are served only to
	// pages of the badge's own domain (see badge.go).
	r.GET("/api/badge/:domain", s.badgeJSON)
	r.GET("/badge/:file", s.badgeSVG)
	r.GET("/embed/:file", s.embedScript)

	if s.cfg.Web != nil {
		fileServer := http.FileServer(http.FS(s.cfg.Web))
		indexHTML, _ := fs.ReadFile(s.cfg.Web, "index.html")
		r.GET("/robots.txt", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/plain; charset=utf-8", seo.Robots(seo.SiteURL(s.cfg.SiteURL, c.Request)))
		})
		r.GET("/sitemap.xml", func(c *gin.Context) {
			entries := s.sitemapEntries(c.Request.Context())
			c.Header("Cache-Control", "public, max-age=600")
			c.Data(http.StatusOK, "application/xml; charset=utf-8", seo.Sitemap(seo.SiteURL(s.cfg.SiteURL, c.Request), entries))
		})
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
				return
			}
			name := strings.TrimPrefix(c.Request.URL.Path, "/")
			if name != "" && name != "index.html" {
				if f, err := s.cfg.Web.Open(name); err == nil {
					f.Close()
					if strings.HasPrefix(name, "static/") {
						c.Header("Cache-Control", "public, max-age=31536000, immutable")
					}
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
			}
			if m := c.Request.Method; m != http.MethodGet && m != http.MethodHead {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
				return
			}
			// Every app route gets its own <head> and plain-HTML body so crawlers
			// and link previews see real content; unknown paths answer 404.
			page, known := s.seoPage(c)
			status := http.StatusOK
			if !known {
				status = http.StatusNotFound
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(status, "text/html; charset=utf-8", seo.Render(indexHTML, seo.SiteURL(s.cfg.SiteURL, c.Request), page))
		})
	}
	return r
}

// owner gives each browser a random, opaque ID. It is not authentication; it
// only lets the browser that ran a check edit that report's remark.
func (s *Server) owner() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := c.Cookie(ownerCookie)
		if err != nil || uuid.Validate(id) != nil {
			id = uuid.NewString()
			http.SetCookie(c.Writer, &http.Cookie{Name: ownerCookie, Value: id, Path: "/", MaxAge: 365 * 24 * 3600,
				HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"})
		}
		c.Set("owner", id)
		c.Next()
	}
}

func ownerID(c *gin.Context) string { return c.GetString("owner") }

// acquire enforces one running check per client IP and a global concurrency
// limit. The returned func releases both; ok=false means a response was sent.
func (s *Server) acquire(c *gin.Context, kind string, slots chan struct{}) (release func(), ok bool) {
	key := kind + ":" + s.clientIP(c)
	if _, loaded := s.active.LoadOrStore(key, true); loaded {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "A model check is already running from your address"})
		return nil, false
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots; s.active.Delete(key) }, true
	default:
		s.active.Delete(key)
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Model checks are busy. Try again later."})
		return nil, false
	}
}

// startKeepalive writes SSE comments so reverse proxies do not treat a long
// inference as an idle connection.
func startKeepalive(parent context.Context, interval time.Duration, write func([]byte) error) func() {
	ctx, cancel := context.WithCancel(parent)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if write([]byte(": keep-alive\n\n")) != nil {
					return
				}
			}
		}
	}()
	return func() { cancel(); <-finished }
}
