package backend

import (
	"fmt"

	"taterli-agent-chat/backend/internal/config"
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

	for _, b := range cfg.Backends {
		summary := BackendSummary{ID: b.ID, Type: b.Type, Model: b.Model, Enabled: b.Enabled}
		m.summaries = append(m.summaries, summary)
		if !b.Enabled {
			continue
		}

		var adapter Adapter
		switch b.Type {
		case "openai_compatible":
			adapter = NewOpenAICompatibleAdapter(b)
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
