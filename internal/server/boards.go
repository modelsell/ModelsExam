package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// The public result boards and live totals were built from every visitor's
// check records. Records are now private, so these endpoints answer empty data
// for older cached clients instead of 404.

type statsView struct {
	Checks        int64 `json:"checks"`
	Sites         int   `json:"sites"`
	Models        int   `json:"models"`
	LastCheckedAt int64 `json:"last_checked_at"`
}

func (s *Server) getStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": statsView{}})
}

func (s *Server) listBoards(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
}

func (s *Server) getBoard(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"model": c.Param("model"), "checks": 0, "entries": []any{}}})
}
