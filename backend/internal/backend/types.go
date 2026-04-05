package backend

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type StreamRequest struct {
	Messages []Message
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type EmitFunc func(event string, payload any) error

type Adapter interface {
	ID() string
	StreamChat(ctx context.Context, req StreamRequest, emit EmitFunc) error
}

type BackendSummary struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Model   string `json:"model"`
	Enabled bool   `json:"enabled"`
}
