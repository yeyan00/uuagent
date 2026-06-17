package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/contextmgr"
	"github.com/yeyan00/uuagent/internal/mcp"
	"github.com/yeyan00/uuagent/internal/memory"
	"github.com/yeyan00/uuagent/internal/paths"
	"github.com/yeyan00/uuagent/internal/project"
	"github.com/yeyan00/uuagent/internal/router"
	"github.com/yeyan00/uuagent/internal/session"
	"github.com/yeyan00/uuagent/internal/skills"
	"github.com/yeyan00/uuagent/internal/tools"
	"github.com/yeyan00/uuagent/internal/types"
)

// Re-export shared chat types for compatibility with callers in this package.
type Event = types.Event
type Message = types.Message
type ToolCall = types.ToolCall

// Agent is the core runtime object.
type Agent struct {
	cfg           *config.Config
	router        *router.Router
	session       *session.Store
	sessionStores map[string]*session.Store
	sessionsMu    sync.Mutex
	tools         *tools.Registry
	skills        *skills.Registry
	mcp           mcp.Client
	memory        *memory.Manager
	projects      *project.Store
	fixedModel    string
	blockedTools  map[string]bool
	enabledSkills []string
	httpClient    *http.Client
	runs             map[string]context.CancelFunc
	runsMu           sync.Mutex
	pendingApprovals map[string]pendingApproval
	approvalsMu      sync.Mutex
}

type pendingApproval struct {
	Session       *session.Session
	RunID         string
	Model         string
	Prompt        string
	Profile       config.AgentProfile
	Policy        runtimePolicy
	ToolCall      ToolCall
	ToolName      string
	ToolWorkspace string
}

// New creates an Agent.
func New(cfg *config.Config) *Agent {
	workspace, _ := os.Getwd()
	return &Agent{
		cfg:           cfg,
		router:        router.New(cfg.Agent.Routing),
		session:       session.NewStore(),
		sessionStores: map[string]*session.Store{},
		tools:         tools.NewRegistry(workspace),
		skills:        skills.NewRegistryFromConfig(cfg),
		mcp:           mcp.NewMockClient(),
		memory:        memory.NewManager(),
		projects:      project.NewStore(""),
		httpClient:       &http.Client{Timeout: 120 * time.Second},
		runs:             map[string]context.CancelFunc{},
		pendingApprovals: map[string]pendingApproval{},
	}
}

var ErrSessionProjectConflict = errors.New("session is bound to another project")

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

// NewChildWithSession creates a restricted child agent with an isolated session store.
func (a *Agent) NewChildWithSession(model string, blockedTools map[string]bool, store *session.Store) *Agent {
	child := a.NewChild(model, blockedTools)
	if store != nil {
		child.session = store
	}
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

// ReloadProjectSkills overlays project-local skills onto the active registry.
func (a *Agent) ReloadProjectSkills(workspace string) {
	a.skills.ScanProject(workspace)
}

// Sessions exposes the session store for API handlers.
func (a *Agent) Sessions() *session.Store { return a.session }

// ProjectSessions returns the session store persisted under the project's config directory.
func (a *Agent) ProjectSessions(projectID string) (*session.Store, project.Project, bool) {
	p, ok := a.projects.Get(projectID)
	if !ok {
		return nil, project.Project{}, false
	}
	return a.projectSessionStore(p), p, true
}

// Memories exposes the memory manager for API handlers.
func (a *Agent) Memories() *memory.Manager { return a.memory }

// RefreshSessionMemorySnapshot rebuilds the frozen memory snapshot for a session.
func (a *Agent) RefreshSessionMemorySnapshot(sessionID, projectID, agentID string) string {
	store, p, ok := a.ProjectSessions(projectID)
	memoryProject := projectID
	if ok {
		memoryProject = p.WorkspacePath
	} else {
		store = a.session
	}
	sess := store.GetOrCreate(sessionID)
	snapshot := a.memory.BuildScopedSystemPrompt(memoryProject, agentID, sessionID)
	sess.RefreshMemorySnapshot(snapshot)
	return snapshot
}

// Projects exposes the project store for API handlers.
func (a *Agent) Projects() *project.Store { return a.projects }

func (a *Agent) projectSessionStore(p project.Project) *session.Store {
	root := filepath.Join(paths.ProjectsDir(), p.ID, "sessions")
	a.sessionsMu.Lock()
	defer a.sessionsMu.Unlock()
	if store, ok := a.sessionStores[root]; ok {
		return store
	}
	store := session.NewStoreAt(root)
	a.sessionStores[root] = store
	return store
}

func (a *Agent) projectRunContext(projectID string) (store *session.Store, projectIDOut, projectPath, memoryProject string, registered bool) {
	if p, ok := a.projects.Get(projectID); ok {
		return a.projectSessionStore(p), p.ID, p.WorkspacePath, p.WorkspacePath, true
	}
	if strings.TrimSpace(projectID) != "" {
		return a.session, projectID, projectID, projectID, false
	}
	return a.session, "", "", "", false
}

func (a *Agent) sessionExistsInOtherProject(sessionID, projectID string) bool {
	if sessionID == "" || projectID == "" {
		return false
	}
	for _, p := range a.projects.List() {
		if p.ID == projectID {
			continue
		}
		if sess, ok := a.projectSessionStore(p).Get(sessionID); ok {
			snap := sess.Snapshot()
			if len(snap.Messages) > 0 || len(snap.Runs) > 0 {
				return true
			}
		}
	}
	return false
}

// Skills returns the active skill registry for API handlers.
func (a *Agent) Skills() *skills.Registry { return a.skills }

// MCPTools returns tool descriptors exposed by the MCP client.
func (a *Agent) MCPTools(ctx context.Context) []mcp.Tool {
	if a.mcp == nil || !a.isMCPServerEnabled("mock") {
		return nil
	}
	tools, err := a.mcp.ListTools(ctx)
	if err != nil {
		return nil
	}
	return tools
}

func (a *Agent) isMCPServerEnabled(id string) bool {
	for _, srv := range a.cfg.MCPServers {
		if srv.ID == id {
			return srv.Enabled
		}
	}
	return false
}

// ToolNames returns built-in tool names.
func (a *Agent) ToolNames() []string { return a.tools.List() }

// StopRun cancels an active agent run by id.
func (a *Agent) StopRun(runID string) bool {
	a.runsMu.Lock()
	cancel, ok := a.runs[runID]
	a.runsMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func (a *Agent) registerRun(parent context.Context, runID string) context.Context {
	ctx, cancel := context.WithCancel(parent)
	a.runsMu.Lock()
	a.runs[runID] = cancel
	a.runsMu.Unlock()
	return ctx
}

func (a *Agent) unregisterRun(runID string) {
	a.runsMu.Lock()
	delete(a.runs, runID)
	a.runsMu.Unlock()
}

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
			return a.normalizeProfile(p), true
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

// SubagentProfiles returns configured subagent profiles.
func (a *Agent) SubagentProfiles() []config.SubagentProfile {
	out := make([]config.SubagentProfile, len(a.cfg.Agent.Subagent.Profiles))
	copy(out, a.cfg.Agent.Subagent.Profiles)
	return out
}

// UpsertSubagentProfile creates or updates a subagent profile and persists user config.
func (a *Agent) UpsertSubagentProfile(profile config.SubagentProfile) (config.SubagentProfile, error) {
	profile = normalizeSubagentProfile(profile)
	for i := range a.cfg.Agent.Subagent.Profiles {
		if a.cfg.Agent.Subagent.Profiles[i].ID == profile.ID {
			a.cfg.Agent.Subagent.Profiles[i] = profile
			return profile, config.SaveUser(a.cfg)
		}
	}
	a.cfg.Agent.Subagent.Profiles = append(a.cfg.Agent.Subagent.Profiles, profile)
	return profile, config.SaveUser(a.cfg)
}

// DeleteSubagentProfile removes a configured subagent profile and persists user config.
func (a *Agent) DeleteSubagentProfile(id string) error {
	for i := range a.cfg.Agent.Subagent.Profiles {
		if a.cfg.Agent.Subagent.Profiles[i].ID == id {
			a.cfg.Agent.Subagent.Profiles = append(a.cfg.Agent.Subagent.Profiles[:i], a.cfg.Agent.Subagent.Profiles[i+1:]...)
			return config.SaveUser(a.cfg)
		}
	}
	return fmt.Errorf("subagent profile not found")
}

func normalizeSubagentProfile(profile config.SubagentProfile) config.SubagentProfile {
	if profile.ID == "" {
		profile.ID = strings.ToLower(strings.ReplaceAll(profile.Name, " ", "-"))
	}
	if profile.ID == "" {
		profile.ID = fmt.Sprintf("subagent-%d", time.Now().UnixNano())
	}
	if profile.Name == "" {
		profile.Name = profile.ID
	}
	return profile
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

// Run executes one conversation turn.
func (a *Agent) Run(ctx context.Context, sessionID string, prompt string) (<-chan Event, error) {
	return a.RunWithAgent(ctx, sessionID, "", prompt)
}

// RunWithAgent runs one turn with an optional configured AgentProfile.
func (a *Agent) RunWithAgent(ctx context.Context, sessionID, agentID string, prompt string) (<-chan Event, error) {
	return a.RunWithAgentProjectParts(ctx, sessionID, agentID, "", []types.ContentPart{{Type: "text", Text: prompt}})
}

// RunWithAgentParts supports multimodal user content, including image_url data URLs.
func (a *Agent) RunWithAgentParts(ctx context.Context, sessionID, agentID string, parts []types.ContentPart) (<-chan Event, error) {
	return a.RunWithAgentProjectParts(ctx, sessionID, agentID, "", parts)
}

// RunWithAgentProjectParts runs one turn with project-scoped memory injection.
func (a *Agent) RunWithAgentProjectParts(ctx context.Context, sessionID, agentID, projectID string, parts []types.ContentPart) (<-chan Event, error) {
	profile, _ := a.GetProfile(agentID)
	return a.runWithResolvedProfile(ctx, sessionID, projectID, profile, parts)
}

func (a *Agent) runWithResolvedProfile(ctx context.Context, sessionID, projectID string, profile config.AgentProfile, parts []types.ContentPart) (<-chan Event, error) {
	prompt := ""
	for _, p := range parts {
		if p.Type == "text" {
			prompt += p.Text
		}
	}
	sessionStore, boundProjectID, projectPath, memoryProject, registeredProject := a.projectRunContext(projectID)
	if registeredProject && a.sessionExistsInOtherProject(sessionID, boundProjectID) {
		return nil, ErrSessionProjectConflict
	}
	model, tier := a.Route(prompt, 0)
	if profile.Model != "" {
		model = profile.Model
	}
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	ctx = a.registerRun(ctx, runID)
	events := make(chan Event, 64)

	go func() {
		defer close(events)
		defer a.unregisterRun(runID)
		events <- Event{Type: "run", RunID: runID}
		events <- Event{Type: "route", RunID: runID, Model: model, Tier: string(tier)}

		sess := sessionStore.GetOrCreate(sessionID)
		if registeredProject {
			if err := sess.BindProject(boundProjectID, projectPath); err != nil {
				events <- Event{Type: "error", RunID: runID, Text: err.Error()}
				return
			}
		}
		if a.cfg.Agent.Context.AutoCompress {
			if summary, ok := sess.MaybeCompress(a.cfg.Agent.Context.MaxTokens, a.cfg.Agent.Context.CompressThreshold, a.cfg.Agent.Context.KeepLastMessages); ok {
				events <- Event{Type: "status", RunID: runID, Text: fmt.Sprintf("context compressed: %d -> %d tokens", summary.TokenBefore, summary.TokenAfter)}
			}
		}
		sess.MaybeTitleFromPrompt(prompt)
		if len(parts) == 1 && parts[0].Type == "text" {
			sess.Append("user", prompt)
		} else {
			sess.AppendMessage(Message{Role: "user", Content: parts})
		}

		events <- Event{Type: "status", RunID: runID, Text: "thinking..."}
		policy := runtimePolicyFromProfile(profile)
		sess.AppendRun(session.RunInfo{ID: runID, Status: "running", AgentID: profile.ID, ProjectID: boundProjectID, ProjectPath: projectPath, Model: model, Prompt: prompt, Tools: a.toolNamesForPolicy(policy), MCPServers: policy.mcpNames()})
		defer func() {
			if ctx.Err() != nil {
				sess.UpdateRunStatus(runID, "cancelled")
			}
		}()

		maxTurns := profile.MaxTurns
		if maxTurns <= 0 {
			maxTurns = a.cfg.Agent.MaxTurns
		}
		if maxTurns <= 0 || maxTurns > 20 {
			maxTurns = 20
		}

		toolWorkspace := a.toolWorkspace(projectPath)
		for turn := 0; turn < maxTurns; turn++ {
			if ctx.Err() != nil {
				events <- Event{Type: "error", RunID: runID, Text: ctx.Err().Error()}
				return
			}
			memorySnapshot := sess.EnsureMemorySnapshot(a.memory.BuildScopedSystemPrompt(memoryProject, profile.ID, sessionID))
			messages := a.withSystemPrompt(sess.Snapshot().Messages, profile, memorySnapshot, prompt)
			response, toolCalls, usage, err := a.callLLMStream(ctx, model, messages, policy, func(delta string) {
				if delta != "" {
					events <- Event{Type: "content", RunID: runID, Text: delta}
				}
			})
			if err != nil {
				events <- Event{Type: "error", RunID: runID, Text: err.Error()}
				return
			}

			sess.AppendMessage(Message{Role: "assistant", Content: response, ToolCalls: toolCalls})
			sess.AddRunUsage(runID, usage.withFallback(messages, response))
			if len(toolCalls) == 0 {
				a.queueAutoDraftMemory(prompt, response, memoryProject)
				sess.UpdateRunStatus(runID, "done")
				events <- Event{Type: "done", RunID: runID}
				return
			}

			for _, tc := range toolCalls {
				name := toolCallName(tc)
				events <- Event{Type: "tool_start", RunID: runID, ToolName: name, ToolID: tc.ID}
				result := a.executeTool(ctx, tc, policy, toolWorkspace)
				events <- Event{Type: "tool_result", RunID: runID, ToolID: tc.ID, Text: result}
				sess.AppendTool(tc.ID, name, result)
				if isApprovalRequiredResult(result) {
					a.storePendingApproval(pendingApproval{Session: sess, RunID: runID, Model: model, Prompt: prompt, Profile: profile, Policy: policy, ToolCall: tc, ToolName: name, ToolWorkspace: toolWorkspace})
					sess.UpdateRunStatus(runID, "waiting_approval")
					events <- Event{Type: "status", RunID: runID, Text: "approval required"}
					return
				}
			}
			events <- Event{Type: "status", RunID: runID, Text: "tools complete; continuing..."}
		}
		events <- Event{Type: "error", RunID: runID, Text: fmt.Sprintf("max agent turns reached (%d)", maxTurns)}
	}()

	return events, nil
}

func (a *Agent) storePendingApproval(p pendingApproval) {
	a.approvalsMu.Lock()
	defer a.approvalsMu.Unlock()
	a.pendingApprovals[p.RunID] = p
}

func (a *Agent) takePendingApproval(runID string) (pendingApproval, bool) {
	a.approvalsMu.Lock()
	defer a.approvalsMu.Unlock()
	p, ok := a.pendingApprovals[runID]
	if ok {
		delete(a.pendingApprovals, runID)
	}
	return p, ok
}

func (a *Agent) ApproveRun(ctx context.Context, runID string) (string, error) {
	events, err := a.ApproveRunEvents(ctx, runID)
	if err != nil {
		return "", err
	}
	var content strings.Builder
	for evt := range events {
		if evt.Type == "error" {
			return content.String(), fmt.Errorf(evt.Text)
		}
		if evt.Type == "content" {
			content.WriteString(evt.Text)
		}
	}
	return content.String(), nil
}

func (a *Agent) ApproveRunEvents(ctx context.Context, runID string) (<-chan Event, error) {
	p, ok := a.takePendingApproval(runID)
	if !ok {
		return nil, fmt.Errorf("pending approval not found")
	}
	events := make(chan Event, 32)
	go func() {
		defer close(events)
		args := map[string]any{}
		if argText := strings.TrimSpace(toolCallArgs(p.ToolCall)); argText != "" {
			_ = json.Unmarshal([]byte(argText), &args)
		}
		args["approved"] = true
		p.ToolCall.Function.Arguments = mustJSON(args)
		events <- Event{Type: "tool_start", RunID: runID, ToolName: p.ToolName, ToolID: p.ToolCall.ID}
		result := a.executeTool(ctx, p.ToolCall, p.Policy, p.ToolWorkspace)
		events <- Event{Type: "tool_result", RunID: runID, ToolName: p.ToolName, ToolID: p.ToolCall.ID, Text: result}
		p.Session.AppendTool(p.ToolCall.ID, p.ToolName, result)

		maxTurns := p.Profile.MaxTurns
		if maxTurns <= 0 {
			maxTurns = a.cfg.Agent.MaxTurns
		}
		if maxTurns <= 0 || maxTurns > 20 {
			maxTurns = 20
		}
		for turn := 0; turn < maxTurns; turn++ {
			memorySnapshot := p.Session.EnsureMemorySnapshot(a.memory.BuildScopedSystemPrompt(p.Session.Snapshot().ProjectPath, p.Profile.ID, p.Session.Snapshot().ID))
			messages := a.withSystemPrompt(p.Session.Snapshot().Messages, p.Profile, memorySnapshot, p.Prompt)
			response, toolCalls, usage, err := a.callLLM(ctx, p.Model, messages, p.Policy, false, func(delta string) {
				if delta != "" {
					events <- Event{Type: "content", RunID: runID, Text: delta}
				}
			})
			if err != nil {
				p.Session.UpdateRunStatus(runID, "failed")
				events <- Event{Type: "error", RunID: runID, Text: err.Error()}
				return
			}
			p.Session.AppendMessage(Message{Role: "assistant", Content: response, ToolCalls: toolCalls})
			p.Session.AddRunUsage(runID, usage.withFallback(messages, response))
			if len(toolCalls) == 0 {
				p.Session.UpdateRunStatus(runID, "done")
				events <- Event{Type: "done", RunID: runID}
				return
			}
			for _, tc := range toolCalls {
				name := toolCallName(tc)
				events <- Event{Type: "tool_start", RunID: runID, ToolName: name, ToolID: tc.ID}
				result := a.executeTool(ctx, tc, p.Policy, p.ToolWorkspace)
				events <- Event{Type: "tool_result", RunID: runID, ToolName: name, ToolID: tc.ID, Text: result}
				p.Session.AppendTool(tc.ID, name, result)
				if isApprovalRequiredResult(result) {
					a.storePendingApproval(pendingApproval{Session: p.Session, RunID: runID, Model: p.Model, Prompt: p.Prompt, Profile: p.Profile, Policy: p.Policy, ToolCall: tc, ToolName: name, ToolWorkspace: p.ToolWorkspace})
					p.Session.UpdateRunStatus(runID, "waiting_approval")
					events <- Event{Type: "status", RunID: runID, Text: "approval required"}
					return
				}
			}
		}
		p.Session.UpdateRunStatus(runID, "failed")
		events <- Event{Type: "error", RunID: runID, Text: fmt.Sprintf("max agent turns reached (%d)", maxTurns)}
	}()
	return events, nil
}

func (a *Agent) DenyRun(runID string) error {
	p, ok := a.takePendingApproval(runID)
	if !ok {
		return fmt.Errorf("pending approval not found")
	}
	p.Session.UpdateRunStatus(runID, "denied")
	return nil
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func isApprovalRequiredResult(result string) bool {
	var payload struct {
		ApprovalRequired bool `json:"approval_required"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return false
	}
	return payload.ApprovalRequired
}

func (a *Agent) queueAutoDraftMemory(userPrompt, assistantResponse, projectID string) {
	if !a.cfg.Agent.Memory.AutoDraft {
		return
	}
	content := extractMemoryDraft(userPrompt, assistantResponse)
	if strings.TrimSpace(content) == "" {
		return
	}
	go a.memory.Add(content, projectID, "project", "ai", memory.StatusDraft)
}

func extractMemoryDraft(userPrompt, assistantResponse string) string {
	text := strings.TrimSpace(userPrompt)
	lower := strings.ToLower(text)
	markers := []string{"remember that ", "remember: ", "记住", "以后", "我的偏好是", "项目约定是"}
	for _, marker := range markers {
		idx := strings.Index(lower, strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		content := strings.TrimSpace(text[idx+len(marker):])
		content = strings.Trim(content, "。\t\r\n")
		if content != "" {
			return content
		}
	}
	return ""
}

// RunWithProfileParts runs one turn with an explicit profile, used by delegated subagents.
func (a *Agent) RunWithProfileParts(ctx context.Context, sessionID, projectID string, profile config.AgentProfile, parts []types.ContentPart) (<-chan Event, error) {
	return a.runWithResolvedProfile(ctx, sessionID, projectID, profile, parts)
}

func (a *Agent) withSystemPrompt(messages []Message, profile config.AgentProfile, memorySnapshot, prompt string) []Message {
	var parts []string
	if sp := strings.TrimSpace(profile.SystemPrompt); sp != "" {
		parts = append(parts, sp)
	}
	if mem := strings.TrimSpace(memorySnapshot); mem != "" {
		parts = append(parts, mem)
	}
	enabledSkills := profile.EnabledSkills
	if len(enabledSkills) == 0 {
		enabledSkills = a.enabledSkills
	}
	if prompt := strings.TrimSpace(a.skills.BuildPrompt(enabledSkills)); prompt != "" {
		parts = append(parts, prompt)
	}
	parts = append(parts, a.skills.ExplicitContentsFromPrompt(prompt)...)
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
	Usage tokenUsage `json:"usage"`
	Error any        `json:"error,omitempty"`
}

type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u tokenUsage) withFallback(messages []Message, response string) session.TokenUsage {
	if u.PromptTokens > 0 || u.CompletionTokens > 0 || u.TotalTokens > 0 {
		total := u.TotalTokens
		if total == 0 {
			total = u.PromptTokens + u.CompletionTokens
		}
		return session.TokenUsage{InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens, TotalTokens: total}
	}
	input := contextmgr.EstimateTokens(messages)
	output := contextmgr.EstimateTokens([]Message{{Role: "assistant", Content: response}})
	return session.TokenUsage{EstimatedInputTokens: input, EstimatedOutputTokens: output, TotalTokens: input + output, Estimated: true}
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
	PermissionMode    string
}

func runtimePolicyFromProfile(profile config.AgentProfile) runtimePolicy {
	policy := runtimePolicy{AllowedTools: map[string]bool{}, AllowedMCPServers: map[string]bool{}, PermissionMode: profile.PermissionMode}
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
func (a *Agent) callLLMStream(ctx context.Context, model string, messages []Message, policy runtimePolicy, onDelta func(string)) (string, []ToolCall, tokenUsage, error) {
	return a.callLLM(ctx, model, messages, policy, true, onDelta)
}

func (a *Agent) callLLM(ctx context.Context, model string, messages []Message, policy runtimePolicy, stream bool, onDelta func(string)) (string, []ToolCall, tokenUsage, error) {
	baseURL := strings.TrimRight(a.cfg.Agent.ProxyURL, "/")
	if baseURL == "" {
		return "", nil, tokenUsage{}, fmt.Errorf("agent proxy-url is empty")
	}
	apiKey := strings.TrimSpace(os.Getenv("UUAGENT_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}

	toolDefs := tools.NewRegistryWithOptions(".", tools.Options{PermissionMode: toolPermissionMode(policy.PermissionMode)}).DefinitionsFor(policy.AllowedTools)
	if len(policy.AllowedTools) == 0 || policy.AllowedTools["memory"] {
		toolDefs = append(toolDefs, memoryToolDefinition())
	}
	if a.isMCPServerEnabled("mock") && (len(policy.AllowedMCPServers) == 0 || policy.AllowedMCPServers["mock"]) {
		toolDefs = append(toolDefs, map[string]any{"type": "function", "function": map[string]any{"name": "mcp_echo", "description": "Echo arguments through the mock MCP client."}})
	}
	payload := chatCompletionRequest{Model: model, Messages: messages, Tools: toolDefs, Stream: stream}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, tokenUsage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", nil, tokenUsage{}, err
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
		return "", nil, tokenUsage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		return "", nil, tokenUsage{}, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(data))
	}
	ct := resp.Header.Get("Content-Type")
	if stream && strings.Contains(ct, "text/event-stream") {
		text, calls, err := parseChatStream(resp.Body, onDelta)
		return text, calls, tokenUsage{}, err
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return parseChatJSON(data, onDelta)
}

func parseChatJSON(data []byte, onDelta func(string)) (string, []ToolCall, tokenUsage, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", nil, tokenUsage{}, err
	}
	if len(parsed.Choices) == 0 {
		return "", nil, tokenUsage{}, fmt.Errorf("llm response has no choices")
	}
	msg := parsed.Choices[0].Message
	if msg.Content != "" && onDelta != nil {
		onDelta(msg.Content)
	}
	calls := make([]ToolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		calls = append(calls, normalizeToolCall(ToolCall{ID: tc.ID, Type: tc.Type, Function: types.ToolFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments}}))
	}
	return msg.Content, calls, parsed.Usage, nil
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
			calls = append(calls, normalizeToolCall(ToolCall{ID: tc.id, Type: "function", Function: types.ToolFunction{Name: tc.name, Arguments: tc.args}}))
		}
	}
	return content, calls
}

func (a *Agent) executeTool(ctx context.Context, tc ToolCall, policy runtimePolicy, workspace string) string {
	name := toolCallName(tc)
	argText := toolCallArgs(tc)
	if a.blockedTools != nil && a.blockedTools[name] {
		return fmt.Sprintf("tool %s is blocked", name)
	}
	if len(policy.AllowedTools) > 0 && !strings.HasPrefix(name, "mcp_") && !policy.AllowedTools[name] {
		return fmt.Sprintf("tool %s is not enabled for this agent", name)
	}
	if strings.HasPrefix(name, "mcp_") && len(policy.AllowedMCPServers) > 0 && !policy.AllowedMCPServers["mock"] {
		return fmt.Sprintf("mcp tool %s is not enabled for this agent", name)
	}
	if strings.HasPrefix(name, "mcp_") && !a.isMCPServerEnabled("mock") {
		return fmt.Sprintf("mcp tool %s is not enabled", name)
	}
	args := map[string]any{}
	if strings.TrimSpace(argText) != "" {
		if err := json.Unmarshal([]byte(argText), &args); err != nil {
			return "invalid tool args: " + err.Error()
		}
	}
	if name == "memory" {
		return a.executeMemoryTool(args)
	}
	if tool, ok := a.tools.Get(name); ok {
		if policy.PermissionMode != "" {
			toolRegistry := tools.NewRegistryWithOptions(workspace, tools.Options{PermissionMode: toolPermissionMode(policy.PermissionMode)})
			if configuredTool, ok := toolRegistry.Get(name); ok {
				tool = configuredTool
			}
		}
		out, err := tool.Execute(ctx, args)
		if err != nil {
			return err.Error()
		}
		return out
	}
	if strings.HasPrefix(name, "mcp_") && a.mcp != nil {
		out, err := a.mcp.CallTool(ctx, name, args)
		if err != nil {
			return err.Error()
		}
		return out
	}
	return fmt.Sprintf("tool %s not found", name)
}

func memoryToolDefinition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "memory",
			"description": "Read or update persistent memory. Writes are durable immediately but do not change the current session's frozen memory snapshot until refresh or a new session.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":   map[string]any{"type": "string", "enum": []string{"add", "read", "replace", "remove"}},
					"content":  map[string]any{"type": "string"},
					"old_text": map[string]any{"type": "string"},
					"project":  map[string]any{"type": "string"},
					"scope":    map[string]any{"type": "string", "enum": []string{"global", "project", "agent", "session"}},
					"status":   map[string]any{"type": "string", "enum": []string{"draft", "confirmed"}},
				},
				"required": []string{"action"},
			},
		},
	}
}

func (a *Agent) executeMemoryTool(args map[string]any) string {
	action, _ := args["action"].(string)
	project, _ := args["project"].(string)
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "project"
	}
	status := memory.StatusConfirmed
	if raw, _ := args["status"].(string); raw == string(memory.StatusDraft) {
		status = memory.StatusDraft
	}
	respond := func(payload map[string]any) string {
		data, _ := json.Marshal(payload)
		return string(data)
	}
	switch action {
	case "add":
		content, _ := args["content"].(string)
		if strings.TrimSpace(content) == "" {
			return respond(map[string]any{"success": false, "error": "content is required"})
		}
		entry := a.memory.Add(content, project, scope, "ai", status)
		return respond(map[string]any{"success": true, "entry": entry, "note": "memory saved; current session snapshot is unchanged until refresh"})
	case "read":
		entries := a.memory.ListFiltered("", project, scope)
		return respond(map[string]any{"success": true, "entries": entries})
	case "replace":
		oldText, _ := args["old_text"].(string)
		content, _ := args["content"].(string)
		if oldText == "" || content == "" {
			return respond(map[string]any{"success": false, "error": "old_text and content are required"})
		}
		entry, ok := a.memory.ReplaceByContent(oldText, content, project, scope)
		return respond(map[string]any{"success": ok, "entry": entry})
	case "remove":
		oldText, _ := args["old_text"].(string)
		if oldText == "" {
			return respond(map[string]any{"success": false, "error": "old_text is required"})
		}
		ok := a.memory.DeleteByContent(oldText, project, scope)
		return respond(map[string]any{"success": ok})
	default:
		return respond(map[string]any{"success": false, "error": "unknown action"})
	}
}

func (a *Agent) toolWorkspace(projectID string) string {
	if projectID != "" {
		return projectID
	}
	workspace, err := os.Getwd()
	if err != nil || workspace == "" {
		return "."
	}
	return workspace
}

func toolPermissionMode(mode string) tools.PermissionMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "ask", "approval", "approval-required", "workspace-write":
		return tools.PermissionAsk
	case "allow", "unrestricted":
		return tools.PermissionAllow
	default:
		return tools.PermissionDeny
	}
}

func normalizeToolCall(tc ToolCall) ToolCall {
	if tc.Type == "" {
		tc.Type = "function"
	}
	if tc.Function.Name == "" {
		tc.Function.Name = tc.Name
	}
	if tc.Function.Arguments == "" {
		tc.Function.Arguments = tc.Args
	}
	if tc.Name == "" {
		tc.Name = tc.Function.Name
	}
	if tc.Args == "" {
		tc.Args = tc.Function.Arguments
	}
	return tc
}

func toolCallName(tc ToolCall) string {
	if tc.Function.Name != "" {
		return tc.Function.Name
	}
	return tc.Name
}

func toolCallArgs(tc ToolCall) string {
	if tc.Function.Arguments != "" {
		return tc.Function.Arguments
	}
	return tc.Args
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
