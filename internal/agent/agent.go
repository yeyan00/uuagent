package agent

import (
	"context"
	"fmt"

	"github.com/uuagent/uuagent/internal/config"
	"github.com/uuagent/uuagent/internal/router"
	"github.com/uuagent/uuagent/internal/session"
)

// Agent 核心 Agent 结构体
type Agent struct {
	cfg     *config.Config
	router  *router.Router
	session *session.Store
}

// New 创建新 Agent
func New(cfg *config.Config) *Agent {
	return &Agent{
		cfg:     cfg,
		router:  router.New(cfg.Agent.Routing),
		session: session.NewStore(),
	}
}

// Run 运行一轮对话 (Agent 7步循环)
func (a *Agent) Run(ctx context.Context, sessionID string, prompt string) (<-chan Event, error) {
	// 1. 路由决策
	model, tier := a.router.Route(prompt, 0)

	// 2. 创建事件流
	events := make(chan Event, 64)

	go func() {
		defer close(events)

		// 发送路由决策事件
		events <- Event{Type: "route", Model: model, Tier: string(tier)}

		// 3. 获取或创建会话
		sess := a.session.GetOrCreate(sessionID)

		// 4. 组装消息
		messages := sess.BuildMessages(prompt)

		// 5. 调用 LLM (通过 CLIProxyAPI)
		events <- Event{Type: "status", Text: "thinking..."}
		response, toolCalls, err := a.callLLM(ctx, model, messages)
		if err != nil {
			events <- Event{Type: "error", Text: err.Error()}
			return
		}

		// 6. 流式输出文本
		if response != "" {
			events <- Event{Type: "content", Text: response}
		}

		// 7. 如果有工具调用，执行工具
		if len(toolCalls) > 0 {
			for _, tc := range toolCalls {
				events <- Event{Type: "tool_start", ToolName: tc.Name, ToolID: tc.ID}
				result := a.executeTool(ctx, tc)
				events <- Event{Type: "tool_result", ToolID: tc.ID, Text: result}
			}
		}

		// 保存到会话
		sess.Append("user", prompt)
		sess.Append("assistant", response)

		events <- Event{Type: "done"}
	}()

	return events, nil
}

// callLLM 调用 LLM API (通过 CLIProxyAPI)
func (a *Agent) callLLM(ctx context.Context, model string, messages []Message) (string, []ToolCall, error) {
	// TODO: 实现 OpenAI-compatible API 调用
	// 通过 a.cfg.Agent.ProxyURL 调用 CLIProxyAPI
	return "", nil, fmt.Errorf("LLM call not implemented yet")
}

// executeTool 执行工具
func (a *Agent) executeTool(ctx context.Context, tc ToolCall) string {
	// TODO: 实现工具执行分发
	return fmt.Sprintf("Tool %s not implemented yet", tc.Name)
}

// Event Agent 事件 (SSE 推送)
type Event struct {
	Type     string `json:"type"`      // route, status, content, tool_start, tool_result, error, done
	Model    string `json:"model,omitempty"`
	Tier     string `json:"tier,omitempty"`
	Text     string `json:"text,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	ToolID   string `json:"tool_id,omitempty"`
}

// Message 消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}
