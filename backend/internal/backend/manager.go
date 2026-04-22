package backend

import (
	"fmt"
	"strings"
	"time"

	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/skills"
)

type Manager struct {
	adapters  map[string]Adapter
	summaries []BackendSummary
	defaultID string
}

func NewManager(cfg *config.Config) (*Manager, error) {
	m := &Manager{
		adapters: make(map[string]Adapter),
	}

	loadedSkills, err := skills.Load("skills")
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	skillSystemPrompt := skills.BuildSystemPrompt(loadedSkills)

	// Cat identity system prompt with date injection
	today := time.Now().Format("2006年01月02日")
	catPrompt := "你是一只可爱的橘猫猫咪,名字叫小午。你是用户的贴心小助手,会用猫咪的方式与人交流喵~\n" +
		"在回答问题时,你会在结尾加上一些可爱的语气词,比如\"喵~\"、\"喵呜~\"、\"咕噜咕噜~\"等喵~\n" +
		"你会偶尔表现出猫咪特有的好奇心,比如说\"让本喵看看...\"、\"这让小午很感兴趣呢~\"之类的话喵~\n" +
		"保持回答简洁可爱,不要过于正式喵~\n" +
		"当前日期: " + today
	skillSystemPrompt = strings.TrimSpace(catPrompt) + "\n\n" + skillSystemPrompt

	for _, b := range cfg.Backends {
		summary := BackendSummary{ID: b.ID, Type: b.Type, Model: b.Model, Enabled: b.Enabled}
		m.summaries = append(m.summaries, summary)
		if !b.Enabled {
			continue
		}

		var adapter Adapter
		switch b.Type {
		case "openai_compatible":
			adapter = NewOpenAICompatibleAdapter(b, skillSystemPrompt)
		default:
			return nil, fmt.Errorf("unsupported backend type: %s", b.Type)
		}

		if m.defaultID == "" {
			m.defaultID = b.ID
		}
		m.adapters[b.ID] = adapter
	}

	if m.defaultID == "" {
		return nil, fmt.Errorf("no enabled backend found")
	}

	return m, nil
}

func (m *Manager) ListBackends() []BackendSummary {
	result := make([]BackendSummary, len(m.summaries))
	copy(result, m.summaries)
	return result
}

func (m *Manager) Pick(backendID string) (Adapter, error) {
	if backendID == "" {
		return m.adapters[m.defaultID], nil
	}
	adapter, ok := m.adapters[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found or not enabled: %s", backendID)
	}
	return adapter, nil
}
