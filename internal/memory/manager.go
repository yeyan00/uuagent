package memory

import (
	"fmt"
	"time"
)

// Status Memory 条目状态
type Status string

const (
	StatusDraft     Status = "draft"     // AI 自动记录，待审核
	StatusConfirmed Status = "confirmed"  // 用户确认
	StatusDeleted   Status = "deleted"   // 已删除
)

// Entry 单条 Memory
type Entry struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Status    Status `json:"status"`    // draft / confirmed / deleted
	Source    string `json:"source"`    // "ai" 或 "user"
	Project   string `json:"project"`   // 项目路径
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Manager Memory 管理器
type Manager struct {
	entries []Entry
}

// NewManager 创建 Memory 管理器
func NewManager() *Manager {
	return &Manager{entries: make([]Entry, 0)}
}

// AddDraft AI 自动添加 draft memory (不直接进入 system prompt)
func (m *Manager) AddDraft(content, project string) Entry {
	entry := Entry{
		ID:        generateID(),
		Content:   content,
		Status:    StatusDraft,
		Source:    "ai",
		Project:   project,
		CreatedAt: nowUnix(),
		UpdatedAt: nowUnix(),
	}
	m.entries = append(m.entries, entry)
	return entry
}

// Confirm 确认 draft memory
func (m *Manager) Confirm(id string) bool {
	for i := range m.entries {
		if m.entries[i].ID == id && m.entries[i].Status == StatusDraft {
			m.entries[i].Status = StatusConfirmed
			m.entries[i].UpdatedAt = nowUnix()
			return true
		}
	}
	return false
}

// Edit 编辑 memory 内容
func (m *Manager) Edit(id, newContent string) bool {
	for i := range m.entries {
		if m.entries[i].ID == id {
			m.entries[i].Content = newContent
			m.entries[i].UpdatedAt = nowUnix()
			return true
		}
	}
	return false
}

// Delete 删除 memory
func (m *Manager) Delete(id string) bool {
	for i := range m.entries {
		if m.entries[i].ID == id {
			m.entries[i].Status = StatusDeleted
			m.entries[i].UpdatedAt = nowUnix()
			return true
		}
	}
	return false
}

// ListDrafts 列出所有 draft memory (待审核)
func (m *Manager) ListDrafts(project string) []Entry {
	return m.filter(StatusDraft, project)
}

// ListConfirmed 列出所有已确认 memory (注入 system prompt)
func (m *Manager) ListConfirmed(project string) []Entry {
	return m.filter(StatusConfirmed, project)
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

func (m *Manager) filter(status Status, project string) []Entry {
	var result []Entry
	for _, e := range m.entries {
		if e.Status == status && (project == "" || e.Project == project) {
			result = append(result, e)
		}
	}
	return result
}

func generateID() string {
	// TODO: 用 UUID
	return fmt.Sprintf("mem-%d", nowUnix())
}

func nowUnix() int64 {
	return time.Now().Unix()
}
