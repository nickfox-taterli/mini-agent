package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/server"
)

func TestMCPGetSystemTime(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "mock-main",
			Type:    "openai_compatible",
			BaseURL: "http://127.0.0.1:1",
			APIKey:  "test",
			Model:   "mock",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get_system_time",
			"arguments": map[string]any{},
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, body=%s", err, w.Body.String())
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result field: %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structuredContent field: %v", result)
	}

	if _, ok := structured["now_unix"]; !ok {
		t.Fatalf("missing now_unix: %v", structured)
	}
	if _, ok := structured["now_rfc3339"]; !ok {
		t.Fatalf("missing now_rfc3339: %v", structured)
	}
	if _, ok := structured["timezone_name"]; !ok {
		t.Fatalf("missing timezone_name: %v", structured)
	}
}

func TestMCPRunSkillBash(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "mock-main",
			Type:    "openai_compatible",
			BaseURL: "http://127.0.0.1:1",
			APIKey:  "test",
			Model:   "mock",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	fileName := "tests/skill-bash.txt"
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "run_skill_bash",
			"arguments": map[string]any{
				"skill_name":      "minimax-xlsx",
				"timeout_seconds": 30,
				"command":         "printf 'ok-from-skill-bash' > \"$FRONTEND_TMP_DIR/" + fileName + "\"",
			},
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, body=%s", err, w.Body.String())
	}
	result, _ := resp["result"].(map[string]any)
	structured, _ := result["structuredContent"].(map[string]any)
	frontendTmpDir, _ := structured["frontend_tmp_dir"].(string)
	if frontendTmpDir == "" {
		t.Fatalf("missing frontend_tmp_dir in response: %v", structured)
	}
	targetPath := filepath.Join(frontendTmpDir, fileName)
	defer os.Remove(targetPath)

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(content) != "ok-from-skill-bash" {
		t.Fatalf("unexpected file content: %s", string(content))
	}
}

func TestMCPWebFetch(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "mock-main",
			Type:    "openai_compatible",
			BaseURL: "http://127.0.0.1:1",
			APIKey:  "test",
			Model:   "mock",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	// 启动一个临时 HTTP 服务器作为抓取目标
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from web fetch"))
	}))
	defer ts.Close()

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "web_fetch",
			"arguments": map[string]any{
				"url":             ts.URL + "/test",
				"timeout_seconds": 10,
			},
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, body=%s", err, w.Body.String())
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result field: %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structuredContent field: %v", result)
	}

	if structured["content"] != "hello from web fetch" {
		t.Fatalf("unexpected content: %v", structured["content"])
	}
	if structured["status_code"] != float64(200) {
		t.Fatalf("unexpected status_code: %v", structured["status_code"])
	}
}

func TestMCPWebFetchInvalidURL(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "mock-main",
			Type:    "openai_compatible",
			BaseURL: "http://127.0.0.1:1",
			APIKey:  "test",
			Model:   "mock",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "web_fetch",
			"arguments": map[string]any{
				"url": "not-a-valid-url",
			},
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, body=%s", err, w.Body.String())
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result field: %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structuredContent field: %v", result)
	}

	if structured["error"] == "" {
		t.Fatalf("expected error for invalid url, got: %v", structured)
	}
}

func TestMCPConvertLocalPathToURL(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "mock-main",
			Type:    "openai_compatible",
			BaseURL: "http://127.0.0.1:1",
			APIKey:  "test",
			Model:   "mock",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	localPath, err := filepath.Abs(filepath.Join("..", "..", "frontend", "upload", "2026", "04", "14", "report.xlsx"))
	if err != nil {
		t.Fatalf("build local path: %v", err)
	}

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      6,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "convert_local_path_to_url",
			"arguments": map[string]any{
				"local_path": localPath,
			},
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, body=%s", err, w.Body.String())
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result field: %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structuredContent field: %v", result)
	}

	url, _ := structured["url"].(string)
	if !strings.HasPrefix(url, "http://127.0.0.1:18889/upload/2026/04/14/") {
		t.Fatalf("unexpected url: %v", url)
	}
}
