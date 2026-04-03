package backend

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamRequest struct {
	Messages []Message
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
