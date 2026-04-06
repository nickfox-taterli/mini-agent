package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/mcpserver"
)

const (
	maxRetries             = 30
	baseRetryDelay         = 1500 * time.Millisecond
	maxRetryDelay          = 8 * time.Second
	defaultToolMaxRounds   = 16
	upstreamConcurrencyCap = 2
)

var upstreamSlots = make(chan struct{}, upstreamConcurrencyCap)

type OpenAICompatibleAdapter struct {
	cfg               config.BackendConfig
	httpClient        *http.Client
	skillSystemPrompt string
}

func NewOpenAICompatibleAdapter(cfg config.BackendConfig, skillSystemPrompt string) *OpenAICompatibleAdapter {
	return &OpenAICompatibleAdapter{
		cfg:               cfg,
		skillSystemPrompt: strings.TrimSpace(skillSystemPrompt),
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (a *OpenAICompatibleAdapter) ID() string {
	return a.cfg.ID
}

type openAIStreamReq struct {
	Model       string         `json:"model"`
	Messages    []Message      `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
	Stream      bool           `json:"stream"`
	Tools       []openAITool   `json:"tools,omitempty"`
	ToolChoice  string         `json:"tool_choice,omitempty"`
	ExtraBody   map[string]any `json:"extra_body,omitempty"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	} `json:"function"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
			ReasoningDetails []struct {
				Text string `json:"text"`
			} `json:"reasoning_details"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]any `json:"usage"`
}

type thinkTagSplitter struct {
	inThink bool
	pending string
}

func (s *thinkTagSplitter) Feed(chunk string) (reasoning string, content string) {
	data := s.pending + chunk
	s.pending = ""

	for len(data) > 0 {
		if s.inThink {
			closeIdx := strings.Index(data, "</think>")
			if closeIdx < 0 {
				keep := partialTagSuffixLen(data, "</think>")
				cut := len(data) - keep
				if cut > 0 {
					reasoning += data[:cut]
				}
				s.pending = data[cut:]
				break
			}
			if closeIdx > 0 {
				reasoning += data[:closeIdx]
			}
			data = data[closeIdx+len("</think>"):]
			s.inThink = false
			continue
		}

		openIdx := strings.Index(data, "<think>")
		if openIdx < 0 {
			keep := partialTagSuffixLen(data, "<think>")
			cut := len(data) - keep
			if cut > 0 {
				content += data[:cut]
			}
			s.pending = data[cut:]
			break
		}

		if openIdx > 0 {
			content += data[:openIdx]
		}
		data = data[openIdx+len("<think>"):]
		s.inThink = true
	}

	return reasoning, content
}

func partialTagSuffixLen(text, tag string) int {
	max := len(tag) - 1
	if max > len(text) {
		max = len(text)
	}
	for i := max; i >= 1; i-- {
		if strings.HasSuffix(text, tag[:i]) {
			return i
		}
	}
	return 0
}

type streamRoundResult struct {
	FinishReason     string
	Usage            map[string]any
	AssistantContent string
	ToolCalls        []ToolCall
}

type upstreamErrorInfo struct {
	StatusCode int
	Retryable  bool
	Busy       bool
	Message    string
	RequestID  string
	ErrorType  string
}

func (a *OpenAICompatibleAdapter) StreamChat(ctx context.Context, req StreamRequest, emit EmitFunc) error {
	traceID := newTraceID()
	workingMessages := a.withSkillSystemPrompt(req.Messages)
	log.Printf("[trace=%s] backend=%s stream start messages=%d", traceID, a.cfg.ID, len(workingMessages))

	maxToolRounds := a.cfg.ToolMaxRounds
	if maxToolRounds <= 0 {
		maxToolRounds = defaultToolMaxRounds
	}

	for round := 0; round <= maxToolRounds; round++ {
		tools := defaultOpenAITools()
		payload := openAIStreamReq{
			Model:       a.cfg.Model,
			Messages:    workingMessages,
			Temperature: a.cfg.Temperature,
			Stream:      true,
			ExtraBody: map[string]any{
				"reasoning_split": a.cfg.ReasoningSplit,
			},
			Tools:      tools,
			ToolChoice: "auto",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		logUpstreamRequest(traceID, a.cfg.ID, round+1, payload, tools, body)

		url := strings.TrimSuffix(a.cfg.BaseURL, "/") + "/chat/completions"
		roundRes, err := a.requestAndReadRound(ctx, traceID, round+1, url, body, emit)
		if err != nil {
			log.Printf("[trace=%s] backend=%s round=%d failed err=%v", traceID, a.cfg.ID, round+1, err)
			return err
		}

		if roundRes.FinishReason != "tool_calls" || len(roundRes.ToolCalls) == 0 {
			if err := emit("done", map[string]any{"finish_reason": roundRes.FinishReason, "usage": roundRes.Usage}); err != nil {
				return err
			}
			log.Printf("[trace=%s] backend=%s stream done finish_reason=%s", traceID, a.cfg.ID, roundRes.FinishReason)
			return nil
		}

		if round == maxToolRounds {
			return fmt.Errorf("tool call rounds exceeded limit: %d", maxToolRounds)
		}

		log.Printf("[trace=%s] backend=%s round=%d tool_calls=%d", traceID, a.cfg.ID, round+1, len(roundRes.ToolCalls))
		workingMessages = append(workingMessages, Message{
			Role:      "assistant",
			Content:   roundRes.AssistantContent,
			ToolCalls: roundRes.ToolCalls,
		})

		for _, call := range roundRes.ToolCalls {
			argsPreview := call.Function.Arguments
			if len(argsPreview) > 200 {
				argsPreview = argsPreview[:200] + "..."
			}
			log.Printf("[trace=%s] tool=%s call_id=%s args=%s", traceID, call.Function.Name, call.ID, argsPreview)

			out, callErr := mcpserver.ExecuteToolByJSON(call.Function.Name, call.Function.Arguments)
			if callErr != nil {
				log.Printf("[trace=%s] tool=%s call_id=%s exec_error=%v", traceID, call.Function.Name, call.ID, callErr)
				out = map[string]any{"error": callErr.Error()}
			} else {
				resultJSON, _ := json.Marshal(out)
				resultPreview := string(resultJSON)
				if len(resultPreview) > 300 {
					resultPreview = resultPreview[:300] + "..."
				}
				log.Printf("[trace=%s] tool=%s call_id=%s exec_ok result=%s", traceID, call.Function.Name, call.ID, resultPreview)
			}
			b, _ := json.Marshal(out)
			workingMessages = append(workingMessages, Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    string(b),
			})
		}
	}

	return nil
}

func (a *OpenAICompatibleAdapter) withSkillSystemPrompt(messages []Message) []Message {
	working := append([]Message(nil), messages...)
	if a.skillSystemPrompt == "" {
		return working
	}
	mergedPrompt := a.skillSystemPrompt + "\nMCP tools are available through tool calling in this chat."
	if len(working) > 0 && working[0].Role == "system" {
		working[0].Content = strings.TrimSpace(mergedPrompt + "\n\n" + working[0].Content)
		return working
	}
	return append([]Message{{Role: "system", Content: mergedPrompt}}, working...)
}

func (a *OpenAICompatibleAdapter) requestAndReadRound(ctx context.Context, traceID string, round int, url string, body []byte, emit EmitFunc) (*streamRoundResult, error) {
	release, err := acquireUpstreamSlot(ctx, traceID, emit)
	if err != nil {
		return nil, err
	}
	defer release()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, reqErr := a.doRequest(ctx, url, body)
		if reqErr != nil {
			log.Printf("[trace=%s] round=%d attempt=%d network_err=%v", traceID, round, attempt+1, reqErr)
			if attempt < maxRetries {
				delay := computeRetryDelay(attempt, nil)
				if err := emitRetrying(emit, traceID, attempt, delay, "network_error", reqErr.Error(), 0, "", true, false); err != nil {
					return nil, err
				}
				if err := waitWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("upstream request failed after retries, trace_id=%s: %w", traceID, reqErr)
		}

		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			return a.readStream(resp, emit)
		}

		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		_ = resp.Body.Close()
		info := parseUpstreamError(resp.StatusCode, resp.Header, rawBody)
		log.Printf("[trace=%s] round=%d attempt=%d status=%d retryable=%t busy=%t error_type=%s upstream_request_id=%s message=%s", traceID, round, attempt+1, info.StatusCode, info.Retryable, info.Busy, info.ErrorType, info.RequestID, info.Message)

		if info.Retryable && attempt < maxRetries {
			delay := computeRetryDelay(attempt, resp.Header)
			if err := emitRetrying(emit, traceID, attempt, delay, info.ErrorType, info.Message, info.StatusCode, info.RequestID, info.Retryable, info.Busy); err != nil {
				return nil, err
			}
			if err := waitWithContext(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		if info.Busy {
			return nil, fmt.Errorf("upstream busy after retries, trace_id=%s, status=%d, upstream_request_id=%s: %s", traceID, info.StatusCode, info.RequestID, info.Message)
		}
		return nil, fmt.Errorf("upstream call failed, trace_id=%s, status=%d, upstream_request_id=%s: %s", traceID, info.StatusCode, info.RequestID, info.Message)
	}

	return nil, fmt.Errorf("upstream retries exhausted, trace_id=%s", traceID)
}

func (a *OpenAICompatibleAdapter) doRequest(ctx context.Context, url string, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request upstream: %w", err)
	}
	return resp, nil
}

func (a *OpenAICompatibleAdapter) readStream(resp *http.Response, emit EmitFunc) (*streamRoundResult, error) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024), 2*1024*1024)

	reasoningSeen := ""
	contentSeen := ""
	finishReason := "stop"
	var usage map[string]any
	splitter := &thinkTagSplitter{}
	contentStarted := false
	assistantContent := ""
	toolCallBuffers := map[int]*ToolCall{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			finishReason = *choice.FinishReason
		}

		if len(choice.Delta.ReasoningDetails) > 0 {
			combined := ""
			for _, item := range choice.Delta.ReasoningDetails {
				combined += item.Text
			}
			delta, nextSeen := normalizeDelta(reasoningSeen, combined)
			reasoningSeen = nextSeen
			if delta != "" {
				if err := emit("reasoning", map[string]string{"delta": delta}); err != nil {
					return nil, err
				}
			}
		}

		if choice.Delta.Content != "" {
			delta, nextSeen := normalizeDelta(contentSeen, choice.Delta.Content)
			contentSeen = nextSeen
			if delta != "" {
				decoded := decodeThinkTagEscapes(delta)
				reasoningText, contentText := splitter.Feed(decoded)
				if reasoningText != "" {
					if err := emit("reasoning", map[string]string{"delta": reasoningText}); err != nil {
						return nil, err
					}
				}
				if contentText != "" {
					if !contentStarted {
						contentText = strings.TrimLeft(contentText, "\r\n")
					}
					if contentText == "" {
						continue
					}
					contentStarted = true
					assistantContent += contentText
					if err := emit("content", map[string]string{"delta": contentText}); err != nil {
						return nil, err
					}
				}
			}
		}

		if len(choice.Delta.ToolCalls) > 0 {
			for _, item := range choice.Delta.ToolCalls {
				call := toolCallBuffers[item.Index]
				if call == nil {
					call = &ToolCall{}
					toolCallBuffers[item.Index] = call
				}
				if item.ID != "" {
					call.ID = item.ID
				}
				if item.Type != "" {
					call.Type = item.Type
				}
				if item.Function.Name != "" {
					call.Function.Name = item.Function.Name
				}
				if item.Function.Arguments != "" {
					call.Function.Arguments += item.Function.Arguments
				}
			}
		}

		if len(choice.Delta.ReasoningDetails) == 0 && choice.Delta.Content == "" && splitter.pending != "" {
			reasoningText, contentText := splitter.Feed("")
			if reasoningText != "" {
				if err := emit("reasoning", map[string]string{"delta": reasoningText}); err != nil {
					return nil, err
				}
			}
			if contentText != "" {
				if !contentStarted {
					contentText = strings.TrimLeft(contentText, "\r\n")
				}
				if contentText == "" {
					continue
				}
				contentStarted = true
				assistantContent += contentText
				if err := emit("content", map[string]string{"delta": contentText}); err != nil {
					return nil, err
				}
			}
		}
	}

	if splitter.pending != "" {
		reasoningText, contentText := splitter.Feed("")
		if reasoningText != "" {
			if err := emit("reasoning", map[string]string{"delta": reasoningText}); err != nil {
				return nil, err
			}
		}
		if contentText != "" {
			if !contentStarted {
				contentText = strings.TrimLeft(contentText, "\r\n")
			}
			if contentText != "" {
				assistantContent += contentText
				if err := emit("content", map[string]string{"delta": contentText}); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read upstream stream: %w", err)
	}

	result := &streamRoundResult{
		FinishReason:     finishReason,
		Usage:            usage,
		AssistantContent: assistantContent,
		ToolCalls:        collectToolCalls(toolCallBuffers),
	}
	return result, nil
}

func collectToolCalls(m map[int]*ToolCall) []ToolCall {
	if len(m) == 0 {
		return nil
	}
	indices := make([]int, 0, len(m))
	for idx := range m {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	out := make([]ToolCall, 0, len(indices))
	for _, idx := range indices {
		call := m[idx]
		if call == nil {
			continue
		}
		if call.Type == "" {
			call.Type = "function"
		}
		out = append(out, *call)
	}
	return out
}

func defaultOpenAITools() []openAITool {
	timeTool := openAITool{}
	timeTool.Type = "function"
	timeTool.Function.Name = "get_system_time"
	timeTool.Function.Description = "Get current system time."
	timeTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
	}

	runSkillBashTool := openAITool{}
	runSkillBashTool.Type = "function"
	runSkillBashTool.Function.Name = "run_skill_bash"
	runSkillBashTool.Function.Description = "Run bash command inside backend/skills/<skill_name>. Use this to execute skill scripts."
	runSkillBashTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"skill_name", "command"},
		"properties": map[string]any{
			"skill_name": map[string]any{
				"type":        "string",
				"description": "Skill folder name, for example minimax-xlsx.",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Bash command executed in skill directory. Env SKILL_DIR, FRONTEND_UPLOAD_DIR and FRONTEND_UPLOAD_URL_BASE are available.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Timeout seconds, default 120.",
			},
		},
	}
	return []openAITool{timeTool, runSkillBashTool}
}

func logUpstreamRequest(traceID string, backendID string, round int, payload openAIStreamReq, tools []openAITool, rawBody []byte) {
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			name = "(unnamed)"
		}
		toolNames = append(toolNames, name)
	}
	preview := string(rawBody)
	if len(preview) > 1200 {
		preview = preview[:1200] + "...(truncated)"
	}
	log.Printf(
		"[trace=%s] backend=%s round=%d upstream_request model=%s messages=%d tool_choice=%s tools=%d tool_names=%v body_bytes=%d body_preview=%s",
		traceID,
		backendID,
		round,
		payload.Model,
		len(payload.Messages),
		payload.ToolChoice,
		len(toolNames),
		toolNames,
		len(rawBody),
		preview,
	)
}

func normalizeDelta(previous, incoming string) (delta string, next string) {
	if incoming == "" {
		return "", previous
	}
	if strings.HasPrefix(incoming, previous) {
		return incoming[len(previous):], incoming
	}
	return incoming, previous + incoming
}

func decodeThinkTagEscapes(text string) string {
	replacer := strings.NewReplacer(
		`\\u003cthink\\u003e`, "<think>",
		`\\u003c/think\\u003e`, "</think>",
		`\\u003Cthink\\u003E`, "<think>",
		`\\u003C/think\\u003E`, "</think>",
		`\\u003cthink\\u003E`, "<think>",
		`\\u003c/think\\u003E`, "</think>",
	)
	return replacer.Replace(text)
}

func newTraceID() string {
	return fmt.Sprintf("t%x%x", time.Now().UnixNano(), rand.Uint32())
}

func acquireUpstreamSlot(ctx context.Context, traceID string, emit EmitFunc) (func(), error) {
	start := time.Now()
	select {
	case upstreamSlots <- struct{}{}:
		waited := time.Since(start)
		if waited > 200*time.Millisecond {
			log.Printf("[trace=%s] queue_wait_ms=%d", traceID, waited.Milliseconds())
		}
		return func() { <-upstreamSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func computeRetryDelay(attempt int, h http.Header) time.Duration {
	if h != nil {
		if ra := strings.TrimSpace(h.Get("Retry-After")); ra != "" {
			if sec, err := strconv.Atoi(ra); err == nil && sec > 0 {
				d := time.Duration(sec) * time.Second
				if d > maxRetryDelay {
					return maxRetryDelay
				}
				return d
			}
		}
	}
	base := baseRetryDelay * (1 << attempt)
	if base > maxRetryDelay {
		base = maxRetryDelay
	}
	jitter := time.Duration(rand.Int63n(int64(base) / 3))
	return base + jitter
}

func emitRetrying(emit EmitFunc, traceID string, attempt int, delay time.Duration, cause, message string, statusCode int, requestID string, retryable bool, busy bool) error {
	return emit("retrying", map[string]any{
		"trace_id":            traceID,
		"attempt":             attempt + 1,
		"max_attempts":        maxRetries,
		"delay_seconds":       delay.Seconds(),
		"status_code":         statusCode,
		"upstream_request_id": requestID,
		"cause":               cause,
		"retryable":           retryable,
		"busy":                busy,
		"message":             message,
	})
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func parseUpstreamError(statusCode int, headers http.Header, body []byte) upstreamErrorInfo {
	info := upstreamErrorInfo{
		StatusCode: statusCode,
		Message:    strings.TrimSpace(string(body)),
		Retryable:  statusCode == http.StatusTooManyRequests || statusCode == 529 || statusCode >= 500,
		Busy:       statusCode == http.StatusTooManyRequests || statusCode == 529,
		RequestID:  strings.TrimSpace(headers.Get("x-request-id")),
	}

	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		if info.Message == "" {
			info.Message = http.StatusText(statusCode)
		}
		return info
	}

	if rid, _ := obj["request_id"].(string); rid != "" {
		info.RequestID = rid
	}
	if et, _ := obj["type"].(string); et != "" {
		info.ErrorType = et
	}
	if msg, _ := obj["message"].(string); msg != "" {
		info.Message = msg
	}
	if errObj, ok := obj["error"].(map[string]any); ok {
		if et, _ := errObj["type"].(string); et != "" {
			info.ErrorType = et
		}
		if msg, _ := errObj["message"].(string); msg != "" {
			info.Message = msg
		}
	}

	if strings.Contains(strings.ToLower(info.ErrorType), "overload") || strings.Contains(strings.ToLower(info.Message), "繁忙") || strings.Contains(strings.ToLower(info.Message), "overload") {
		info.Busy = true
		info.Retryable = true
	}
	if info.Message == "" {
		info.Message = http.StatusText(statusCode)
	}
	return info
}
