package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/mcpserver"
)

type Server struct {
	r           *gin.Engine
	m           *backend.Manager
	host        string
	port        int
	frontendURL string
}

type streamChatReq struct {
	BackendID string            `json:"backend_id"`
	Messages  []backend.Message `json:"messages"`
}

func New(m *backend.Manager, host string, port int, frontendURL string) *Server {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Type"},
		AllowCredentials: false,
		MaxAge:           12 * 3600,
	}))

	mcpserver.InitConfig(frontendURL)

	s := &Server{r: r, m: m, host: host, port: port, frontendURL: frontendURL}
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

	// 文件上传接口
	s.r.POST("/api/upload", s.handleUpload)

	// 静态文件服务: /upload/*filepath -> frontend/upload/
	s.r.GET("/upload/*filepath", s.handleServeUpload)
	s.r.HEAD("/upload/*filepath", s.handleServeUpload)

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

// handleUpload 处理前端文件上传, 保存到 upload/YYYY/MM/DD/ 并返回 URL.
func (s *Server) handleUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file field"})
		return
	}
	defer file.Close()

	// 限制文件大小 (50MB)
	if header.Size > 50*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 50MB)"})
		return
	}

	uploadDir, err := mcpserver.ResolveFrontendUploadDirExported()
	if err != nil {
		log.Printf("upload: resolve dir error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// 生成随机文件名, 保留原始扩展名
	ext := filepath.Ext(header.Filename)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	randomName := hex.EncodeToString(b) + ext

	targetPath := filepath.Join(uploadDir, randomName)
	dst, err := os.Create(targetPath)
	if err != nil {
		log.Printf("upload: create file error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		log.Printf("upload: write file error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	fileURL := mcpserver.BuildFileURL(uploadDir, randomName)
	log.Printf("upload: saved %s (%d bytes) -> %s", randomName, written, fileURL)

	c.JSON(http.StatusOK, gin.H{
		"url":      fileURL,
		"filename": randomName,
		"size":     written,
	})
}

// handleServeUpload 提供上传文件的静态文件服务.
func (s *Server) handleServeUpload(c *gin.Context) {
	filePath := c.Param("filepath")
	if filePath == "" || filePath == "/" {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	uploadDir, err := mcpserver.ResolveFrontendUploadDirExported()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	fullPath := filepath.Join(uploadDir, filepath.Clean(filePath))
	// 安全检查: 确保路径没有逃逸上传目录
	if !strings.HasPrefix(fullPath, uploadDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// 设置 Content-Type
	ext := filepath.Ext(fullPath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(fullPath)))
	c.File(fullPath)
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
