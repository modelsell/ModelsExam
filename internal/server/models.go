package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"model-check/common"
	"model-check/internal/modellist"
)

// listModels asks the visitor's endpoint for its model list so the form can
// offer clickable choices. A failure is an ordinary answer (HTTP 200,
// success=false and a short code): the form then keeps its default models.
// The key is used for this one request and never stored or echoed.
func (s *Server) listModels(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		Kind    string `json:"kind"`
		BaseURL string `json:"base_url"`
		Key     string `json:"key"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if common.DecodeJson(c.Request.Body, &input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	input.Key = strings.TrimSpace(input.Key)
	switch input.Kind {
	case modellist.KindClaude, modellist.KindOpenAI, modellist.KindImage:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	if len(input.Key) < 4 || len(input.Key) > 8192 || strings.ContainsAny(input.Key, "\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Enter a valid API key"})
		return
	}
	// One lookup at a time per address, and a global cap, so this cannot be
	// used to fan requests out through the server.
	guard := "models:" + c.ClientIP()
	if _, busy := s.active.LoadOrStore(guard, true); busy {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "A model list request is already running"})
		return
	}
	defer s.active.Delete(guard)
	select {
	case modelListSlots <- struct{}{}:
		defer func() { <-modelListSlots }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Busy. Try again later."})
		return
	}
	client := s.newClient()
	defer client.CloseIdleConnections()
	res, err := modellist.Fetch(c.Request.Context(), client.Client, client.ValidateURL, input.BaseURL, input.Key, input.Kind)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "code": modellist.Code(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

var modelListSlots = make(chan struct{}, 16)
