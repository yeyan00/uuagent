package session

import (
	"sync"

	"github.com/uuagent/uuagent/internal/agent"
)

// Store 会话存储
type Store struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// Session 单个会话
type Session struct {
	ID       string
	Messages []agent.Message
}

// NewStore 创建会话存储
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*Session),
	}
}

// GetOrCreate 获取或创建会话
func (s *Store) GetOrCreate(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		return sess
	}

	sess := &Session{ID: id}
	s.sessions[id] = sess
	return sess
}

// Append 追加消息
func (s *Session) Append(role, content string) {
	s.Messages = append(s.Messages, agent.Message{Role: role, Content: content})
}

// BuildMessages 组装发送给 LLM 的消息列表
func (s *Session) BuildMessages(prompt string) []agent.Message {
	msgs := make([]agent.Message, len(s.Messages))
	copy(msgs, s.Messages)
	msgs = append(msgs, agent.Message{Role: "user", Content: prompt})
	return msgs
}
