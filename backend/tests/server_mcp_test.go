package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/mcpserver"
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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

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

func TestMCPPythonAndCodeDockerFlow(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found")
	}
	if out, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skip("docker daemon unavailable")
	}

	workspaceRoot := t.TempDir()
	if err := mcpserver.InitDockerRuntime(mcpserver.DockerRuntimeConfig{
		Enabled:               true,
		SessionTTLSeconds:     1800,
		MaxLifetimeSeconds:    3600,
		DefaultTimeoutSeconds: 120,
		MaxTimeoutSeconds:     600,
		MemoryLimit:           "512m",
		CPULimit:              1.0,
		PidsLimit:             128,
		WorkspaceRoot:         workspaceRoot,
	}); err != nil {
		t.Skipf("init docker runtime failed: %v", err)
	}
	defer func() {
		_ = mcpserver.InitDockerRuntime(mcpserver.DockerRuntimeConfig{})
	}()

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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

	pythonSessionID := callMCPAndGetString(t, srv, "python_session_init", map[string]any{
		"python_version": "3.11",
	}, "session_id")
	defer callMCP(t, srv, "python_session_close", map[string]any{"session_id": pythonSessionID})

	pyRun := callMCP(t, srv, "python_run_code", map[string]any{
		"session_id": pythonSessionID,
		"code":       "print('hello-python')",
	})
	if pyRun["exit_code"] != float64(0) {
		t.Fatalf("python run failed: %v", pyRun)
	}
	if !strings.Contains(pyRun["stdout"].(string), "hello-python") {
		t.Fatalf("unexpected python stdout: %v", pyRun["stdout"])
	}

	codeSessionID := callMCPAndGetString(t, srv, "code_session_init", map[string]any{}, "session_id")
	defer callMCP(t, srv, "code_session_close", map[string]any{"session_id": codeSessionID})

	codeRun := callMCP(t, srv, "code_run", map[string]any{
		"session_id":  codeSessionID,
		"language":    "shell",
		"source_code": "echo hello-code",
	})
	if codeRun["exit_code"] != float64(0) {
		t.Fatalf("code run failed: %v", codeRun)
	}
	if !strings.Contains(codeRun["stdout"].(string), "hello-code") {
		t.Fatalf("unexpected code stdout: %v", codeRun["stdout"])
	}
}

func callMCPAndGetString(t *testing.T, srv *server.Server, tool string, args map[string]any, key string) string {
	t.Helper()
	out := callMCP(t, srv, tool, args)
	got, _ := out[key].(string)
	if strings.TrimSpace(got) == "" {
		t.Fatalf("missing %s in response: %v", key, out)
	}
	return got
}

func callMCP(t *testing.T, srv *server.Server, tool string, args map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      tool,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": args,
		},
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("tool=%s expected 200, got %d, body=%s", tool, w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("tool=%s unmarshal response: %v, body=%s", tool, err, w.Body.String())
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tool=%s missing result field: %v", tool, resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("tool=%s missing structuredContent field: %v", tool, result)
	}
	return structured
}
