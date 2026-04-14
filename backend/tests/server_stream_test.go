package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/mcpserver"
	"taterli-agent-chat/backend/internal/server"
)

func TestStreamChat_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"text\":\"思\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"text\":\"思考\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:             "minimax-main",
			Type:           "openai_compatible",
			BaseURL:        upstream.URL,
			APIKey:         "test",
			Model:          "MiniMax-M2.7",
			Temperature:    1.0,
			ReasoningSplit: true,
			Enabled:        true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, `event: reasoning`) {
		t.Fatalf("missing reasoning event: %s", respBody)
	}
	if !strings.Contains(respBody, `{"delta":"思"}`) || !strings.Contains(respBody, `{"delta":"考"}`) {
		t.Fatalf("reasoning delta mismatch: %s", respBody)
	}
	if !strings.Contains(respBody, `event: content`) {
		t.Fatalf("missing content event: %s", respBody)
	}
	if !strings.Contains(respBody, `{"delta":"你"}`) || !strings.Contains(respBody, `{"delta":"好"}`) {
		t.Fatalf("content delta mismatch: %s", respBody)
	}
	if !strings.Contains(respBody, `event: done`) {
		t.Fatalf("missing done event: %s", respBody)
	}
}

func TestStreamChat_ConversationLockConflict(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "minimax-main",
			Type:    "openai_compatible",
			BaseURL: upstream.URL,
			APIKey:  "test",
			Model:   "MiniMax-M2.7",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"conversation_id": "conv-lock",
		"messages":        []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	doneFirst := make(chan struct{})
	go func() {
		defer close(doneFirst)
		req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("first stream expected 200, got %d", w.Code)
		}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting first stream to start")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("second stream expected 409, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "conversation_streaming") {
		t.Fatalf("expected conversation_streaming code, got: %s", w2.Body.String())
	}

	close(release)
	<-doneFirst
}

func TestConversationState_StreamingFlag(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "minimax-main",
			Type:    "openai_compatible",
			BaseURL: upstream.URL,
			APIKey:  "test",
			Model:   "MiniMax-M2.7",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"conversation_id": "conv-state",
		"messages":        []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	doneFirst := make(chan struct{})
	go func() {
		defer close(doneFirst)
		req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("stream expected 200, got %d", w.Code)
		}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting stream to start")
	}

	reqState := httptest.NewRequest(http.MethodGet, "/api/conversations/conv-state/state", nil)
	wState := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wState, reqState)
	if wState.Code != http.StatusOK {
		t.Fatalf("state endpoint expected 200, got %d", wState.Code)
	}
	if !strings.Contains(wState.Body.String(), `"is_streaming":true`) {
		t.Fatalf("expected is_streaming true, got %s", wState.Body.String())
	}

	close(release)
	<-doneFirst

	reqStateDone := httptest.NewRequest(http.MethodGet, "/api/conversations/conv-state/state", nil)
	wStateDone := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wStateDone, reqStateDone)
	if wStateDone.Code != http.StatusOK {
		t.Fatalf("state endpoint expected 200, got %d", wStateDone.Code)
	}
	if !strings.Contains(wStateDone.Body.String(), `"is_streaming":false`) {
		t.Fatalf("expected is_streaming false, got %s", wStateDone.Body.String())
	}
}

func TestConversationState_ReleaseOnClientCancel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer upstream.CloseClientConnections()
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "minimax-main",
			Type:    "openai_compatible",
			BaseURL: upstream.URL,
			APIKey:  "test",
			Model:   "MiniMax-M2.7",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"conversation_id": "conv-cancel",
		"messages":        []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	ctx, cancel := context.WithCancel(context.Background())
	doneReq := make(chan struct{})
	go func() {
		defer close(doneReq)
		req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
	}()

	var locked bool
	for i := 0; i < 20; i++ {
		reqState := httptest.NewRequest(http.MethodGet, "/api/conversations/conv-cancel/state", nil)
		wState := httptest.NewRecorder()
		srv.Handler().ServeHTTP(wState, reqState)
		if strings.Contains(wState.Body.String(), `"is_streaming":true`) {
			locked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !locked {
		t.Fatal("expected conversation lock to be active before cancel")
	}

	cancel()

	var unlocked bool
	for i := 0; i < 50; i++ {
		reqState := httptest.NewRequest(http.MethodGet, "/api/conversations/conv-cancel/state", nil)
		wState := httptest.NewRecorder()
		srv.Handler().ServeHTTP(wState, reqState)
		if strings.Contains(wState.Body.String(), `"is_streaming":false`) {
			unlocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !unlocked {
		t.Fatal("expected conversation lock to release after cancel")
	}

	<-doneReq
}

func TestStreamChat_UpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "minimax-main",
			Type:    "openai_compatible",
			BaseURL: upstream.URL,
			APIKey:  "test",
			Model:   "MiniMax-M2.7",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	body := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, `event: error`) {
		t.Fatalf("missing error event: %s", respBody)
	}
	if !strings.Contains(respBody, `status=401`) {
		t.Fatalf("missing upstream error details: %s", respBody)
	}
}

func TestStreamChat_AttachmentURLConvertedToLocalPath(t *testing.T) {
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		upstreamBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:             "minimax-main",
			Type:           "openai_compatible",
			BaseURL:        upstream.URL,
			APIKey:         "test",
			Model:          "MiniMax-M2.7",
			Temperature:    1.0,
			ReasoningSplit: true,
			Enabled:        true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889")

	uploadDir, err := mcpserver.ResolveFrontendUploadDirExported()
	if err != nil {
		t.Fatalf("resolve upload dir: %v", err)
	}
	localPath := filepath.Join(uploadDir, "attach-test.xls")
	if err := os.WriteFile(localPath, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	defer os.Remove(localPath)
	fileURL := mcpserver.BuildFileURL(uploadDir, "attach-test.xls")

	body := map[string]any{
		"messages": []map[string]string{{
			"role":    "user",
			"content": "请看这个文件\n\n[附件: attach.xls](" + fileURL + ")",
		}},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(upstreamBody, "[本地路径:") {
		t.Fatalf("expected converted local path hint in upstream body: %s", upstreamBody)
	}
	if !strings.Contains(upstreamBody, filepath.ToSlash(localPath)) && !strings.Contains(upstreamBody, localPath) {
		t.Fatalf("expected local path in upstream body: %s", upstreamBody)
	}
}
