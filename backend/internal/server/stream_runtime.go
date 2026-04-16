package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/db"
)

const taskTimeout = 20 * time.Minute

type sseEnvelope struct {
	Event   string
	Payload any
}

type taskRuntimeToolCall struct {
	CallID      string `json:"call_id"`
	ToolName    string `json:"tool_name"`
	DisplayName string `json:"display_name"`
	Arguments   string `json:"arguments"`
	Result      any    `json:"result,omitempty"`
	Status      string `json:"status"`
}

type taskRuntimeSnapshot struct {
	Reasoning      string                `json:"reasoning"`
	Content        string                `json:"content"`
	ReasoningDone  bool                  `json:"reasoning_done"`
	ThinkingStart  int64                 `json:"thinking_start"`
	ThinkingSecond float64               `json:"thinking_duration"`
	Retrying       map[string]any        `json:"retrying,omitempty"`
	ToolCalls      []taskRuntimeToolCall `json:"tool_calls,omitempty"`
	TokenTotal     *int                  `json:"token_total,omitempty"`
	TokenPerSecond *float64              `json:"token_per_second,omitempty"`
}

type taskRuntimeState struct {
	taskID    string
	backendID string
	snapshot  taskRuntimeSnapshot
	toolCalls map[string]*taskRuntimeToolCall
}

type streamTask struct {
	id          string
	backendID   string
	fingerprint string
	adapter     backend.Adapter
	req         backend.StreamRequest
}

type conversationRuntime struct {
	running                bool
	queue                  []*streamTask
	subscribers            map[int]chan sseEnvelope
	nextSubscriberID       int
	current                *taskRuntimeState
	currentTaskFingerprint string
	lastError              string
}

type conversationStreamRegistry struct {
	mu     sync.Mutex
	states map[string]*conversationRuntime
}

func newConversationStreamRegistry() *conversationStreamRegistry {
	return &conversationStreamRegistry{
		states: make(map[string]*conversationRuntime),
	}
}

func (r *conversationStreamRegistry) ensureRuntime(conversationID string) *conversationRuntime {
	rt := r.states[conversationID]
	if rt != nil {
		return rt
	}
	rt = &conversationRuntime{
		queue:       make([]*streamTask, 0),
		subscribers: make(map[int]chan sseEnvelope),
	}
	r.states[conversationID] = rt
	return rt
}

func (r *conversationStreamRegistry) enqueueAndSubscribe(conversationID string, task *streamTask) (int, <-chan sseEnvelope, map[string]any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rt := r.ensureRuntime(conversationID)
	ch := make(chan sseEnvelope, 128)
	subID := rt.nextSubscriberID
	rt.nextSubscriberID++
	rt.subscribers[subID] = ch

	enqueued := true
	if rt.running && rt.currentTaskFingerprint != "" && rt.currentTaskFingerprint == task.fingerprint {
		enqueued = false
	}
	if enqueued {
		rt.queue = append(rt.queue, task)
	}
	snapshot := r.buildSnapshotLocked(conversationID, rt)
	go r.runConversation(conversationID)
	return subID, ch, snapshot, enqueued
}

func (r *conversationStreamRegistry) unsubscribe(conversationID string, subID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := r.states[conversationID]
	if rt == nil {
		return
	}
	ch, ok := rt.subscribers[subID]
	if ok {
		delete(rt.subscribers, subID)
		close(ch)
	}
	if !rt.running && len(rt.queue) == 0 && len(rt.subscribers) == 0 {
		delete(r.states, conversationID)
	}
}

func (r *conversationStreamRegistry) broadcastLocked(rt *conversationRuntime, event string, payload any) {
	env := sseEnvelope{Event: event, Payload: payload}
	for id, ch := range rt.subscribers {
		select {
		case ch <- env:
		default:
			delete(rt.subscribers, id)
			close(ch)
		}
	}
}

func (r *conversationStreamRegistry) runConversation(conversationID string) {
	for {
		r.mu.Lock()
		rt := r.states[conversationID]
		if rt == nil {
			r.mu.Unlock()
			return
		}
		if rt.running {
			r.mu.Unlock()
			return
		}
		if len(rt.queue) == 0 {
			if len(rt.subscribers) == 0 {
				delete(r.states, conversationID)
			}
			r.mu.Unlock()
			return
		}
		task := rt.queue[0]
		rt.queue = rt.queue[1:]
		rt.running = true
		rt.lastError = ""
		rt.currentTaskFingerprint = task.fingerprint
		rt.current = &taskRuntimeState{
			taskID:    task.id,
			backendID: task.backendID,
			snapshot: taskRuntimeSnapshot{
				ThinkingStart: time.Now().UnixMilli(),
			},
			toolCalls: make(map[string]*taskRuntimeToolCall),
		}
		r.broadcastLocked(rt, "snapshot", r.buildSnapshotLocked(conversationID, rt))
		r.mu.Unlock()

		r.executeTask(conversationID, task)

		r.mu.Lock()
		rt = r.states[conversationID]
		if rt == nil {
			r.mu.Unlock()
			return
		}
		rt.running = false
		rt.currentTaskFingerprint = ""
		r.broadcastLocked(rt, "snapshot", r.buildSnapshotLocked(conversationID, rt))
		r.mu.Unlock()
	}
}

func (r *conversationStreamRegistry) executeTask(conversationID string, task *streamTask) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	emit := func(event string, payload any) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		rt := r.states[conversationID]
		if rt == nil || rt.current == nil || rt.current.taskID != task.id {
			return nil
		}
		r.applyEventLocked(rt, event, payload)
		r.broadcastLocked(rt, event, payload)
		return nil
	}

	err := task.adapter.StreamChat(ctx, task.req, emit)
	if err != nil {
		_ = emit("error", map[string]string{"message": err.Error()})
	}
	r.persistTaskResult(conversationID)
}

func (r *conversationStreamRegistry) applyEventLocked(rt *conversationRuntime, event string, payload any) {
	if rt.current == nil {
		return
	}
	state := rt.current
	now := time.Now()
	state.snapshot.ThinkingSecond = now.Sub(time.UnixMilli(state.snapshot.ThinkingStart)).Seconds()
	switch event {
	case "reasoning":
		if m, ok := payload.(map[string]string); ok {
			state.snapshot.Reasoning += m["delta"]
		} else if m, ok := payload.(map[string]any); ok {
			state.snapshot.Reasoning += strVal(m["delta"])
		}
	case "content":
		if m, ok := payload.(map[string]string); ok {
			state.snapshot.Content += m["delta"]
		} else if m, ok := payload.(map[string]any); ok {
			state.snapshot.Content += strVal(m["delta"])
		}
	case "tool_start":
		m, ok := payload.(map[string]any)
		if !ok {
			break
		}
		id := strVal(m["call_id"])
		if id == "" {
			break
		}
		item := &taskRuntimeToolCall{
			CallID:      id,
			ToolName:    strVal(m["tool_name"]),
			DisplayName: strVal(m["display_name"]),
			Arguments:   strVal(m["arguments"]),
			Status:      "running",
		}
		state.toolCalls[id] = item
		state.snapshot.ToolCalls = collectToolCalls(state.toolCalls)
	case "tool_end":
		m, ok := payload.(map[string]any)
		if !ok {
			break
		}
		id := strVal(m["call_id"])
		if id == "" {
			break
		}
		item := state.toolCalls[id]
		if item == nil {
			item = &taskRuntimeToolCall{CallID: id}
			state.toolCalls[id] = item
		}
		item.ToolName = strVal(m["tool_name"])
		item.DisplayName = strVal(m["display_name"])
		if item.Arguments == "" {
			item.Arguments = strVal(m["arguments"])
		}
		item.Result = m["result"]
		if resObj, ok := m["result"].(map[string]any); ok && strVal(resObj["error"]) != "" {
			item.Status = "error"
		} else {
			item.Status = "completed"
		}
		state.snapshot.ToolCalls = collectToolCalls(state.toolCalls)
	case "retrying":
		if m, ok := payload.(map[string]any); ok {
			state.snapshot.Retrying = map[string]any{
				"attempt":       m["attempt"],
				"max_attempts":  m["max_attempts"],
				"delay_seconds": m["delay_seconds"],
			}
		}
	case "error":
		msg := ""
		if m, ok := payload.(map[string]string); ok {
			msg = m["message"]
		}
		if m, ok := payload.(map[string]any); ok && msg == "" {
			msg = strVal(m["message"])
		}
		if msg != "" {
			if strings.TrimSpace(state.snapshot.Content) != "" {
				state.snapshot.Content += "\n"
			}
			state.snapshot.Content += "[Error] " + msg
			rt.lastError = msg
		}
		state.snapshot.ReasoningDone = true
	case "done":
		state.snapshot.ReasoningDone = true
		state.snapshot.Retrying = nil
		if m, ok := payload.(map[string]any); ok {
			total := intFromAny(m["usage"], "total_tokens")
			if total > 0 {
				state.snapshot.TokenTotal = &total
				sec := state.snapshot.ThinkingSecond
				if sec > 0 {
					tps := float64(total) / sec
					state.snapshot.TokenPerSecond = &tps
				}
			}
		}
	}
}

func (r *conversationStreamRegistry) persistTaskResult(conversationID string) {
	if !db.Ready() {
		return
	}
	r.mu.Lock()
	rt := r.states[conversationID]
	if rt == nil || rt.current == nil {
		r.mu.Unlock()
		return
	}
	snapshot := rt.current.snapshot
	rt.current = nil
	r.mu.Unlock()

	msgs := make([]db.Msg, 0, 1)
	assistant := db.Msg{
		Role:             "assistant",
		Content:          snapshot.Content,
		Reasoning:        snapshot.Reasoning,
		ReasoningDone:    snapshot.ReasoningDone,
		ThinkingDuration: &snapshot.ThinkingSecond,
		TokenTotal:       snapshot.TokenTotal,
		TokenPerSecond:   snapshot.TokenPerSecond,
	}
	if len(snapshot.ToolCalls) > 0 {
		if b, err := json.Marshal(snapshot.ToolCalls); err == nil {
			assistant.ToolCalls = string(b)
		}
	}
	msgs = append(msgs, assistant)

	conv, err := db.GetConversation(conversationID)
	if err != nil {
		log.Printf("persist conversation load failed id=%s err=%v", conversationID, err)
		return
	}
	if conv == nil {
		conv = &db.Conversation{
			ID:        conversationID,
			Title:     "新对话",
			CreatedAt: time.Now().UnixMilli(),
			Messages:  msgs,
		}
		if err := db.CreateConversation(conv); err != nil {
			log.Printf("persist conversation create failed id=%s err=%v", conversationID, err)
			return
		}
		return
	}
	merged := append([]db.Msg{}, conv.Messages...)
	merged = append(merged, msgs...)
	if err := db.SaveMessages(conversationID, merged); err != nil {
		log.Printf("persist conversation save failed id=%s err=%v", conversationID, err)
	}
}

func (r *conversationStreamRegistry) buildSnapshotLocked(conversationID string, rt *conversationRuntime) map[string]any {
	var currentTaskID string
	var draft any
	if rt.current != nil {
		currentTaskID = rt.current.taskID
		draft = rt.current.snapshot
	}
	return map[string]any{
		"conversation_id": conversationID,
		"is_streaming":    rt.running || len(rt.queue) > 0,
		"queue_length":    len(rt.queue),
		"current_task_id": currentTaskID,
		"last_error":      rt.lastError,
		"assistant":       draft,
	}
}

func (r *conversationStreamRegistry) getState(conversationID string) conversationStreamState {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := r.states[conversationID]
	if rt == nil {
		return conversationStreamState{}
	}
	var taskID string
	if rt.current != nil {
		taskID = rt.current.taskID
	}
	return conversationStreamState{
		IsStreaming:   rt.running || len(rt.queue) > 0,
		QueueLength:   len(rt.queue),
		CurrentTaskID: taskID,
		LastError:     rt.lastError,
	}
}

func collectToolCalls(m map[string]*taskRuntimeToolCall) []taskRuntimeToolCall {
	out := make([]taskRuntimeToolCall, 0, len(m))
	for _, item := range m {
		if item == nil {
			continue
		}
		out = append(out, *item)
	}
	return out
}

func strVal(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func intFromAny(v any, key string) int {
	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	raw := m[key]
	switch n := raw.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
