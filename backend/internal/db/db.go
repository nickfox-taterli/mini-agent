package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Conversation 表示一个会话.
type Conversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"createdAt"`
	Messages  []Msg  `json:"messages,omitempty"`
}

// Msg 表示一条消息.
type Msg struct {
	Role             string   `json:"role"`
	Content          string   `json:"content"`
	Reasoning        string   `json:"reasoning,omitempty"`
	ReasoningDone    bool     `json:"reasoningDone,omitempty"`
	ThinkingDuration *float64 `json:"thinkingDuration,omitempty"`
	ToolCalls        string   `json:"toolCalls,omitempty"`
	TokenTotal       *int     `json:"tokenTotal,omitempty"`
	TokenPerSecond   *float64 `json:"tokenPerSecond,omitempty"`
}

var globalDB *sql.DB

const (
	defaultTokenTotal     = 1
	defaultTokenPerSecond = 1.0
)

// Init 初始化数据库连接并建表.
func Init(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	var err error
	globalDB, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}
	// 迁移: 为已有数据库补齐新增列.
	runMigration(`ALTER TABLE messages ADD COLUMN tool_calls TEXT NOT NULL DEFAULT ''`)
	runMigration(`ALTER TABLE messages ADD COLUMN token_total INTEGER NOT NULL DEFAULT 1`)
	runMigration(`ALTER TABLE messages ADD COLUMN token_per_second REAL NOT NULL DEFAULT 1`)
	// 对历史无记录数据进行默认回填.
	runMigration(`UPDATE messages SET token_total = 10 WHERE token_total IS NULL`)
	runMigration(`UPDATE messages SET token_per_second = 10 WHERE token_per_second IS NULL`)
	return nil
}

func runMigration(query string) {
	if _, err := globalDB.Exec(query); err != nil {
		// 忽略重复列等幂等迁移错误.
	}
}

func createTables() error {
	_, err := globalDB.Exec(`
		CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '新对话',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK(role IN ('user','assistant')),
			content TEXT NOT NULL DEFAULT '',
			reasoning TEXT NOT NULL DEFAULT '',
			reasoning_done INTEGER NOT NULL DEFAULT 0,
			thinking_duration REAL,
			tool_calls TEXT NOT NULL DEFAULT '',
			token_total INTEGER NOT NULL DEFAULT 10,
			token_per_second REAL NOT NULL DEFAULT 10,
			sort_order INTEGER NOT NULL
		);
	`)
	return err
}

// ListConversations 返回所有会话 (不含消息), 按创建时间降序.
func ListConversations() ([]Conversation, error) {
	rows, err := globalDB.Query(`SELECT id, title, created_at FROM conversations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if result == nil {
		result = []Conversation{}
	}
	return result, rows.Err()
}

// GetConversation 返回单个会话及其全部消息.
func GetConversation(id string) (*Conversation, error) {
	var c Conversation
	err := globalDB.QueryRow(`SELECT id, title, created_at FROM conversations WHERE id = ?`, id).
		Scan(&c.ID, &c.Title, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	msgs, err := loadMessages(id)
	if err != nil {
		return nil, err
	}
	c.Messages = msgs
	return &c, nil
}

func loadMessages(convID string) ([]Msg, error) {
	rows, err := globalDB.Query(
		`SELECT role, content, reasoning, reasoning_done, thinking_duration, tool_calls, token_total, token_per_second
		 FROM messages WHERE conversation_id = ? ORDER BY sort_order ASC`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Msg
	for rows.Next() {
		var m Msg
		var reasoningDone int
		var thinkingDuration sql.NullFloat64
		var tokenTotal sql.NullInt64
		var tokenPerSecond sql.NullFloat64
		if err := rows.Scan(
			&m.Role, &m.Content, &m.Reasoning, &reasoningDone, &thinkingDuration, &m.ToolCalls, &tokenTotal, &tokenPerSecond,
		); err != nil {
			return nil, err
		}
		m.ReasoningDone = reasoningDone != 0
		if thinkingDuration.Valid {
			m.ThinkingDuration = &thinkingDuration.Float64
		}
		total := defaultTokenTotal
		if tokenTotal.Valid {
			total = int(tokenTotal.Int64)
		}
		speed := defaultTokenPerSecond
		if tokenPerSecond.Valid {
			speed = tokenPerSecond.Float64
		}
		m.TokenTotal = &total
		m.TokenPerSecond = &speed
		result = append(result, m)
	}
	if result == nil {
		result = []Msg{}
	}
	return result, rows.Err()
}

// CreateConversation 创建新会话.
func CreateConversation(c *Conversation) error {
	tx, err := globalDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		c.ID, c.Title, c.CreatedAt)
	if err != nil {
		return err
	}

	for i, m := range c.Messages {
		if err := insertMessage(tx, c.ID, &m, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertMessage(tx *sql.Tx, convID string, m *Msg, order int) error {
	var dur sql.NullFloat64
	if m.ThinkingDuration != nil {
		dur.Float64 = *m.ThinkingDuration
		dur.Valid = true
	}
	done := 0
	if m.ReasoningDone {
		done = 1
	}
	total := defaultTokenTotal
	if m.TokenTotal != nil {
		total = *m.TokenTotal
	}
	speed := defaultTokenPerSecond
	if m.TokenPerSecond != nil {
		speed = *m.TokenPerSecond
	}
	_, err := tx.Exec(
		`INSERT INTO messages (conversation_id, role, content, reasoning, reasoning_done, thinking_duration, tool_calls, token_total, token_per_second, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		convID, m.Role, m.Content, m.Reasoning, done, dur, m.ToolCalls, total, speed, order)
	return err
}

// SaveMessages 全量替换某会话的消息.
func SaveMessages(convID string, msgs []Msg) error {
	tx, err := globalDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM messages WHERE conversation_id = ?`, convID)
	if err != nil {
		return err
	}

	for i, m := range msgs {
		if err := insertMessage(tx, convID, &m, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateConversation 更新会话标题.
func UpdateConversation(id, title string) error {
	_, err := globalDB.Exec(`UPDATE conversations SET title = ? WHERE id = ?`, title, id)
	return err
}

// DeleteConversation 删除会话及其消息.
func DeleteConversation(id string) error {
	_, err := globalDB.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	return err
}
