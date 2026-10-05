package server

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"model-check/internal/seo"
)

// seoPage returns the page (head tags and plain-HTML body) for the requested
// path. Check records are private to the browser that ran them, so no page is
// ever filled from stored runs: report and record pages render the generic,
// noindex fallback and the app loads the visitor's own data. known=false means
// the caller should answer 404.
func (s *Server) seoPage(c *gin.Context) (seo.Page, bool) {
	path := strings.TrimRight(c.Request.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	// Model boards and their pages are gone with the public records.
	if path == "/models" || strings.HasPrefix(path, "/models/") {
		page, _ := seo.Lookup("/404")
		return page, false
	}
	page, known := seo.Lookup(path)
	if !known {
		return page, false
	}
	if path == "/" {
		page.FAQ = seo.FAQ
	}
	return page, true
}

// sitemapEntries is empty: reports, site and model pages are not public.
func (s *Server) sitemapEntries(context.Context) []seo.Entry { return nil }
