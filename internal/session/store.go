package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/uuagent/uuagent/internal/contextmgr"
	"github.com/uuagent/uuagent/internal/paths"
	"github.com/uuagent/uuagent/internal/types"
)

// Store 会话存储。P0 persistence stores each session as JSON so users can
// inspect conversation history directly under ~/.uuagent/sessions.
type Store struct {
	sessions map[string]*Session
	root     string
	mu       sync.RWMutex
}

// RunInfo records per-turn runtime metadata, including exposed tools.
type RunInfo struct {
	ID         string   `json:"id"`
	AgentID    string   `json:"agent_id,omitempty"`
	Model      string   `json:"model"`
	Prompt     string   `json:"prompt,omitempty"`
	Tools      []string `json:"tools,omitempty"`
	MCPServers []string `json:"mcp_servers,omitempty"`
	CreatedAt  int64    `json:"created_at"`
}

// Session 单个会话
type Session struct {
	ID        string               `json:"id"`
	Title     string               `json:"title,omitempty"`
	ParentID  string               `json:"parent_id,omitempty"`
	Messages  []types.Message      `json:"messages"`
	Runs      []RunInfo            `json:"runs,omitempty"`
	Summaries []contextmgr.Summary `json:"summaries,omitempty"`
	CreatedAt int64                `json:"created_at"`
	UpdatedAt int64                `json:"updated_at"`
	path      string               `json:"-"`
	mu        sync.Mutex
}

// NewStore 创建会话存储，默认持久化到 ~/.uuagent/sessions。
func NewStore() *Store {
	return NewStoreAt(paths.SessionsDir())
}

// NewStoreAt creates a store persisted under root. It eagerly loads existing
// *.json sessions from disk.
func NewStoreAt(root string) *Store {
	if strings.TrimSpace(root) == "" {
		root = ".uuagent-sessions"
	}
	s := &Store{sessions: make(map[string]*Session), root: filepath.Clean(root)}
	_ = os.MkdirAll(s.root, 0755)
	_ = s.Load()
	return s
}

// Root returns the directory containing session JSON files.
func (s *Store) Root() string { return s.root }

// Load reads all persisted session JSON files.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return fmt.Errorf("load session %s: %w", path, err)
		}
		if sess.ID == "" {
			sess.ID = strings.TrimSuffix(entry.Name(), ".json")
		}
		sess.path = path
		s.sessions[sess.ID] = &sess
	}
	return nil
}

// GetOrCreate 获取或创建会话
func (s *Store) GetOrCreate(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = normalizeID(id)

	if sess, ok := s.sessions[id]; ok {
		return sess
	}

	now := time.Now().Unix()
	sess := &Session{ID: id, Title: id, CreatedAt: now, UpdatedAt: now, path: s.pathFor(id)}
	s.sessions[id] = sess
	_ = sess.saveLocked()
	return sess
}

// Get returns an existing session without creating it.
func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[normalizeID(id)]
	return sess, ok
}

// Append 追加文本消息
func (s *Session) Append(role, content string) {
	s.AppendMessage(types.Message{Role: role, Content: content})
}

// UpdateTitle updates the human-readable session title.
func (s *Session) UpdateTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Title = strings.TrimSpace(title)
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// AppendRun persists per-turn runtime metadata.
func (s *Session) AppendRun(info RunInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info.ID == "" {
		info.ID = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	if info.CreatedAt == 0 {
		info.CreatedAt = time.Now().Unix()
	}
	s.Runs = append(s.Runs, info)
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// AppendMessage appends a full message, including multimodal content or tool calls.
func (s *Session) AppendMessage(msg types.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// AppendTool persists a tool result message in the session history.
func (s *Session) AppendTool(toolCallID, toolName, content string) {
	s.AppendMessage(types.Message{Role: "tool", ToolCallID: toolCallID, ToolName: toolName, Content: content})
}

// BuildMessages 组装发送给 LLM 的消息列表
func (s *Session) BuildMessages(prompt string) []types.Message {
	return s.BuildMessagesParts([]types.ContentPart{{Type: "text", Text: prompt}})
}

// BuildMessagesParts supports multimodal user content.
func (s *Session) BuildMessagesParts(parts []types.ContentPart) []types.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]types.Message, len(s.Messages))
	copy(msgs, s.Messages)
	var content any = parts
	if len(parts) == 1 && parts[0].Type == "text" {
		content = parts[0].Text
	}
	msgs = append(msgs, types.Message{Role: "user", Content: content})
	return msgs
}

// MaybeCompress compresses older messages when the session exceeds a budget.
func (s *Session) MaybeCompress(maxTokens int, threshold float64, keepLast int) (contextmgr.Summary, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !contextmgr.ShouldCompress(s.Messages, maxTokens, threshold) {
		return contextmgr.Summary{}, false
	}
	compressed, summary, ok := contextmgr.CompressOldMessages(s.ID, s.Messages, keepLast)
	if !ok {
		return contextmgr.Summary{}, false
	}
	s.Messages = compressed
	s.Summaries = append(s.Summaries, summary)
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
	return summary, true
}

// ListSummaries returns a copy of compression summaries.
func (s *Session) ListSummaries() []contextmgr.Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contextmgr.Summary(nil), s.Summaries...)
}

// Snapshot returns a copy safe for JSON responses.
func (s *Session) Snapshot() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := Session{ID: s.ID, Title: s.Title, ParentID: s.ParentID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	cp.Messages = append([]types.Message(nil), s.Messages...)
	cp.Runs = append([]RunInfo(nil), s.Runs...)
	cp.Summaries = append([]contextmgr.Summary(nil), s.Summaries...)
	return cp
}

// Fork copies an existing session into a new session. If upto is >= 0, only
// messages up to and including that index are copied.
func (s *Store) Fork(parentID, newID string, upto int) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentID = normalizeID(parentID)
	newID = normalizeID(newID)
	parent, ok := s.sessions[parentID]
	if !ok {
		return nil, fmt.Errorf("parent session %q not found", parentID)
	}
	if _, exists := s.sessions[newID]; exists {
		return nil, fmt.Errorf("session %q already exists", newID)
	}

	parent.mu.Lock()
	defer parent.mu.Unlock()

	limit := len(parent.Messages)
	if upto >= 0 && upto+1 < limit {
		limit = upto + 1
	}
	messages := make([]types.Message, limit)
	copy(messages, parent.Messages[:limit])

	now := time.Now().Unix()
	summaries := append([]contextmgr.Summary(nil), parent.Summaries...)
	runs := append([]RunInfo(nil), parent.Runs...)
	child := &Session{ID: newID, Title: newID, ParentID: parentID, Messages: messages, Runs: runs, Summaries: summaries, CreatedAt: now, UpdatedAt: now, path: s.pathFor(newID)}
	s.sessions[newID] = child
	if err := child.saveLocked(); err != nil {
		return nil, err
	}
	return child, nil
}

// Delete removes a session from memory and disk.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = normalizeID(id)
	sess, ok := s.sessions[id]
	if !ok {
		return false
	}
	delete(s.sessions, id)
	if sess.path != "" {
		_ = os.Remove(sess.path)
	}
	return true
}

// List returns a snapshot of all sessions.
func (s *Store) List() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess.Snapshot())
	}
	return out
}

func (s *Store) pathFor(id string) string {
	return filepath.Join(s.root, safeFileName(id)+".json")
}

func (s *Session) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func normalizeID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "default"
	}
	return id
}

func safeFileName(id string) string {
	var b strings.Builder
	for _, r := range id {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}
