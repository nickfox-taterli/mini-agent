package mcpserver

import (
	"log"
	"sync/atomic"
)

// MiniMaxKeyManager 管理 MiniMax API Key 的 round-robin 轮换.
type MiniMaxKeyManager struct {
	keys    []string
	current uint64
}

// NewMiniMaxKeyManager 创建 Key 轮换管理器.
func NewMiniMaxKeyManager(keys []string) *MiniMaxKeyManager {
	log.Printf("[minimax-keys] loaded %d API keys", len(keys))
	return &MiniMaxKeyManager{keys: keys}
}

// NextKey 以 round-robin 方式返回下一个 Key.
// 每次调用都会打印日志以便调试.
func (m *MiniMaxKeyManager) NextKey() string {
	idx := atomic.AddUint64(&m.current, 1) % uint64(len(m.keys))
	key := m.keys[idx]
	suffix := keySuffix(key)
	log.Printf("[minimax-keys] using key index %d/%d (suffix: ...%s)", idx, len(m.keys), suffix)
	return key
}

// ReportError 报告某个 Key 失败, 推进到下一个 Key.
func (m *MiniMaxKeyManager) ReportError(key string) {
	for i, k := range m.keys {
		if k == key {
			log.Printf("[minimax-keys] key index %d failed, rotating to next", i)
			atomic.AddUint64(&m.current, 1)
			return
		}
	}
}

// KeyCount 返回管理的 Key 数量.
func (m *MiniMaxKeyManager) KeyCount() int {
	return len(m.keys)
}

// keySuffix 返回 Key 的最后 8 个字符用于日志标识.
func keySuffix(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[len(key)-8:]
}
