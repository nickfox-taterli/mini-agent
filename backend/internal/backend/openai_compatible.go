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
	"sync/atomic"
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
	askUserOpenTag         = "<ask_user>"
	askUserCloseTag        = "</ask_user>"
)

var upstreamSlots = make(chan struct{}, upstreamConcurrencyCap)

type OpenAICompatibleAdapter struct {
	cfg               config.BackendConfig
	httpClient        *http.Client
	skillSystemPrompt string
	keyIdx            uint64 // atomic, round-robin index for API keys
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

// nextKey 以 round-robin 方式返回下一个 API Key.
func (a *OpenAICompatibleAdapter) nextKey() string {
	keys := a.cfg.APIKeys
	if len(keys) == 0 {
		return a.cfg.APIKey
	}
	if len(keys) == 1 {
		return keys[0]
	}
	idx := atomic.AddUint64(&a.keyIdx, 1) % uint64(len(keys))
	key := keys[idx]
	suffix := key
	if len(key) > 8 {
		suffix = key[len(key)-8:]
	}
	log.Printf("[backend-keys] %s using key index %d/%d (...%s)", a.cfg.ID, idx, len(keys), suffix)
	return key
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

type askUserPayload struct {
	Question    string         `json:"question,omitempty"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	FieldKey    string         `json:"field_key,omitempty"`
	Placeholder string         `json:"placeholder,omitempty"`
	InputType   string         `json:"input_type,omitempty"`
	Options     []string       `json:"options,omitempty"`
	Required    bool           `json:"required"`
	Fields      []askUserField `json:"fields,omitempty"`
	SubmitLabel string         `json:"submit_label,omitempty"`
}

type askUserField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	InputType   string   `json:"input_type,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required"`
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

			// 发送工具开始事件
			if err := emit("tool_start", map[string]any{
				"tool_name":    call.Function.Name,
				"display_name": mcpserver.ToolDisplayName(call.Function.Name),
				"call_id":      call.ID,
				"arguments":    call.Function.Arguments,
			}); err != nil {
				return err
			}

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

			// 发送工具结束事件
			if err := emit("tool_end", map[string]any{
				"tool_name":    call.Function.Name,
				"display_name": mcpserver.ToolDisplayName(call.Function.Name),
				"call_id":      call.ID,
				"result":       out,
			}); err != nil {
				return err
			}

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
	mergedPrompt := strings.TrimSpace(
		a.skillSystemPrompt +
			"\nMCP tools are available through tool calling in this chat.\n\n" +
			buildAskUserPrompt() + "\n\n" +
			buildContainerMountGuardPrompt(),
	)
	if len(working) > 0 && working[0].Role == "system" {
		working[0].Content = strings.TrimSpace(mergedPrompt + "\n\n" + working[0].Content)
		return working
	}
	return append([]Message{{Role: "system", Content: mergedPrompt}}, working...)
}

func buildAskUserPrompt() string {
	return "If user intent is clear but key information is missing, ask follow-up questions with a structured card format. " +
		"Strong preference: collect multiple missing fields (2-4) in a single ask_user card via fields array, instead of one-by-one. " +
		"Use one-by-one only if dependency is strict (next question depends on previous answer). " +
		"Use input_type select or multiselect whenever choices are predictable; use text only when options are open-ended. " +
		"For select/multiselect fields, put recommended option first so UI can preselect it, and include an 'Other' choice semantics for manual input fallback. " +
		"Hard rule: when any required parameter is missing, do NOT provide provisional answers, do NOT provide broad summaries, and do NOT call tools/search yet. Ask first, then answer after user provides required info. " +
		"For location-dependent requests (weather, nearby places, local recommendations), location is required: ask for city/region first via ask_user before any analysis. " +
		"Output ONLY: <ask_user>{\"title\":\"...\",\"description\":\"...\",\"fields\":[{\"key\":\"...\",\"label\":\"...\",\"input_type\":\"text|select|multiselect\",\"options\":[...],\"placeholder\":\"...\",\"required\":true}],\"submit_label\":\"...\"}</ask_user>. " +
		"You may use legacy single-question shape if only one field is missing. No markdown, no explanation outside tags. " +
		"If you already started asking via ask_user, keep using ask_user for each next follow-up until all required info is collected. Continue normal answering only after information is sufficient."
}

func buildContainerMountGuardPrompt() string {
	uploadRootHint := "<frontend_upload_root>"
	if uploadRoot, err := mcpserver.ResolveFrontendUploadRootDirExported(); err == nil {
		uploadRootHint = uploadRoot
	}
	return fmt.Sprintf(
		"Container file-access policy:\n"+
			"1) Uploaded files are auto-mounted read-only into Docker at the same absolute path (typically under %s).\n"+
			"2) Before searching files, first run mount sanity checks in container: `pwd && ls -la`, then `findmnt -T .` (or read `/proc/mounts`).\n"+
			"3) For each user-provided path, verify existence first: `test -e <path>`.\n"+
			"4) If key paths are missing, stop blind searching and report mount diagnostics (cwd, mount summary, checked paths) before proceeding.",
		uploadRootHint,
	)
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
	httpReq.Header.Set("Authorization", "Bearer "+a.nextKey())

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
	contentMode := "unknown"
	pendingProbe := ""

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
					var err error
					contentMode, pendingProbe, err = emitContentWithAskDetection(emit, contentMode, pendingProbe, contentText)
					if err != nil {
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
				var err error
				contentMode, pendingProbe, err = emitContentWithAskDetection(emit, contentMode, pendingProbe, contentText)
				if err != nil {
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
				var err error
				contentMode, pendingProbe, err = emitContentWithAskDetection(emit, contentMode, pendingProbe, contentText)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	var askPayload *askUserPayload
	if contentMode == "unknown" && pendingProbe != "" {
		trimmed := strings.TrimLeft(pendingProbe, " \t\r\n")
		if !isAskUserPrefixCandidate(trimmed) {
			if err := emit("content", map[string]string{"delta": pendingProbe}); err != nil {
				return nil, err
			}
			contentMode = "normal"
		} else if strings.HasPrefix(trimmed, askUserOpenTag) {
			contentMode = "ask"
		}
		pendingProbe = ""
	}
	if contentMode == "ask" {
		if parsed, ok := parseAskUserPayload(assistantContent); ok {
			askPayload = parsed
			assistantContent = ""
			finishReason = "ask_user"
			if err := emit("ask_user", parsed); err != nil {
				return nil, err
			}
		} else {
			contentMode = "normal"
			if err := emit("content", map[string]string{"delta": assistantContent}); err != nil {
				return nil, err
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
	if askPayload != nil {
		result.AssistantContent = ""
	}
	return result, nil
}

func emitContentWithAskDetection(emit EmitFunc, mode string, pendingProbe string, delta string) (string, string, error) {
	switch mode {
	case "normal":
		if err := emit("content", map[string]string{"delta": delta}); err != nil {
			return mode, pendingProbe, err
		}
		return mode, pendingProbe, nil
	case "ask":
		return mode, pendingProbe, nil
	default:
		pendingProbe += delta
		trimmed := strings.TrimLeft(pendingProbe, " \t\r\n")
		if isAskUserPrefixCandidate(trimmed) {
			if strings.HasPrefix(trimmed, askUserOpenTag) {
				return "ask", pendingProbe, nil
			}
			return "unknown", pendingProbe, nil
		}
		if err := emit("content", map[string]string{"delta": pendingProbe}); err != nil {
			return "normal", "", err
		}
		return "normal", "", nil
	}
}

func isAskUserPrefixCandidate(trimmed string) bool {
	if trimmed == "" {
		return true
	}
	if strings.HasPrefix(trimmed, askUserOpenTag) {
		return true
	}
	if len(trimmed) <= len(askUserOpenTag) && strings.HasPrefix(askUserOpenTag, trimmed) {
		return true
	}
	return false
}

func parseAskUserPayload(content string) (*askUserPayload, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, askUserOpenTag) {
		return nil, false
	}
	end := strings.LastIndex(trimmed, askUserCloseTag)
	if end < 0 {
		return nil, false
	}
	if strings.TrimSpace(trimmed[end+len(askUserCloseTag):]) != "" {
		return nil, false
	}
	jsonPart := strings.TrimSpace(trimmed[len(askUserOpenTag):end])
	if jsonPart == "" {
		return nil, false
	}
	var payload askUserPayload
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return nil, false
	}
	payload.Question = strings.TrimSpace(payload.Question)
	payload.Title = strings.TrimSpace(payload.Title)
	payload.Description = strings.TrimSpace(payload.Description)
	payload.FieldKey = strings.TrimSpace(payload.FieldKey)
	payload.Placeholder = strings.TrimSpace(payload.Placeholder)
	payload.InputType = strings.TrimSpace(payload.InputType)
	if payload.InputType != "select" && payload.InputType != "multiselect" {
		payload.InputType = "text"
	}
	filtered := make([]string, 0, len(payload.Options))
	for _, option := range payload.Options {
		option = strings.TrimSpace(option)
		if option != "" {
			filtered = append(filtered, option)
		}
	}
	payload.Options = filtered
	if (payload.InputType == "select" || payload.InputType == "multiselect") && len(payload.Options) == 0 {
		payload.InputType = "text"
	}
	if payload.FieldKey == "" {
		payload.FieldKey = "additional_info"
	}
	if !payload.Required {
		payload.Required = true
	}

	normalizedFields := make([]askUserField, 0, len(payload.Fields))
	for i, field := range payload.Fields {
		field.Key = strings.TrimSpace(field.Key)
		field.Label = strings.TrimSpace(field.Label)
		if field.Key == "" {
			field.Key = fmt.Sprintf("field_%d", i+1)
		}
		if field.Label == "" {
			field.Label = field.Key
		}
		field.InputType = strings.TrimSpace(field.InputType)
		if field.InputType != "select" && field.InputType != "multiselect" {
			field.InputType = "text"
		}
		field.Placeholder = strings.TrimSpace(field.Placeholder)
		fieldOpts := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			option = strings.TrimSpace(option)
			if option != "" {
				fieldOpts = append(fieldOpts, option)
			}
		}
		field.Options = fieldOpts
		if (field.InputType == "select" || field.InputType == "multiselect") && len(field.Options) == 0 {
			field.InputType = "text"
		}
		if !field.Required {
			field.Required = true
		}
		normalizedFields = append(normalizedFields, field)
	}
	payload.Fields = normalizedFields
	if payload.Question == "" && payload.Title == "" && payload.Description == "" && len(payload.Fields) == 0 {
		return nil, false
	}
	if len(payload.Fields) == 0 {
		payload.Fields = []askUserField{{
			Key:         payload.FieldKey,
			Label:       payload.Question,
			InputType:   payload.InputType,
			Placeholder: payload.Placeholder,
			Options:     payload.Options,
			Required:    payload.Required,
		}}
	}
	if payload.Title == "" {
		if payload.Question != "" {
			payload.Title = payload.Question
		} else {
			payload.Title = "请补充以下信息"
		}
	}
	payload.SubmitLabel = strings.TrimSpace(payload.SubmitLabel)
	if payload.SubmitLabel == "" {
		payload.SubmitLabel = "提交补充信息"
	}
	return &payload, true
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
				"description": "Bash command executed in skill directory. Env SKILL_DIR and FRONTEND_UPLOAD_DIR are available.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Timeout seconds, default 120.",
			},
		},
	}

	webFetchTool := openAITool{}
	webFetchTool.Type = "function"
	webFetchTool.Function.Name = "web_fetch"
	webFetchTool.Function.Description = "Fetch a web page via HTTP GET and return its text content. Only text content types are supported."
	webFetchTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"url"},
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Target URL to fetch. Must be a valid http or https URL.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Request timeout in seconds, default 30, max 120.",
			},
			"max_length": map[string]any{
				"type":        "integer",
				"description": "Maximum content length in characters, default 100000.",
			},
		},
	}

	convertPathTool := openAITool{}
	convertPathTool.Type = "function"
	convertPathTool.Function.Name = "convert_local_path_to_url"
	convertPathTool.Function.Description = "Convert a local file path under frontend directory into a downloadable URL."
	convertPathExample := "<frontend_upload_dir>/report.xlsx"
	if uploadDir, err := mcpserver.ResolveFrontendUploadDirExported(); err == nil {
		convertPathExample = uploadDir + "/report.xlsx"
	}
	convertPathTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"local_path"},
		"properties": map[string]any{
			"local_path": map[string]any{
				"type":        "string",
				"description": "Absolute local file path under frontend directory, for example " + convertPathExample,
			},
		},
	}

	webSearchTool := openAITool{}
	webSearchTool.Type = "function"
	webSearchTool.Function.Name = "minimax_web_search"
	webSearchTool.Function.Description = "Search the web using MiniMax search API. Returns titles, URLs and snippets. Aim for 3-5 keywords for best results. For time-sensitive topics, include the current date."
	webSearchTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query, 3-5 keywords for best results.",
			},
		},
	}

	imageTool := openAITool{}
	imageTool.Type = "function"
	imageTool.Function.Name = "minimax_understand_image"
	imageTool.Function.Description = "Analyze and understand an image using MiniMax vision model. Provide image URL or local file path and a prompt describing what to analyze. Supports JPEG, PNG, WebP formats."
	imageTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"image_url", "prompt"},
		"properties": map[string]any{
			"image_url": map[string]any{
				"type":        "string",
				"description": "Image URL (http/https) or absolute local file path to analyze.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "What to analyze or describe about the image.",
			},
		},
	}

	pythonSessionInitTool := openAITool{}
	pythonSessionInitTool.Type = "function"
	pythonSessionInitTool.Function.Name = "python_session_init"
	pythonSessionInitTool.Function.Description = "Initialize a Python Docker sandbox session. MUST be called before python_run_code. Returns session_id which is required for subsequent calls. Uploaded files under frontend/upload are auto-mounted read-only when available."
	pythonSessionInitTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"session_id": map[string]any{
				"type":        "string",
				"description": "Optional session id for reusing existing container.",
			},
			"python_version": map[string]any{
				"type":        "string",
				"description": "Optional Python version, one of 3.10/3.11/3.12. Default 3.11.",
			},
		},
	}

	pythonInstallTool := openAITool{}
	pythonInstallTool.Type = "function"
	pythonInstallTool.Function.Name = "python_install_packages"
	pythonInstallTool.Function.Description = "Install Python packages explicitly in a Python sandbox session."
	pythonInstallTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"session_id", "packages"},
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"packages": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"timeout_seconds": map[string]any{"type": "integer"},
		},
	}

	pythonRunTool := openAITool{}
	pythonRunTool.Type = "function"
	pythonRunTool.Function.Name = "python_run_code"
	pythonRunTool.Function.Description = "Run Python code in a Docker sandbox session. Requires session_id from python_session_init."
	pythonRunTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"session_id"},
		"properties": map[string]any{
			"session_id":      map[string]any{"type": "string", "description": "Session ID from python_session_init."},
			"code":            map[string]any{"type": "string"},
			"file_path":       map[string]any{"type": "string"},
			"args":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"stdin":           map[string]any{"type": "string"},
			"timeout_seconds": map[string]any{"type": "integer"},
		},
	}

	pythonCloseTool := openAITool{}
	pythonCloseTool.Type = "function"
	pythonCloseTool.Function.Name = "python_session_close"
	pythonCloseTool.Function.Description = "Close Python Docker sandbox session. Call when done to release resources."
	pythonCloseTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"session_id"},
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string", "description": "Session ID from python_session_init."},
		},
	}

	codeSessionInitTool := openAITool{}
	codeSessionInitTool.Type = "function"
	codeSessionInitTool.Function.Name = "code_session_init"
	codeSessionInitTool.Function.Description = "Initialize a common code Docker sandbox session for shell/c/cpp/java/php. MUST be called before code_run. Returns session_id which is required for subsequent calls. Uploaded files under frontend/upload are auto-mounted read-only when available."
	codeSessionInitTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string", "description": "Optional session id for reusing existing container."},
		},
	}

	codeRunTool := openAITool{}
	codeRunTool.Type = "function"
	codeRunTool.Function.Name = "code_run"
	codeRunTool.Function.Description = "Run shell/c/cpp/java/php code in Docker sandbox session. Requires session_id from code_session_init. For Java, the public class must be named 'Main'. Shell scripts support args."
	codeRunTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"session_id", "language", "source_code"},
		"properties": map[string]any{
			"session_id":      map[string]any{"type": "string", "description": "Session ID from code_session_init"},
			"language":        map[string]any{"type": "string"},
			"source_code":     map[string]any{"type": "string"},
			"stdin":           map[string]any{"type": "string"},
			"args":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"timeout_seconds": map[string]any{"type": "integer"},
		},
	}

	codeCloseTool := openAITool{}
	codeCloseTool.Type = "function"
	codeCloseTool.Function.Name = "code_session_close"
	codeCloseTool.Function.Description = "Close common code Docker sandbox session. Call when done to release resources."
	codeCloseTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"session_id"},
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
		},
	}

	// LibreOffice 工具集 (重量级容器操作, 优先使用 Skill)
	loConvertTool := openAITool{}
	loConvertTool.Type = "function"
	loConvertTool.Function.Name = "libreoffice_convert"
	loConvertTool.Function.Description = "Heavyweight operation - launches a Docker container (~1GB image). Prefer existing Skills (minimax-xlsx, minimax-docx, minimax-pdf, pptx-generator) when possible. Convert documents between formats using LibreOffice. Supports docx/xlsx/pptx/odt/ods/odp to pdf/html/txt/csv/png and more."
	loConvertTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"file_path"},
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the source document (e.g. .docx, .xlsx, .pptx, .odt)",
			},
			"output_format": map[string]any{
				"type":        "string",
				"description": "Target format extension: pdf (default), html, txt, csv, png, jpg, odt, ods, odp, xlsx, docx, pptx, rtf",
			},
		},
	}

	loExtractTextTool := openAITool{}
	loExtractTextTool.Type = "function"
	loExtractTextTool.Function.Name = "libreoffice_extract_text"
	loExtractTextTool.Function.Description = "Heavyweight operation - launches a Docker container (~1GB image). Prefer existing Skills when possible. Extract plain text content from documents (docx, xlsx, pptx, odt, etc.) using LibreOffice."
	loExtractTextTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"file_path"},
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the document to extract text from",
			},
		},
	}

	loBatchConvertTool := openAITool{}
	loBatchConvertTool.Type = "function"
	loBatchConvertTool.Function.Name = "libreoffice_batch_convert"
	loBatchConvertTool.Function.Description = "Heavyweight operation - launches a Docker container (~1GB image). Prefer existing Skills when possible. Batch convert all documents in a directory to a target format using LibreOffice."
	loBatchConvertTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"directory"},
		"properties": map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Absolute path to directory containing source files",
			},
			"output_format": map[string]any{
				"type":        "string",
				"description": "Target format, default pdf",
			},
			"file_pattern": map[string]any{
				"type":        "string",
				"description": "File glob pattern, default *.docx (e.g. *.xlsx, *.pptx)",
			},
		},
	}

	loReadMetadataTool := openAITool{}
	loReadMetadataTool.Type = "function"
	loReadMetadataTool.Function.Name = "libreoffice_read_metadata"
	loReadMetadataTool.Function.Description = "Heavyweight operation - launches a Docker container (~1GB image). Prefer existing Skills when possible. Read document metadata (title, author, page count, word count, file info, etc.) using LibreOffice and Python-UNO."
	loReadMetadataTool.Function.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"file_path"},
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the document",
			},
		},
	}

	return []openAITool{
		timeTool, runSkillBashTool, webFetchTool, convertPathTool, webSearchTool, imageTool,
		pythonSessionInitTool, pythonInstallTool, pythonRunTool, pythonCloseTool,
		codeSessionInitTool, codeRunTool, codeCloseTool,
		loConvertTool, loExtractTextTool, loBatchConvertTool, loReadMetadataTool,
	}
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
