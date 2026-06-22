package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/paths"
	"github.com/yeyan00/uuagent/internal/router"
	"github.com/yeyan00/uuagent/internal/session"
	"github.com/yeyan00/uuagent/internal/types"
)

// Result is the result of one delegated subtask.
type Result struct {
	ID        string `json:"id"`
	Goal      string `json:"goal"`
	Model     string `json:"model"`
	SessionID string `json:"session_id,omitempty"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
}

// Task is one persisted delegated subagent task.
type Task struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`
	Goal      string `json:"goal"`
	Model     string `json:"model,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Manager coordinates delegated subagents.
type Manager struct {
	cfg         config.SubagentConfig
	router      *router.Router
	taskPath    string
	sessionRoot string
	mu          sync.Mutex
	tasks       []Task
}

// NewManager creates a subagent manager.
func NewManager(cfg config.SubagentConfig, r *router.Router) *Manager {
	return NewManagerAt(cfg, r, filepath.Join(paths.UserDir(), "subagent_tasks.json"), "")
}

// NewManagerAt creates a subagent manager with explicit persistence paths for tests and local state.
func NewManagerAt(cfg config.SubagentConfig, r *router.Router, taskPath, sessionRoot string) *Manager {
	m := &Manager{cfg: cfg, router: r, taskPath: taskPath, sessionRoot: sessionRoot}
	_ = m.LoadTasks()
	return m
}

// Delegate executes subtasks concurrently.
func (m *Manager) Delegate(parent ParentAgent, goals []string) []Result {
	return m.DelegateContext(context.Background(), parent, goals)
}

// DelegateContext executes subtasks concurrently and propagates cancellation to child agents.
func (m *Manager) DelegateContext(ctx context.Context, parent ParentAgent, goals []string) []Result {
	return m.delegate(ctx, parent, "", "", goals)
}

// DelegateProfile executes goals through a configured subagent profile and persists task state.
func (m *Manager) DelegateProfile(ctx context.Context, parent ParentAgent, parentID, profileID string, goals []string) []Result {
	return m.delegate(ctx, parent, parentID, profileID, goals)
}

func (m *Manager) delegate(ctx context.Context, parent ParentAgent, parentID, profileID string, goals []string) []Result {
	var wg sync.WaitGroup
	results := make([]Result, len(goals))
	maxConcurrent := m.cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	sem := make(chan struct{}, maxConcurrent) // concurrency limit

	for i, goal := range goals {
		wg.Add(1)
		go func(idx int, g string) {
			defer wg.Done()
			profile := m.profile(profileID)
			profile.MaxTurns = m.resolveMaxTurns(profile)

			select {
			case sem <- struct{}{}: // acquire semaphore
			case <-ctx.Done():
				id := fmt.Sprintf("sa-%d", idx)
				results[idx] = Result{ID: id, Goal: g, Error: ctx.Err().Error()}
				return
			}
			defer func() { <-sem }() // release semaphore

			// Route simple subtasks to cheaper models when configured.
			model, _ := m.router.Route(g, 0)
			if profile.Model != "" {
				model = profile.Model
			}

			childID := fmt.Sprintf("sa-%d", idx)
			if parentID != "" || profile.ID != "" {
				childID = fmt.Sprintf("sa-%d", time.Now().UnixNano())
			}
			sessionID := ""
			if parentID != "" || profile.ID != "" {
				sessionID = "subagent-" + childID
			}
			m.upsertTask(Task{ID: childID, ParentID: parentID, ProfileID: profile.ID, Goal: g, Model: model, SessionID: sessionID, Status: "running"})

			// Create an isolated child Agent with the routed model and restricted tools.
			child := parent.NewSubagentChildWithSession(model, m.blockedToolsForProfile(profile), m.sessionStore())

			result, err := m.runChild(ctx, child, sessionID, profile, g)
			if err != nil {
				results[idx] = Result{
					ID:        childID,
					Goal:      g,
					Model:     model,
					SessionID: sessionID,
					Error:     err.Error(),
				}
				m.finishTask(childID, "error", "", err.Error())
			} else {
				results[idx] = Result{
					ID:        childID,
					Goal:      g,
					Model:     model,
					SessionID: sessionID,
					Output:    result,
				}
				m.finishTask(childID, "done", result, "")
			}
		}(i, goal)
	}

	wg.Wait()
	return results
}

func (m *Manager) runChild(ctx context.Context, child ChildAgent, sessionID string, profile config.SubagentProfile, goal string) (string, error) {
	if profile.ID == "" {
		return child.RunOnce(ctx, goal)
	}
	agentProfile := config.AgentProfile{
		ID:                profile.ID,
		Name:              profile.Name,
		Description:       profile.Description,
		SystemPrompt:      profile.SystemPrompt,
		Model:             profile.Model,
		EnabledTools:      profile.EnabledTools,
		EnabledSkills:     profile.EnabledSkills,
		EnabledMCPServers: profile.EnabledMCPServers,
		PermissionMode:    profile.PermissionMode,
		MaxTurns:          profile.MaxTurns,
	}
	events, err := child.RunWithProfileParts(ctx, sessionID, "", agentProfile, []types.ContentPart{{Type: "text", Text: goal}})
	if err != nil {
		return "", err
	}
	var out string
	for evt := range events {
		switch evt.Type {
		case "content", "tool_result":
			out += evt.Text
		case "error":
			return out, fmt.Errorf(evt.Text)
		}
	}
	return out, nil
}

func (m *Manager) profile(id string) config.SubagentProfile {
	for _, profile := range m.cfg.Profiles {
		if profile.ID == id {
			return profile
		}
	}
	return config.SubagentProfile{ID: id}
}

func (m *Manager) sessionStore() *session.Store {
	if m.sessionRoot == "" {
		return nil
	}
	return session.NewStoreAt(m.sessionRoot)
}

// blockedTools returns tool names disabled for child agents.
func (m *Manager) blockedTools() map[string]bool {
	return m.blockedToolsForProfile(config.SubagentProfile{})
}

func (m *Manager) blockedToolsForProfile(profile config.SubagentProfile) map[string]bool {
	blocked := make(map[string]bool)
	for _, t := range m.cfg.BlockedTools {
		blocked[t] = true
	}
	for _, t := range profile.BlockedTools {
		blocked[t] = true
	}
	return blocked
}

// LoadTasks reads persisted tasks if configured.
func (m *Manager) LoadTasks() error {
	if m.taskPath == "" {
		return nil
	}
	data, err := os.ReadFile(m.taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &m.tasks)
}

// Tasks returns a copy of persisted/in-memory tasks.
func (m *Manager) Tasks() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Task(nil), m.tasks...)
}

func (m *Manager) upsertTask(task Task) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	for i := range m.tasks {
		if m.tasks[i].ID == task.ID {
			m.tasks[i] = task
			_ = m.saveTasksLocked()
			return
		}
	}
	m.tasks = append(m.tasks, task)
	_ = m.saveTasksLocked()
}

func (m *Manager) finishTask(id, status, output, errText string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			m.tasks[i].Status = status
			m.tasks[i].Output = output
			m.tasks[i].Error = errText
			m.tasks[i].UpdatedAt = time.Now().Unix()
			_ = m.saveTasksLocked()
			return
		}
	}
}

func (m *Manager) saveTasksLocked() error {
	if m.taskPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.taskPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.taskPath, data, 0600)
}
