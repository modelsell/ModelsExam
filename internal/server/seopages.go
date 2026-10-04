package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"model-check/internal/badge"
	"model-check/internal/seo"
	"model-check/internal/store"
)

const (
	homeRecords   = 20
	homeModels    = 12
	homeSites     = 12
	listLimit     = 100 // records on a site or model page
	sitemapLimit  = 1000
	maxRecordPage = 10000
)

func reportInfo(run store.Run) seo.ReportInfo {
	return seo.ReportInfo{
		ID: run.ID, SiteName: run.ChannelName, Description: run.SiteDescription, Model: run.ModelName,
		Endpoint: run.Endpoint, Status: run.Status, Score: run.Score, Transport: run.Transport,
		StartedAt: run.StartedAt, Requests: run.RequestCount, Passed: run.PassCount, Failed: run.FailCount,
		DurationMS: run.DurationMS,
	}
}

func recordsOf(runs []store.Run) []seo.Record {
	out := make([]seo.Record, 0, len(runs))
	for _, run := range runs {
		info := reportInfo(run)
		info.Status = "completed" // only indexable (completed) runs are listed
		out = append(out, seo.RecordOf(info))
	}
	return out
}

// seoPage returns the page (head tags and plain-HTML body) for the requested
// path, filled from the database where the page lists records. known=false
// means the caller should answer 404.
func (s *Server) seoPage(c *gin.Context) (seo.Page, bool) {
	ctx := c.Request.Context()
	path := strings.TrimRight(c.Request.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	site := seo.SiteURL(s.cfg.SiteURL, c.Request)
	page, known := seo.Lookup(path)
	if !known {
		return page, false
	}
	switch {
	case path == "/":
		s.fillHome(ctx, &page)
	case path == "/models":
		if sn, err := s.snapshot(ctx); err == nil {
			page = seo.BoardsIndex(site, boardBlocksOf(sn.boards(50, boardRows)))
		}
	case path == "/records":
		n := 1
		if raw := c.Query("page"); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 || v > maxRecordPage {
				return page, false
			}
			n = v
		}
		runs, total, err := s.cfg.Store.ListIndexablePage(ctx, n, seo.RecordsPerPage)
		if err != nil {
			return page, true
		}
		pages := int((total + seo.RecordsPerPage - 1) / seo.RecordsPerPage)
		if pages < 1 {
			pages = 1
		}
		if n > pages {
			return page, false
		}
		page = seo.RecordsPage(site, n, pages, recordsOf(runs))
	case strings.HasPrefix(path, "/reports/"):
		return s.reportSEO(ctx, site, path, page)
	case strings.HasPrefix(path, "/sites/"):
		domain := strings.ToLower(strings.TrimPrefix(path, "/sites/"))
		runs, err := s.cfg.Store.ListIndexableBySite(ctx, domain, listLimit)
		if err != nil {
			return page, true
		}
		exact := runs[:0]
		for _, run := range runs {
			if badge.HostMatches(run.Endpoint, domain) {
				exact = append(exact, run)
			}
		}
		page = seo.SitePage(site, domain, recordsOf(exact))
	case strings.HasPrefix(path, "/models/"):
		model := strings.TrimPrefix(path, "/models/")
		runs, err := s.cfg.Store.ListIndexableByModel(ctx, model, listLimit)
		if err != nil {
			return page, true
		}
		if len(runs) == 0 {
			return page, false
		}
		var rows []seo.BoardRow
		if sn, err := s.snapshot(ctx); err == nil {
			rows = boardRowsOf(sn.board(model, boardRows))
		}
		page = seo.ModelPage(site, model, recordsOf(runs), rows)
	}
	return page, true
}

// fillHome adds the record list and the links that let a crawler reach model
// and site pages from the home page.
func (s *Server) fillHome(ctx context.Context, page *seo.Page) {
	runs, _, err := s.cfg.Store.ListIndexablePage(ctx, 1, homeRecords)
	if err != nil {
		return
	}
	page.Records = recordsOf(runs)
	page.MaxRecords = homeRecords
	page.RecordsHead = "最近的检测记录"
	page.FAQ = seo.FAQ
	if sn, err := s.snapshot(ctx); err == nil {
		st := sn.stats()
		if st.Checks > 0 {
			page.Stats = fmt.Sprintf("已完成 %d 次检测，覆盖 %d 个站点、%d 个模型。数据来自公开检测记录，实时统计。", st.Checks, st.Sites, st.Models)
		}
		page.Boards = boardBlocksOf(sn.boards(homeBoards, homeBoardRows))
	}
	seen := map[string]bool{}
	var sites []seo.Link
	for _, run := range runs {
		sp, ok := seo.SitePath(run.Endpoint)
		if !ok || seen[sp] || len(sites) >= homeSites {
			continue
		}
		seen[sp] = true
		sites = append(sites, seo.Link{Path: sp, Label: run.ChannelName + "（" + seo.HostOf(run.Endpoint) + "）"})
	}
	var models []seo.Link
	if top, err := s.cfg.Store.TopIndexableModels(ctx, homeModels); err == nil {
		for _, m := range top {
			if p, ok := seo.ModelPath(m.Model); ok {
				models = append(models, seo.Link{Path: p, Label: fmt.Sprintf("%s（%d 次检测）", m.Model, m.N)})
			}
		}
	}
	page.LinkGroups = []seo.LinkGroup{
		{Heading: "已检测的模型", Links: models},
		{Heading: "最近检测的站点", Links: sites},
	}
}

// reportSEO fills a report page's head and body from the stored run: the
// tested site's name and description become the title and description search
// engines show. An unknown report id answers 404.
func (s *Server) reportSEO(ctx context.Context, site, path string, fallback seo.Page) (seo.Page, bool) {
	id := strings.TrimPrefix(path, "/reports/")
	if uuid.Validate(id) != nil {
		return fallback, false
	}
	run, err := s.cfg.Store.GetRun(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fallback, false
	}
	if err != nil {
		return fallback, true
	}
	return seo.ReportPage(site, reportInfo(*run)), true
}

// sitemapEntries lists every public report, the site pages and the model pages
// that have at least one indexable record.
func (s *Server) sitemapEntries(ctx context.Context) []seo.Entry {
	runs, err := s.cfg.Store.ListIndexableRuns(ctx, sitemapLimit)
	if err != nil {
		return nil
	}
	var entries []seo.Entry
	sites, models := map[string]int64{}, map[string]int64{}
	var siteOrder, modelOrder []string
	for _, run := range runs {
		entries = append(entries, seo.Entry{Path: "/reports/" + run.ID, LastMod: run.UpdatedAt})
		if sp, ok := seo.SitePath(run.Endpoint); ok {
			if _, seen := sites[sp]; !seen {
				siteOrder = append(siteOrder, sp)
			}
			if run.UpdatedAt > sites[sp] {
				sites[sp] = run.UpdatedAt
			}
		}
		if p, ok := seo.ModelPath(run.ModelName); ok {
			if _, seen := models[p]; !seen {
				modelOrder = append(modelOrder, p)
			}
			if run.UpdatedAt > models[p] {
				models[p] = run.UpdatedAt
			}
		}
	}
	for _, sp := range siteOrder {
		entries = append(entries, seo.Entry{Path: sp, LastMod: sites[sp]})
	}
	for _, p := range modelOrder {
		entries = append(entries, seo.Entry{Path: p, LastMod: models[p]})
	}
	return entries
}
