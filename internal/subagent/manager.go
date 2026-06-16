package subagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/router"
)

// Result is the result of one delegated subtask.
type Result struct {
	ID     string `json:"id"`
	Goal   string `json:"goal"`
	Model  string `json:"model"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Manager coordinates delegated subagents.
type Manager struct {
	cfg    config.SubagentConfig
	router *router.Router
}

// NewManager creates a subagent manager.
func NewManager(cfg config.SubagentConfig, r *router.Router) *Manager {
	return &Manager{cfg: cfg, router: r}
}

// Delegate executes subtasks concurrently.
func (m *Manager) Delegate(parent *agent.Agent, goals []string) []Result {
	var wg sync.WaitGroup
	results := make([]Result, len(goals))
	sem := make(chan struct{}, m.cfg.MaxConcurrent) // concurrency limit

	for i, goal := range goals {
		wg.Add(1)
		go func(idx int, g string) {
			defer wg.Done()

			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			// Route simple subtasks to cheaper models when configured.
			model, _ := m.router.Route(g, 0)

			childID := fmt.Sprintf("sa-%d", idx)

			// Create an isolated child Agent with the routed model and restricted tools.
			child := parent.NewChild(model, m.blockedTools())

			result, err := child.RunOnce(context.Background(), g)
			if err != nil {
				results[idx] = Result{
					ID:    childID,
					Goal:  g,
					Model: model,
					Error: err.Error(),
				}
			} else {
				results[idx] = Result{
					ID:     childID,
					Goal:   g,
					Model:  model,
					Output: result,
				}
			}
		}(i, goal)
	}

	wg.Wait()
	return results
}

// blockedTools returns tool names disabled for child agents.
func (m *Manager) blockedTools() map[string]bool {
	blocked := make(map[string]bool)
	for _, t := range m.cfg.BlockedTools {
		blocked[t] = true
	}
	return blocked
}
