package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"model-check/internal/board"
	"model-check/internal/seo"
)

const (
	snapshotTTL   = time.Minute
	snapshotRuns  = 5000 // newest scored runs the boards are built from
	boardRows     = 10   // rows of one model's board
	homeBoards    = 3    // models with a board on the home page
	homeBoardRows = 5
)

// snapshot is the board data derived from the newest scored runs. It is
// rebuilt at most once per snapshotTTL, so a crawler walking the site does not
// turn every page view into a table scan.
type snapshot struct {
	at        time.Time
	runs      []board.Run
	byModel   map[string][]board.Run
	models    []modelCount // most-checked first
	directory []board.Site // newest check first
	checks    int64
	lastAt    int64
}

type modelCount struct {
	Model string `json:"model"`
	Count int    `json:"count"`
}

type snapCache struct {
	mu   sync.Mutex
	snap *snapshot
}

func (s *Server) snapshot(ctx context.Context) (*snapshot, error) {
	s.snaps.mu.Lock()
	defer s.snaps.mu.Unlock()
	if sn := s.snaps.snap; sn != nil && time.Since(sn.at) < snapshotTTL {
		return sn, nil
	}
	runs, err := s.cfg.Store.ListScoredRuns(ctx, snapshotRuns)
	if err != nil {
		return nil, err
	}
	checks, err := s.cfg.Store.CountCompleted(ctx)
	if err != nil {
		return nil, err
	}
	sn := &snapshot{at: time.Now(), byModel: map[string][]board.Run{}, checks: checks}
	for _, r := range runs {
		br := board.Run{ID: r.ID, Site: r.ChannelName, Model: r.ModelName, Endpoint: r.Endpoint, Transport: r.Transport, Score: r.Score, StartedAt: r.StartedAt}
		sn.runs = append(sn.runs, br)
		sn.byModel[br.Model] = append(sn.byModel[br.Model], br)
		if br.StartedAt > sn.lastAt {
			sn.lastAt = br.StartedAt
		}
	}
	for m, rs := range sn.byModel {
		sn.models = append(sn.models, modelCount{m, len(rs)})
	}
	sort.Slice(sn.models, func(i, j int) bool {
		if sn.models[i].Count != sn.models[j].Count {
			return sn.models[i].Count > sn.models[j].Count
		}
		return sn.models[i].Model < sn.models[j].Model
	})
	sn.directory = board.Directory(sn.runs, sn.at)
	s.snaps.snap = sn
	return sn, nil
}

func (sn *snapshot) board(model string, limit int) []board.Entry {
	return board.Build(sn.byModel[model], sn.at, limit)
}

type statsView struct {
	Checks        int64 `json:"checks"`
	Sites         int   `json:"sites"`
	Models        int   `json:"models"`
	LastCheckedAt int64 `json:"last_checked_at"`
}

func (sn *snapshot) stats() statsView {
	return statsView{Checks: sn.checks, Sites: len(sn.directory), Models: len(sn.models), LastCheckedAt: sn.lastAt}
}

type boardView struct {
	Model   string        `json:"model"`
	Checks  int           `json:"checks"`
	Entries []board.Entry `json:"entries"`
}

func (sn *snapshot) boards(n, rows int) []boardView {
	out := []boardView{}
	for i, m := range sn.models {
		if i >= n {
			break
		}
		out = append(out, boardView{Model: m.Model, Checks: m.Count, Entries: sn.board(m.Model, rows)})
	}
	return out
}

func queryInt(c *gin.Context, key string, def, min, max int) int {
	v, err := strconv.Atoi(c.Query(key))
	if err != nil || v < min {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func (s *Server) getStats(c *gin.Context) {
	sn, err := s.snapshot(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sn.stats()})
}

func (s *Server) listBoards(c *gin.Context) {
	sn, err := s.snapshot(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sn.boards(queryInt(c, "limit", homeBoards, 1, 50), queryInt(c, "size", homeBoardRows, 1, 50))})
}

func (s *Server) getBoard(c *gin.Context) {
	sn, err := s.snapshot(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false})
		return
	}
	model := c.Param("model")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": boardView{Model: model, Checks: len(sn.byModel[model]), Entries: sn.board(model, queryInt(c, "size", boardRows, 1, 100))}})
}

// zh labels for the plain-HTML boards, matching the app.
func zhTier(t string) string {
	switch t {
	case "conformant":
		return "符合"
	case "mostly":
		return "基本符合"
	}
	return "建议核对"
}

func zhSource(s string) string {
	switch s {
	case "official":
		return "官方"
	case "aws":
		return "AWS Bedrock"
	}
	return "其他"
}

func dayOf(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02") }

func boardRowsOf(es []board.Entry) []seo.BoardRow {
	out := make([]seo.BoardRow, 0, len(es))
	for _, e := range es {
		name := e.Site
		if name == "" {
			name = e.Host
		}
		out = append(out, seo.BoardRow{Rank: e.Rank, Site: name, Host: e.Host, Verdict: zhTier(e.Tier), Source: zhSource(e.Source), Date: dayOf(e.CheckedAt), ReportID: e.ReportID, Score: e.Score, Fresh: e.Fresh})
	}
	return out
}

func boardBlocksOf(bs []boardView) []seo.BoardBlock {
	out := make([]seo.BoardBlock, 0, len(bs))
	for _, b := range bs {
		blk := seo.BoardBlock{Model: b.Model, Rows: boardRowsOf(b.Entries)}
		if p, ok := seo.ModelPath(b.Model); ok {
			blk.Path = p
		}
		out = append(out, blk)
	}
	return out
}
