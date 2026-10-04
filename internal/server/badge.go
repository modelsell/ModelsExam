package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"model-check/internal/badge"
	"model-check/internal/seo"
)

// fromThisService reports whether a URL (an Origin or Referer value) is a page
// of this service itself, such as the badge preview.
func fromThisService(raw string, r *http.Request) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.Host == r.Host
}

func (s *Server) badgeDomain(c *gin.Context, raw string) (string, bool) {
	domain, err := badge.NormalizeDomain(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return "", false
	}
	return domain, true
}

func (s *Server) badgeResult(c *gin.Context, domain string) (badge.Result, bool) {
	runs, err := s.cfg.Store.ListCompletedByDomain(c.Request.Context(), domain, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Could not load the badge"})
		return badge.Result{}, false
	}
	items := make([]badge.Run, 0, len(runs))
	for _, r := range runs {
		items = append(items, badge.Run{ID: r.ID, Model: r.ModelName, Transport: r.Transport, Endpoint: r.Endpoint, Score: r.Score, StartedAt: r.StartedAt})
	}
	return badge.Pick(items, domain, time.Now()), true
}

// badgeJSON answers a browser only when its Origin is a page of the badge's
// domain (or this service), so a badge cannot be loaded onto another site.
func (s *Server) badgeJSON(c *gin.Context) {
	domain, ok := s.badgeDomain(c, c.Param("domain"))
	if !ok {
		return
	}
	c.Header("Vary", "Origin")
	if origin := c.GetHeader("Origin"); origin != "" {
		if !badge.OriginAllowed(origin, domain) && !fromThisService(origin, c.Request) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "This badge is not issued for this site"})
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
	}
	res, ok := s.badgeResult(c, domain)
	if !ok {
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// badgeSVG serves /badge/<domain>.svg for places that cannot run a script.
// An image request that names another site as its Referer gets a grey
// "not issued" badge instead.
func (s *Server) badgeSVG(c *gin.Context) {
	name := c.Param("file")
	if !strings.HasSuffix(name, ".svg") {
		c.Status(http.StatusNotFound)
		return
	}
	domain, ok := s.badgeDomain(c, strings.TrimSuffix(name, ".svg"))
	if !ok {
		return
	}
	c.Header("Vary", "Referer")
	c.Header("Content-Security-Policy", "default-src 'none'")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	if ref := c.GetHeader("Referer"); ref != "" && !badge.OriginAllowed(ref, domain) && !fromThisService(ref, c.Request) {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", badge.MismatchSVG(domain, badge.ThemeOf(c.Query("theme"))))
		return
	}
	res, ok := s.badgeResult(c, domain)
	if !ok {
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", badge.SVG(res, badge.ThemeOf(c.Query("theme"))))
}

// embedScript serves /embed/<domain>.js, the widget for one domain.
func (s *Server) embedScript(c *gin.Context) {
	name := c.Param("file")
	if !strings.HasSuffix(name, ".js") {
		c.Status(http.StatusNotFound)
		return
	}
	domain, ok := s.badgeDomain(c, strings.TrimSuffix(name, ".js"))
	if !ok {
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", badge.Script(domain, seo.SiteURL(s.cfg.SiteURL, c.Request)))
}
