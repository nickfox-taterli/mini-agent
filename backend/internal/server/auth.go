package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const tokenMaxAge = 24 * time.Hour

// tokenStore 在内存中维护单用户的 Bearer token.
type tokenStore struct {
	mu       sync.Mutex
	token    string
	lastSeen time.Time
}

func newTokenStore() *tokenStore {
	ts := &tokenStore{}
	go ts.cleanupLoop()
	return ts
}

func (ts *tokenStore) cleanupLoop() {
	for range time.Tick(10 * time.Minute) {
		ts.mu.Lock()
		if ts.token != "" && time.Since(ts.lastSeen) > tokenMaxAge {
			ts.token = ""
		}
		ts.mu.Unlock()
	}
}

// issue 生成新 token 并使其生效 (旧 token 立即失效).
func (ts *tokenStore) issue() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	ts.mu.Lock()
	ts.token = token
	ts.lastSeen = time.Now()
	ts.mu.Unlock()
	return token
}

// valid 检查 token 是否有效, 有效时刷新最后活跃时间.
func (ts *tokenStore) valid(token string) bool {
	if token == "" {
		return false
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.token == token {
		ts.lastSeen = time.Now()
		return true
	}
	return false
}

// authMiddleware 返回要求 Bearer token 的 Gin 中间件.
func authMiddleware(ts *tokenStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if !ts.valid(token) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// handleLogin 返回处理登录请求的 handler.
func handleLogin(ts *tokenStore, password string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if req.Password != password {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong password"})
			return
		}
		token := ts.issue()
		c.JSON(http.StatusOK, gin.H{"token": token})
	}
}
