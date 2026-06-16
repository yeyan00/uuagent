package subagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/uuagent/uuagent/internal/agent"
	"github.com/uuagent/uuagent/internal/config"
	"github.com/uuagent/uuagent/internal/router"
)

// Result 子任务结果
type Result struct {
	ID     string `json:"id"`
	Goal   string `json:"goal"`
	Model  string `json:"model"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Manager Subagent 管理器
type Manager struct {
	cfg    config.SubagentConfig
	router *router.Router
}

// NewManager 创建 Subagent 管理器
func NewManager(cfg config.SubagentConfig, r *router.Router) *Manager {
	return &Manager{cfg: cfg, router: r}
}

// Delegate 并行执行子任务
func (m *Manager) Delegate(parent *agent.Agent, goals []string) []Result {
	var wg sync.WaitGroup
	results := make([]Result, len(goals))
	sem := make(chan struct{}, m.cfg.MaxConcurrent) // 并发控制

	for i, goal := range goals {
		wg.Add(1)
		go func(idx int, g string) {
			defer wg.Done()

			sem <- struct{}{}        // 获取信号量
			defer func() { <-sem }() // 释放信号量

			// 智能路由: 简单子任务用便宜模型
			model, tier := m.router.Route(g, 0)

			childID := fmt.Sprintf("sa-%d", idx)

			// 创建隔离的子 Agent
			// 子 Agent 用路由决定的模型，受限工具集
			child := agent.NewWithModel(model, m.blockedTools())

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

// blockedTools 返回子任务禁用的工具名
func (m *Manager) blockedTools() map[string]bool {
	blocked := make(map[string]bool)
	for _, t := range m.cfg.BlockedTools {
		blocked[t] = true
	}
	return blocked
}
