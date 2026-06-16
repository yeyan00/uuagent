package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/uuagent/uuagent/internal/paths"
)

// Status Memory 条目状态
type Status string

const (
	StatusDraft     Status = "draft"     // AI 自动记录，待审核
	StatusConfirmed Status = "confirmed" // 用户确认
	StatusDeleted   Status = "deleted"   // 已删除
)

// Entry 单条 Memory
type Entry struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Status    Status `json:"status"`  // draft / confirmed / deleted
	Source    string `json:"source"`  // "ai" 或 "user"
	Project   string `json:"project"` // 项目路径/ID
	Scope     string `json:"scope"`   // global / project / agent / session
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Manager Memory 管理器
type Manager struct {
	entries []Entry
	path    string
	mu      sync.RWMutex
}

// NewManager creates a persistent Memory manager at ~/.uuagent/memory.json.
func NewManager() *Manager {
	return NewManagerAt(filepath.Join(paths.UserDir(), "memory.json"))
}

// NewManagerAt creates a Memory manager persisted at path. Empty path disables persistence.
func NewManagerAt(path string) *Manager {
	m := &Manager{entries: make([]Entry, 0), path: path}
	_ = m.Load()
	return m
}

// Load reads memory entries from disk if persistence is enabled and the file exists.
func (m *Manager) Load() error {
	if m.path == "" {
		return nil
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	m.mu.Lock()
	m.entries = entries
	m.mu.Unlock()
	return nil
}

// Save writes memory entries to disk.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0600)
}

// Path returns the persistence file path.
func (m *Manager) Path() string { return m.path }

// AddDraft AI 自动添加 draft memory (不直接进入 system prompt)
func (m *Manager) AddDraft(content, project string) Entry {
	return m.Add(content, project, "project", "ai", StatusDraft)
}

// AddUser adds a user-confirmed memory.
func (m *Manager) AddUser(content, project, scope string) Entry {
	return m.Add(content, project, scope, "user", StatusConfirmed)
}

// Add inserts a memory entry with explicit metadata.
func (m *Manager) Add(content, project, scope, source string, status Status) Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if scope == "" {
		scope = "project"
	}
	if source == "" {
		source = "user"
	}
	now := nowUnix()
	entry := Entry{
		ID:        generateID(),
		Content:   content,
		Status:    status,
		Source:    source,
		Project:   project,
		Scope:     scope,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.entries = append(m.entries, entry)
	_ = m.saveLocked()
	return entry
}

// Confirm 确认 draft memory
func (m *Manager) Confirm(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.entries {
		if m.entries[i].ID == id && m.entries[i].Status == StatusDraft {
			m.entries[i].Status = StatusConfirmed
			m.entries[i].UpdatedAt = nowUnix()
			_ = m.saveLocked()
			return true
		}
	}
	return false
}

// Edit 编辑 memory 内容
func (m *Manager) Edit(id, newContent string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.entries {
		if m.entries[i].ID == id {
			m.entries[i].Content = newContent
			m.entries[i].UpdatedAt = nowUnix()
			_ = m.saveLocked()
			return true
		}
	}
	return false
}

// Delete 删除 memory
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.entries {
		if m.entries[i].ID == id {
			m.entries[i].Status = StatusDeleted
			m.entries[i].UpdatedAt = nowUnix()
			_ = m.saveLocked()
			return true
		}
	}
	return false
}

// List lists entries filtered by status/project. Empty filters match all.
func (m *Manager) List(status Status, project string) []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filterLocked(status, project)
}

// ListDrafts 列出所有 draft memory (待审核)
func (m *Manager) ListDrafts(project string) []Entry {
	return m.List(StatusDraft, project)
}

// ListConfirmed 列出所有已确认 memory (注入 system prompt)
func (m *Manager) ListConfirmed(project string) []Entry {
	return m.List(StatusConfirmed, project)
}

// BuildSystemPrompt 构建 memory 注入到 system prompt 的文本
func (m *Manager) BuildSystemPrompt(project string) string {
	confirmed := m.ListConfirmed(project)
	if len(confirmed) == 0 {
		return ""
	}

	text := "[Memory]\n"
	for _, e := range confirmed {
		text += "- " + e.Content + "\n"
	}
	return text
}

func (m *Manager) filterLocked(status Status, project string) []Entry {
	var result []Entry
	for _, e := range m.entries {
		if status != "" && e.Status != status {
			continue
		}
		if project != "" && e.Project != project {
			continue
		}
		result = append(result, e)
	}
	return result
}

func generateID() string {
	return fmt.Sprintf("mem-%d", time.Now().UnixNano())
}

func nowUnix() int64 {
	return time.Now().Unix()
}
