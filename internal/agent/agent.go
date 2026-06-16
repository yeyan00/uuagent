package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uuagent/uuagent/internal/config"
	"github.com/uuagent/uuagent/internal/mcp"
	"github.com/uuagent/uuagent/internal/memory"
	"github.com/uuagent/uuagent/internal/project"
	"github.com/uuagent/uuagent/internal/router"
	"github.com/uuagent/uuagent/internal/session"
	"github.com/uuagent/uuagent/internal/skills"
	"github.com/uuagent/uuagent/internal/tools"
	"github.com/uuagent/uuagent/internal/types"
)

// Re-export shared chat types for compatibility with callers in this package.
type Event = types.Event
type Message = types.Message
type ToolCall = types.ToolCall

// Agent 核心 Agent 结构体
type Agent struct {
	cfg           *config.Config
	router        *router.Router
	session       *session.Store
	tools         *tools.Registry
	skills        *skills.Registry
	mcp           mcp.Client
	memory        *memory.Manager
	projects      *project.Store
	fixedModel    string
	blockedTools  map[string]bool
	enabledSkills []string
	httpClient    *http.Client
}

// New 创建新 Agent
func New(cfg *config.Config) *Agent {
	workspace, _ := os.Getwd()
	return &Agent{
		cfg:        cfg,
		router:     router.New(cfg.Agent.Routing),
		session:    session.NewStore(),
		tools:      tools.NewRegistry(workspace),
		skills:     skills.NewRegistryFromConfig(cfg),
		mcp:        mcp.NewMockClient(),
		memory:     memory.NewManager(),
		projects:   project.NewStore(""),
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// NewWithModel creates a restricted child agent. It is used by subagents and tests.
func NewWithModel(model string, blockedTools map[string]bool) *Agent {
	cfg := config.Default()
	config.ApplyEnv(cfg)
	a := New(cfg)
	a.fixedModel = model
	a.blockedTools = blockedTools
	return a
}

// NewChild creates a restricted child agent inheriting parent configuration.
func (a *Agent) NewChild(model string, blockedTools map[string]bool) *Agent {
	child := New(a.cfg)
	child.fixedModel = model
	child.blockedTools = blockedTools
	child.enabledSkills = append([]string(nil), a.enabledSkills...)
	return child
}

// RunOnce runs a single isolated prompt and returns the final text response.
func (a *Agent) RunOnce(ctx context.Context, prompt string) (string, error) {
	sessionID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	events, err := a.Run(ctx, sessionID, prompt)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for evt := range events {
		switch evt.Type {
		case "content", "tool_result":
			out.WriteString(evt.Text)
		case "error":
			return out.String(), fmt.Errorf(evt.Text)
		}
	}
	return out.String(), nil
}

// Config exposes the active runtime config for API handlers.
func (a *Agent) Config() *config.Config { return a.cfg }

// ReloadConfig applies a merged runtime config without dropping sessions,
// memories, or the in-memory project registry.
func (a *Agent) ReloadConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	a.cfg = cfg
	a.router = router.New(cfg.Agent.Routing)
	a.skills = skills.NewRegistryFromConfig(cfg)
}

// Sessions exposes the session store for API handlers.
func (a *Agent) Sessions() *session.Store { return a.session }

// Memories exposes the memory manager for API handlers.
func (a *Agent) Memories() *memory.Manager { return a.memory }

// Projects exposes the project store for API handlers.
func (a *Agent) Projects() *project.Store { return a.projects }

// Route returns the model routing decision without running the agent.
func (a *Agent) Route(prompt string, tokenCount int) (string, router.Tier) {
	if a.fixedModel != "" {
		return a.fixedModel, router.TierStrong
	}
	return a.router.Route(prompt, tokenCount)
}

// Profiles returns configured agent profiles.
func (a *Agent) Profiles() []config.AgentProfile {
	out := make([]config.AgentProfile, len(a.cfg.Agents))
	copy(out, a.cfg.Agents)
	return out
}

// GetProfile returns one profile by id. Empty id resolves to default.
func (a *Agent) GetProfile(id string) (config.AgentProfile, bool) {
	if id == "" {
		id = "default"
	}
	for _, p := range a.cfg.Agents {
		if p.ID == id {
			return p, true
		}
	}
	if id == "default" {
		return config.AgentProfile{ID: "default", Name: "Default Agent", PermissionMode: a.cfg.Agent.DefaultPermission, MaxTurns: a.cfg.Agent.MaxTurns}, true
	}
	return config.AgentProfile{}, false
}

// UpsertProfile creates or updates an agent profile in the active runtime config.
func (a *Agent) UpsertProfile(profile config.AgentProfile) config.AgentProfile {
	profile = a.normalizeProfile(profile)
	for i := range a.cfg.Agents {
		if a.cfg.Agents[i].ID == profile.ID {
			a.cfg.Agents[i] = profile
			return profile
		}
	}
	a.cfg.Agents = append(a.cfg.Agents, profile)
	return profile
}

// UpsertProfilePersistent updates runtime config and writes ~/.uuagent/config.yaml.
func (a *Agent) UpsertProfilePersistent(profile config.AgentProfile) (config.AgentProfile, error) {
	profile = a.UpsertProfile(profile)
	return profile, config.SaveUser(a.cfg)
}

// DeleteProfile removes an agent profile except the built-in default.
func (a *Agent) DeleteProfile(id string) bool {
	if id == "" || id == "default" {
		return false
	}
	for i := range a.cfg.Agents {
		if a.cfg.Agents[i].ID == id {
			a.cfg.Agents = append(a.cfg.Agents[:i], a.cfg.Agents[i+1:]...)
			return true
		}
	}
	return false
}

// DeleteProfilePersistent removes an agent profile and writes ~/.uuagent/config.yaml.
func (a *Agent) DeleteProfilePersistent(id string) error {
	if !a.DeleteProfile(id) {
		return fmt.Errorf("agent not found or protected")
	}
	return config.SaveUser(a.cfg)
}

// CloneProfile creates a copy with a new id/name.
func (a *Agent) CloneProfile(id, newID, newName string) (config.AgentProfile, error) {
	profile, ok := a.GetProfile(id)
	if !ok {
		return config.AgentProfile{}, fmt.Errorf("agent not found")
	}
	profile.ID = newID
	profile.Name = newName
	profile = a.normalizeProfile(profile)
	for _, existing := range a.cfg.Agents {
		if existing.ID == profile.ID {
			return config.AgentProfile{}, fmt.Errorf("agent already exists")
		}
	}
	a.cfg.Agents = append(a.cfg.Agents, profile)
	return profile, nil
}

func (a *Agent) CloneProfilePersistent(id, newID, newName string) (config.AgentProfile, error) {
	profile, err := a.CloneProfile(id, newID, newName)
	if err != nil {
		return profile, err
	}
	return profile, config.SaveUser(a.cfg)
}

func (a *Agent) normalizeProfile(profile config.AgentProfile) config.AgentProfile {
	if profile.ID == "" {
		profile.ID = strings.ToLower(strings.ReplaceAll(profile.Name, " ", "-"))
	}
	if profile.ID == "" {
		profile.ID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	if profile.Name == "" {
		profile.Name = profile.ID
	}
	if profile.PermissionMode == "" {
		profile.PermissionMode = a.cfg.Agent.DefaultPermission
	}
	if profile.MaxTurns == 0 {
		profile.MaxTurns = a.cfg.Agent.MaxTurns
	}
	return profile
}

// Run 运行一轮对话 (Agent 7步循环)
func (a *Agent) Run(ctx context.Context, sessionID string, prompt string) (<-chan Event, error) {
	return a.RunWithAgent(ctx, sessionID, "", prompt)
}

// RunWithAgent runs one turn with an optional configured AgentProfile.
func (a *Agent) RunWithAgent(ctx context.Context, sessionID, agentID string, prompt string) (<-chan Event, error) {
	return a.RunWithAgentParts(ctx, sessionID, agentID, []types.ContentPart{{Type: "text", Text: prompt}})
}

// RunWithAgentParts supports multimodal user content, including image_url data URLs.
func (a *Agent) RunWithAgentParts(ctx context.Context, sessionID, agentID string, parts []types.ContentPart) (<-chan Event, error) {
	prompt := ""
	for _, p := range parts {
		if p.Type == "text" {
			prompt += p.Text
		}
	}
	profile, _ := a.GetProfile(agentID)
	model, tier := a.Route(prompt, 0)
	if profile.Model != "" {
		model = profile.Model
	}
	events := make(chan Event, 64)

	go func() {
		defer close(events)
		events <- Event{Type: "route", Model: model, Tier: string(tier)}

		sess := a.session.GetOrCreate(sessionID)
		if a.cfg.Agent.Context.AutoCompress {
			if summary, ok := sess.MaybeCompress(a.cfg.Agent.Context.MaxTokens, a.cfg.Agent.Context.CompressThreshold, a.cfg.Agent.Context.KeepLastMessages); ok {
				events <- Event{Type: "status", Text: fmt.Sprintf("context compressed: %d -> %d tokens", summary.TokenBefore, summary.TokenAfter)}
			}
		}
		if len(parts) == 1 && parts[0].Type == "text" {
			sess.Append("user", prompt)
		} else {
			sess.AppendMessage(Message{Role: "user", Content: parts})
		}

		events <- Event{Type: "status", Text: "thinking..."}
		policy := runtimePolicyFromProfile(profile)
		sess.AppendRun(session.RunInfo{AgentID: profile.ID, Model: model, Prompt: prompt, Tools: a.toolNamesForPolicy(policy), MCPServers: policy.mcpNames()})

		maxTurns := profile.MaxTurns
		if maxTurns <= 0 {
			maxTurns = a.cfg.Agent.MaxTurns
		}
		if maxTurns <= 0 || maxTurns > 20 {
			maxTurns = 20
		}

		for turn := 0; turn < maxTurns; turn++ {
			messages := a.withSystemPrompt(sess.Snapshot().Messages, profile)
			response, toolCalls, err := a.callLLMStream(ctx, model, messages, policy, func(delta string) {
				if delta != "" {
					events <- Event{Type: "content", Text: delta}
				}
			})
			if err != nil {
				events <- Event{Type: "error", Text: err.Error()}
				return
			}

			sess.AppendMessage(Message{Role: "assistant", Content: response, ToolCalls: toolCalls})
			if len(toolCalls) == 0 {
				events <- Event{Type: "done"}
				return
			}

			for _, tc := range toolCalls {
				events <- Event{Type: "tool_start", ToolName: tc.Name, ToolID: tc.ID}
				result := a.executeTool(ctx, tc, policy)
				events <- Event{Type: "tool_result", ToolID: tc.ID, Text: result}
				sess.AppendTool(tc.ID, tc.Name, result)
			}
			events <- Event{Type: "status", Text: "tools complete; continuing..."}
		}
		events <- Event{Type: "error", Text: fmt.Sprintf("max agent turns reached (%d)", maxTurns)}
	}()

	return events, nil
}

func (a *Agent) withSystemPrompt(messages []Message, profile config.AgentProfile) []Message {
	var parts []string
	if sp := strings.TrimSpace(profile.SystemPrompt); sp != "" {
		parts = append(parts, sp)
	}
	if mem := strings.TrimSpace(a.memory.BuildSystemPrompt("")); mem != "" {
		parts = append(parts, mem)
	}
	enabledSkills := profile.EnabledSkills
	if len(enabledSkills) == 0 {
		enabledSkills = a.enabledSkills
	}
	if prompt := strings.TrimSpace(a.skills.BuildPrompt(enabledSkills)); prompt != "" {
		parts = append(parts, prompt)
	}
	if len(parts) == 0 {
		return messages
	}
	out := make([]Message, 0, len(messages)+1)
	out = append(out, Message{Role: "system", Content: strings.Join(parts, "\n")})
	out = append(out, messages...)
	return out
}

type chatCompletionRequest struct {
	Model    string                 `json:"model"`
	Messages []Message              `json:"messages"`
	Tools    []map[string]any       `json:"tools,omitempty"`
	Stream   bool                   `json:"stream,omitempty"`
	Extra    map[string]interface{} `json:"-"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error any `json:"error,omitempty"`
}

type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type runtimePolicy struct {
	AllowedTools      map[string]bool
	AllowedMCPServers map[string]bool
}

func runtimePolicyFromProfile(profile config.AgentProfile) runtimePolicy {
	policy := runtimePolicy{AllowedTools: map[string]bool{}, AllowedMCPServers: map[string]bool{}}
	for _, name := range profile.EnabledTools {
		if strings.TrimSpace(name) != "" {
			policy.AllowedTools[name] = true
		}
	}
	for _, id := range profile.EnabledMCPServers {
		if strings.TrimSpace(id) != "" {
			policy.AllowedMCPServers[id] = true
		}
	}
	return policy
}

func (p runtimePolicy) mcpNames() []string {
	out := make([]string, 0, len(p.AllowedMCPServers))
	for name := range p.AllowedMCPServers {
		out = append(out, name)
	}
	return out
}

func (a *Agent) toolNamesForPolicy(policy runtimePolicy) []string {
	if len(policy.AllowedTools) > 0 {
		out := make([]string, 0, len(policy.AllowedTools))
		for name := range policy.AllowedTools {
			out = append(out, name)
		}
		return out
	}
	return a.tools.List()
}

// callLLMStream calls OpenAI-compatible Chat Completions. It requests stream=true
// and falls back to non-stream parsing if the provider returns JSON.
func (a *Agent) callLLMStream(ctx context.Context, model string, messages []Message, policy runtimePolicy, onDelta func(string)) (string, []ToolCall, error) {
	return a.callLLM(ctx, model, messages, policy, true, onDelta)
}

func (a *Agent) callLLM(ctx context.Context, model string, messages []Message, policy runtimePolicy, stream bool, onDelta func(string)) (string, []ToolCall, error) {
	baseURL := strings.TrimRight(a.cfg.Agent.ProxyURL, "/")
	if baseURL == "" {
		return "", nil, fmt.Errorf("agent proxy-url is empty")
	}
	apiKey := strings.TrimSpace(os.Getenv("UUAGENT_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}

	toolDefs := a.tools.DefinitionsFor(policy.AllowedTools)
	if len(policy.AllowedMCPServers) == 0 || policy.AllowedMCPServers["mock"] {
		toolDefs = append(toolDefs, map[string]any{"type": "function", "function": map[string]any{"name": "mcp_echo", "description": "Echo arguments through the mock MCP client."}})
	}
	payload := chatCompletionRequest{Model: model, Messages: messages, Tools: toolDefs, Stream: stream}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		return "", nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(data))
	}
	ct := resp.Header.Get("Content-Type")
	if stream && strings.Contains(ct, "text/event-stream") {
		return parseChatStream(resp.Body, onDelta)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return parseChatJSON(data, onDelta)
}

func parseChatJSON(data []byte, onDelta func(string)) (string, []ToolCall, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", nil, err
	}
	if len(parsed.Choices) == 0 {
		return "", nil, fmt.Errorf("llm response has no choices")
	}
	msg := parsed.Choices[0].Message
	if msg.Content != "" && onDelta != nil {
		onDelta(msg.Content)
	}
	calls := make([]ToolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		calls = append(calls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: tc.Function.Arguments})
	}
	return msg.Content, calls, nil
}

type streamToolCall struct{ id, name, args string }

func parseChatStream(r io.Reader, onDelta func(string)) (string, []ToolCall, error) {
	var content strings.Builder
	toolsByIndex := map[int]*streamToolCall{}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				idx := bytes.IndexByte(buf, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimSpace(string(buf[:idx]))
				buf = buf[idx+1:]
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if payload == "" {
					continue
				}
				if payload == "[DONE]" {
					text, calls := streamResult(content.String(), toolsByIndex)
					return text, calls, nil
				}
				var chunk chatCompletionChunk
				if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
					continue
				}
				for _, choice := range chunk.Choices {
					if choice.Delta.Content != "" {
						content.WriteString(choice.Delta.Content)
						if onDelta != nil {
							onDelta(choice.Delta.Content)
						}
					}
					for _, tc := range choice.Delta.ToolCalls {
						a := toolsByIndex[tc.Index]
						if a == nil {
							a = &streamToolCall{}
							toolsByIndex[tc.Index] = a
						}
						if tc.ID != "" {
							a.id = tc.ID
						}
						if tc.Function.Name != "" {
							a.name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							a.args += tc.Function.Arguments
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, err
		}
	}
	text, calls := streamResult(content.String(), toolsByIndex)
	return text, calls, nil
}

func streamResult(content string, toolsByIndex map[int]*streamToolCall) (string, []ToolCall) {
	calls := make([]ToolCall, 0, len(toolsByIndex))
	for _, tc := range toolsByIndex {
		if tc.name != "" {
			calls = append(calls, ToolCall{ID: tc.id, Name: tc.name, Args: tc.args})
		}
	}
	return content, calls
}

func (a *Agent) executeTool(ctx context.Context, tc ToolCall, policy runtimePolicy) string {
	if a.blockedTools != nil && a.blockedTools[tc.Name] {
		return fmt.Sprintf("tool %s is blocked", tc.Name)
	}
	if len(policy.AllowedTools) > 0 && !strings.HasPrefix(tc.Name, "mcp_") && !policy.AllowedTools[tc.Name] {
		return fmt.Sprintf("tool %s is not enabled for this agent", tc.Name)
	}
	if strings.HasPrefix(tc.Name, "mcp_") && len(policy.AllowedMCPServers) > 0 && !policy.AllowedMCPServers["mock"] {
		return fmt.Sprintf("mcp tool %s is not enabled for this agent", tc.Name)
	}
	args := map[string]any{}
	if strings.TrimSpace(tc.Args) != "" {
		if err := json.Unmarshal([]byte(tc.Args), &args); err != nil {
			return "invalid tool args: " + err.Error()
		}
	}
	if tool, ok := a.tools.Get(tc.Name); ok {
		out, err := tool.Execute(ctx, args)
		if err != nil {
			return err.Error()
		}
		return out
	}
	if strings.HasPrefix(tc.Name, "mcp_") && a.mcp != nil {
		out, err := a.mcp.CallTool(ctx, tc.Name, args)
		if err != nil {
			return err.Error()
		}
		return out
	}
	return fmt.Sprintf("tool %s not found", tc.Name)
}

// ConfigFromModelFile parses tests/models.txt-like JSON fragments without
// persisting secrets. It is intentionally available only as a helper for tests.
func ConfigFromModelFile(path string) (*config.Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	text := "{" + strings.Trim(strings.TrimSpace(string(data)), ",") + "}"
	var raw struct {
		BaseURL string `json:"baseUrl"`
		Models  []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, err
	}
	cfg := config.Default()
	cfg.Agent.ProxyURL = strings.TrimRight(raw.BaseURL, "/")
	if len(raw.Models) > 0 {
		cfg.Agent.Routing.Tiers["strong"] = []string{raw.Models[0].ID}
		cfg.Agent.Routing.Fallback = "strong"
	}
	return cfg, nil
}
