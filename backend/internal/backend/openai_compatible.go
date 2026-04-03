package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"taterli-agent-chat/backend/internal/config"
)

type OpenAICompatibleAdapter struct {
	cfg        config.BackendConfig
	httpClient *http.Client
}

func NewOpenAICompatibleAdapter(cfg config.BackendConfig) *OpenAICompatibleAdapter {
	return &OpenAICompatibleAdapter{
		cfg: cfg,
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
	ExtraBody   map[string]any `json:"extra_body,omitempty"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
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

func (a *OpenAICompatibleAdapter) StreamChat(ctx context.Context, req StreamRequest, emit EmitFunc) error {
	payload := openAIStreamReq{
		Model:       a.cfg.Model,
		Messages:    req.Messages,
		Temperature: a.cfg.Temperature,
		Stream:      true,
		ExtraBody: map[string]any{
			"reasoning_split": a.cfg.ReasoningSplit,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(a.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return fmt.Errorf("upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024), 2*1024*1024)

	reasoningSeen := ""
	contentSeen := ""
	doneEmitted := false
	finishReason := "stop"
	var usage map[string]any
	splitter := &thinkTagSplitter{}
	contentStarted := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if !doneEmitted {
				doneEmitted = true
				if err := emit("done", map[string]any{"finish_reason": finishReason, "usage": usage}); err != nil {
					return err
				}
			}
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
					return err
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
						return err
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
					if err := emit("content", map[string]string{"delta": contentText}); err != nil {
						return err
					}
				}
			}
		}

		if len(choice.Delta.ReasoningDetails) == 0 && choice.Delta.Content == "" && splitter.pending != "" {
			reasoningText, contentText := splitter.Feed("")
			if reasoningText != "" {
				if err := emit("reasoning", map[string]string{"delta": reasoningText}); err != nil {
					return err
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
				if err := emit("content", map[string]string{"delta": contentText}); err != nil {
					return err
				}
			}
		}

		if choice.FinishReason != nil && !doneEmitted {
			doneEmitted = true
			if err := emit("done", map[string]any{"finish_reason": finishReason, "usage": usage}); err != nil {
				return err
			}
		}
	}

	if splitter.pending != "" {
		reasoningText, contentText := splitter.Feed("")
		if reasoningText != "" {
			if err := emit("reasoning", map[string]string{"delta": reasoningText}); err != nil {
				return err
			}
		}
		if contentText != "" {
			if !contentStarted {
				contentText = strings.TrimLeft(contentText, "\r\n")
			}
			if contentText != "" {
				contentStarted = true
				if err := emit("content", map[string]string{"delta": contentText}); err != nil {
					return err
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read upstream stream: %w", err)
	}

	if !doneEmitted {
		if err := emit("done", map[string]any{"finish_reason": finishReason, "usage": usage}); err != nil {
			return err
		}
	}

	return nil
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
