package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/mcpserver"
)

type Server struct {
	r    *gin.Engine
	m    *backend.Manager
	host string
	port int
}

type streamChatReq struct {
	BackendID string            `json:"backend_id"`
	Messages  []backend.Message `json:"messages"`
}

func New(m *backend.Manager, host string, port int) *Server {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Type"},
		AllowCredentials: false,
		MaxAge:           12 * 3600,
	}))

	s := &Server{r: r, m: m, host: host, port: port}
	s.mountRoutes()
	return s
}

func (s *Server) mountRoutes() {
	s.r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"host":   s.host,
			"port":   s.port,
		})
	})

	s.r.GET("/api/backends", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"backends": s.m.ListBackends()})
	})

	s.r.POST("/api/chat/stream", s.handleStreamChat)

	mcpHandler := gin.WrapH(mcpserver.NewHTTPHandler())
	s.r.Any("/api/mcp", mcpHandler)
	s.r.Any("/api/mcp/*path", mcpHandler)
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	return s.r.Run(addr)
}

func (s *Server) Handler() http.Handler {
	return s.r
}

func (s *Server) handleStreamChat(c *gin.Context) {
	var req streamChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages must not be empty"})
		return
	}

	for _, msg := range req.Messages {
		if !validRole(msg.Role) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role: " + msg.Role})
			return
		}
		if strings.TrimSpace(msg.Content) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message content must not be empty"})
			return
		}
	}

	adapter, err := s.m.Pick(req.BackendID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	emit := func(event string, payload any) error {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := c.Writer.WriteString("event: " + event + "\n"); err != nil {
			return err
		}
		if _, err := c.Writer.WriteString("data: " + string(b) + "\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := adapter.StreamChat(c.Request.Context(), backend.StreamRequest{Messages: req.Messages}, emit); err != nil {
		_ = emit("error", map[string]string{"message": err.Error()})
	}
}

func validRole(role string) bool {
	switch role {
	case "system", "user", "assistant":
		return true
	default:
		return false
	}
}
