package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yeyan00/uuagent/internal/contextmgr"
	"github.com/yeyan00/uuagent/internal/paths"
	"github.com/yeyan00/uuagent/internal/types"
)

// Store persists sessions as JSON so users can inspect conversation history
// directly under ~/.uuagent/sessions.
type Store struct {
	sessions map[string]*Session
	root     string
	mu       sync.RWMutex
}

// RunInfo records per-turn runtime metadata, including exposed tools.
type RunInfo struct {
	ID          string     `json:"id"`
	Status      string     `json:"status,omitempty"`
	AgentID     string     `json:"agent_id,omitempty"`
	ProjectID   string     `json:"project_id,omitempty"`
	ProjectPath string     `json:"project_path,omitempty"`
	Model       string     `json:"model"`
	Prompt      string     `json:"prompt,omitempty"`
	Tools       []string   `json:"tools,omitempty"`
	MCPServers  []string   `json:"mcp_servers,omitempty"`
	Usage       TokenUsage `json:"usage,omitempty"`
	CreatedAt   int64      `json:"created_at"`
}

// TokenUsage tracks model token consumption for a run or session.
type TokenUsage struct {
	InputTokens           int  `json:"input_tokens,omitempty"`
	OutputTokens          int  `json:"output_tokens,omitempty"`
	TotalTokens           int  `json:"total_tokens,omitempty"`
	EstimatedInputTokens  int  `json:"estimated_input_tokens,omitempty"`
	EstimatedOutputTokens int  `json:"estimated_output_tokens,omitempty"`
	Estimated             bool `json:"estimated,omitempty"`
}

// ContextStats describes current session context size.
type ContextStats struct {
	EstimatedTokens int     `json:"estimated_tokens"`
	MaxTokens       int     `json:"max_tokens"`
	Percent         float64 `json:"percent"`
}

// CompactArchive records messages removed from active context during compaction.
type CompactArchive struct {
	ID        string          `json:"id"`
	SummaryID string          `json:"summary_id"`
	Messages  []types.Message `json:"messages"`
	CreatedAt int64           `json:"created_at"`
}

// Session is one conversation thread.
type Session struct {
	ID          string               `json:"id"`
	Title       string               `json:"title,omitempty"`
	ProjectID   string               `json:"project_id,omitempty"`
	ProjectPath string               `json:"project_path,omitempty"`
	ParentID    string               `json:"parent_id,omitempty"`
	Messages    []types.Message      `json:"messages"`
	Runs        []RunInfo            `json:"runs,omitempty"`
	Usage       TokenUsage           `json:"usage,omitempty"`
	Summaries   []contextmgr.Summary `json:"summaries,omitempty"`
	Archives    []CompactArchive     `json:"archives,omitempty"`
	Memory      string               `json:"memory_snapshot,omitempty"`
	CreatedAt   int64                `json:"created_at"`
	UpdatedAt   int64                `json:"updated_at"`
	path        string               `json:"-"`
	mu          sync.Mutex
}

// NewStore creates a session store persisted under ~/.uuagent/sessions.
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

// GetOrCreate returns an existing session or creates a new one.
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

// Append adds a text message.
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

// BindProject records the project that owns this session.
func (s *Session) BindProject(projectID, projectPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ProjectID != "" && s.ProjectID != projectID {
		return fmt.Errorf("session %q is bound to project %q", s.ID, s.ProjectID)
	}
	s.ProjectID = projectID
	s.ProjectPath = projectPath
	s.UpdatedAt = time.Now().Unix()
	return s.saveLocked()
}

// MaybeTitleFromPrompt sets a default title from the first user prompt.
func (s *Session) MaybeTitleFromPrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Messages) > 0 {
		return
	}
	if s.Title != "" && s.Title != s.ID && s.Title != "New Session" {
		return
	}
	title := strings.Join(strings.Fields(prompt), " ")
	if len([]rune(title)) > 40 {
		runes := []rune(title)
		title = string(runes[:40]) + "..."
	}
	if title == "" {
		title = s.ID
	}
	s.Title = title
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

// UpdateRunStatus updates one run status and persists it.
func (s *Session) UpdateRunStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Runs {
		if s.Runs[i].ID == id {
			s.Runs[i].Status = status
			s.UpdatedAt = time.Now().Unix()
			_ = s.saveLocked()
			return
		}
	}
}

// AppendMessage appends a full message, including multimodal content or tool calls.
func (s *Session) AppendMessage(msg types.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// MemorySnapshot returns the frozen memory prompt snapshot for this session.
func (s *Session) MemorySnapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Memory
}

// EnsureMemorySnapshot stores a memory snapshot once and returns the frozen value.
func (s *Session) EnsureMemorySnapshot(snapshot string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Memory == "" {
		s.Memory = snapshot
		s.UpdatedAt = time.Now().Unix()
		_ = s.saveLocked()
	}
	return s.Memory
}

// RefreshMemorySnapshot replaces the frozen memory prompt snapshot.
func (s *Session) RefreshMemorySnapshot(snapshot string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Memory = snapshot
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// AppendTool persists a tool result message in the session history.
func (s *Session) AppendTool(toolCallID, toolName, content string) {
	s.AppendMessage(types.Message{Role: "tool", ToolCallID: toolCallID, ToolName: toolName, Content: content})
}

// BuildMessages assembles messages for the LLM.
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
	result, ok := contextmgr.CompactOldMessages(s.ID, s.Messages, keepLast)
	if !ok {
		return contextmgr.Summary{}, false
	}
	s.Messages = result.Messages
	s.Summaries = append(s.Summaries, result.Summary)
	s.Archives = append(s.Archives, CompactArchive{
		ID:        fmt.Sprintf("archive-%d", time.Now().UnixNano()),
		SummaryID: result.Summary.ID,
		Messages:  append([]types.Message(nil), result.Archive.Messages...),
		CreatedAt: result.Summary.CreatedAt,
	})
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
	return result.Summary, true
}

// ListSummaries returns a copy of compression summaries.
func (s *Session) ListSummaries() []contextmgr.Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contextmgr.Summary(nil), s.Summaries...)
}

// CompactArchive persists a compact summary and its archived messages.
func (s *Session) CompactArchive(summary contextmgr.Summary, archive CompactArchive) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if archive.ID == "" {
		archive.ID = fmt.Sprintf("archive-%d", time.Now().UnixNano())
	}
	if archive.SummaryID == "" {
		archive.SummaryID = summary.ID
	}
	if archive.CreatedAt == 0 {
		archive.CreatedAt = time.Now().Unix()
	}
	archive.Messages = append([]types.Message(nil), archive.Messages...)
	s.Summaries = append(s.Summaries, summary)
	s.Archives = append(s.Archives, archive)
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// ListArchives returns a copy of compact archives.
func (s *Session) ListArchives() []CompactArchive {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneArchives(s.Archives)
}

// Snapshot returns a copy safe for JSON responses.
func (s *Session) Snapshot() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := Session{ID: s.ID, Title: s.Title, ProjectID: s.ProjectID, ProjectPath: s.ProjectPath, ParentID: s.ParentID, Usage: s.Usage, Memory: s.Memory, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	cp.Messages = append([]types.Message(nil), s.Messages...)
	cp.Runs = append([]RunInfo(nil), s.Runs...)
	cp.Summaries = append([]contextmgr.Summary(nil), s.Summaries...)
	cp.Archives = cloneArchives(s.Archives)
	return cp
}

// AddRunUsage records token usage for a run and session totals.
func (s *Session) AddRunUsage(runID string, usage TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Runs {
		if s.Runs[i].ID == runID {
			s.Runs[i].Usage = usage
			break
		}
	}
	s.Usage.InputTokens += usage.InputTokens
	s.Usage.OutputTokens += usage.OutputTokens
	s.Usage.TotalTokens += usage.TotalTokens
	s.Usage.EstimatedInputTokens += usage.EstimatedInputTokens
	s.Usage.EstimatedOutputTokens += usage.EstimatedOutputTokens
	if usage.Estimated {
		s.Usage.Estimated = true
	}
	s.UpdatedAt = time.Now().Unix()
	_ = s.saveLocked()
}

// ContextStats returns current estimated context size against the configured max.
func (s *Session) ContextStats(maxTokens int) ContextStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	estimated := contextmgr.EstimateTokens(s.Messages)
	percent := 0.0
	if maxTokens > 0 {
		percent = float64(estimated) / float64(maxTokens)
	}
	return ContextStats{EstimatedTokens: estimated, MaxTokens: maxTokens, Percent: percent}
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
	archives := cloneArchives(parent.Archives)
	runs := append([]RunInfo(nil), parent.Runs...)
	child := &Session{ID: newID, Title: newID, ParentID: parentID, Messages: messages, Runs: runs, Summaries: summaries, Archives: archives, CreatedAt: now, UpdatedAt: now, path: s.pathFor(newID)}
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

func cloneArchives(archives []CompactArchive) []CompactArchive {
	out := make([]CompactArchive, len(archives))
	for i, archive := range archives {
		out[i] = archive
		out[i].Messages = append([]types.Message(nil), archive.Messages...)
	}
	return out
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
