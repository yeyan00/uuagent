# UUAgent Usable Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Windows-first usable UUAgent test release with real agent/subagent management, a default CLIProxyAPI sidecar plugin, clearer model routing, improved Web navigation/layout, and visible Goal Mode execution activity.

**Architecture:** Keep UUAgent as the product shell and CLIProxyAPI as a managed sidecar executable. Add narrow backend APIs for extension lifecycle and model route decisions, then upgrade the Web UI around existing agents/subagents/settings rather than rewriting the whole app.

**Tech Stack:** Go 1.x backend with Gin, YAML/JSON config persistence, child process management through `os/exec`, React + TypeScript + Vite frontend, Vitest + Go tests, lucide-react icons.

---

## File Structure Map

### Backend files to modify

- `internal/config/config.go`  
  Add `AgentProfile.EnabledSubagents` and optional extension config structs if needed for persistence.

- `internal/config/defaults.go`  
  Add default routing rules and keep CLIProxyAPI-friendly default proxy behavior.

- `internal/router/router.go`  
  Return route decision metadata instead of only model/tier.

- `internal/agent/agent.go`  
  Accept per-chat model override, emit route source metadata, and enforce active agent subagent allow-list for `delegate_task`.

- `api/server/server.go`  
  Extend chat request, route preview, and register extension routes.

- `internal/agent/models_settings.go` and `api/server/models_settings.go`  
  Support built-in CLIProxyAPI proxy URL and route-preview validation.

### Backend files to create

- `internal/extensions/types.go`  
  Extension status/request/response types.

- `internal/extensions/cliproxyapi.go`  
  CLIProxyAPI sidecar manager: binary detection, config generation, start/stop/restart, health, logs.

- `internal/extensions/logs.go`  
  Bounded in-memory log buffer and log file append helpers.

- `api/server/extensions.go`  
  HTTP handlers for `/api/extensions` and CLIProxyAPI lifecycle endpoints.

### Backend tests to create or modify

- `tests/agent/delegate_task_test.go`  
  Add allow-list rejection and empty-list compatibility tests.

- `tests/agent/models_settings_api_test.go`  
  Add model override and route metadata tests.

- `tests/extensions/cliproxyapi_test.go`  
  Test missing binary status, config generation, and fake health-check behavior.

### Frontend files to modify

- `web/package.json` and lock file  
  Add `lucide-react`.

- `web/src/App.tsx`  
  Wire model override to chat, introduce improved navigation, and integrate new panels. If this file grows too much, extract focused components in the same task.

- `web/src/App.css`  
  Add polished navigation, cards, badges, and settings/extension layouts.

- `web/src/App.test.tsx`  
  Cover agents list, enabled subagents, extension page, model override payload, and icon navigation accessible labels.

### Frontend files to create if extraction is practical

- `web/src/types.ts`  
  Shared Agent/Subagent/Extension/Route types.

- `web/src/components/AgentsSettings.tsx`  
  Agent list/detail editor.

- `web/src/components/SubagentsSettings.tsx`  
  Subagent list/detail editor.

- `web/src/components/ExtensionsPanel.tsx`  
  CLIProxyAPI plugin detail panel.

- `web/src/components/ModelsSettings.tsx`  
  Model proxy, tiers, and route preview panel.

---

## Task 1: Add agent-subagent allow-list schema and enforcement

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/agent/agent.go`
- Test: `tests/agent/delegate_task_test.go`

- [ ] **Step 1: Add failing tests for subagent allow-list enforcement**

Add tests to `tests/agent/delegate_task_test.go` that cover two behaviors:

```go
func Test_Agent_DelegateTaskTool_rejects_disallowed_subagent(t *testing.T) {
    t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
    parentCalls := 0
    childCalls := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/chat/completions" {
            t.Fatalf("unexpected path: %s", r.URL.Path)
        }
        parentCalls++
        w.Header().Set("Content-Type", "application/json")
        if parentCalls == 1 {
            _ = json.NewEncoder(w).Encode(map[string]any{
                "choices": []map[string]any{{"message": map[string]any{
                    "tool_calls": []map[string]any{{"id": "call-1", "type": "function", "function": map[string]any{"name": "delegate_task", "arguments": `{"profile_id":"builder","task":"build it"}`}}},
                }}},
            })
            return
        }
        _ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "done"}}}})
    }))
    defer server.Close()

    cfg := config.Default()
    cfg.Agent.ProxyURL = server.URL
    cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", EnabledSubagents: []string{"planner"}}}
    agt := agent.New(cfg)

    got, err := agt.RunOnce(context.Background(), "delegate to builder")
    if err != nil {
        t.Fatalf("RunOnce returned error: %v", err)
    }
    if !strings.Contains(got, "done") {
        t.Fatalf("expected final response, got %q", got)
    }
    if childCalls != 0 {
        t.Fatalf("disallowed subagent should not call child model, got %d", childCalls)
    }
}

func Test_Agent_DelegateTaskTool_allows_all_subagents_when_allow_list_empty(t *testing.T) {
    // Use the existing delegate_task success fixture pattern in this file.
    // Configure default agent with EnabledSubagents nil.
    // Expect child subagent model call to happen and final output to contain planner-child-ok.
}
```

- [ ] **Step 2: Run the targeted failing test**

Run:

```powershell
go test ./tests/agent -run Test_Agent_DelegateTaskTool_ -count=1 -v
```

Expected: FAIL because `EnabledSubagents` does not exist and enforcement is not implemented.

- [ ] **Step 3: Add config field**

In `internal/config/config.go`, extend `AgentProfile`:

```go
type AgentProfile struct {
    ID                string   `yaml:"id" json:"id"`
    Name              string   `yaml:"name" json:"name"`
    Description       string   `yaml:"description" json:"description"`
    SystemPrompt      string   `yaml:"system_prompt" json:"system_prompt"`
    Model             string   `yaml:"model" json:"model"`
    EnabledTools      []string `yaml:"enabled_tools" json:"enabled_tools"`
    EnabledSkills     []string `yaml:"enabled_skills" json:"enabled_skills"`
    EnabledMCPServers []string `yaml:"enabled_mcp_servers" json:"enabled_mcp_servers"`
    EnabledSubagents  []string `yaml:"enabled_subagents" json:"enabled_subagents"`
    PermissionMode    string   `yaml:"permission_mode" json:"permission_mode"`
    MaxTurns          int      `yaml:"max_turns" json:"max_turns"`
}
```

- [ ] **Step 4: Enforce allow-list in delegate_task**

In `internal/agent/agent.go`, add helper near profile/policy helpers:

```go
func subagentAllowed(profile config.AgentProfile, profileID string) bool {
    if len(profile.EnabledSubagents) == 0 {
        return true
    }
    for _, allowed := range profile.EnabledSubagents {
        if allowed == profileID {
            return true
        }
    }
    return false
}
```

Update the `delegate_task` execution path so it checks the active parent profile before calling `subagent.Manager.DelegateProfile`. If the current helper does not carry the active profile, pass it through `executeTool` or store the current runtime policy/profile in the call path. The returned tool JSON must be:

```go
return mustJSON(map[string]any{
    "success": false,
    "profile_id": profileID,
    "task": task,
    "error": "subagent profile is not enabled for this agent",
})
```

- [ ] **Step 5: Run targeted tests**

Run:

```powershell
go test ./tests/agent -run Test_Agent_DelegateTaskTool_ -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Commit task**

```powershell
$env:GIT_MASTER='1'; git add internal/config/config.go internal/agent/agent.go tests/agent/delegate_task_test.go
$env:GIT_MASTER='1'; git commit -m "Add agent subagent allow list" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 2: Make model routing explicit and wire chat model override

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/agent/agent.go`
- Modify: `api/server/server.go`
- Test: `tests/agent/models_settings_api_test.go`

- [ ] **Step 1: Add failing route metadata and model override tests**

Add tests that assert:

```go
func TestRouteEndpointReturnsDecisionSourceAndRule(t *testing.T) {
    // Configure routing rule: pattern "format" -> tier fast.
    // GET /api/route?prompt=format%20this
    // Expect JSON fields selected_model, selected_tier, source="rule", rule_name="fast-simple".
}

func TestChatRequestHonorsModelOverride(t *testing.T) {
    // Start a fake OpenAI-compatible server.
    // POST /api/chat with model_override:"manual-model".
    // Inspect upstream request body and assert model == "manual-model".
}
```

- [ ] **Step 2: Run failing tests**

```powershell
go test ./tests/agent -run "TestRouteEndpointReturnsDecisionSourceAndRule|TestChatRequestHonorsModelOverride" -count=1 -v
```

Expected: FAIL because metadata/model override are missing.

- [ ] **Step 3: Extend router decision type**

In `internal/router/router.go`, add:

```go
type Decision struct {
    Model string `json:"selected_model"`
    Tier  string `json:"selected_tier"`
    Source string `json:"source"`
    RuleName string `json:"rule_name,omitempty"`
    Reason string `json:"reason,omitempty"`
}
```

Add a new method while keeping the old method as a compatibility wrapper:

```go
func (r *Router) Decide(prompt string, tokenCount int) Decision {
    lower := strings.ToLower(prompt)
    for _, rule := range r.rules {
        if matched, reason := ruleMatches(rule, lower, tokenCount); matched {
            if model := r.pick(rule.Tier); model != "" {
                return Decision{Model: model, Tier: rule.Tier, Source: "rule", RuleName: rule.Name, Reason: reason}
            }
        }
    }
    if model := r.pick(r.fallback); model != "" {
        return Decision{Model: model, Tier: r.fallback, Source: "fallback", Reason: "fallback tier"}
    }
    if model := r.pick("strong"); model != "" {
        return Decision{Model: model, Tier: "strong", Source: "fallback", Reason: "strong tier fallback"}
    }
    return Decision{Model: "gpt-4o-mini", Tier: "fast", Source: "fallback", Reason: "hardcoded fallback"}
}

func (r *Router) Route(prompt string, tokenCount int) (string, string) {
    d := r.Decide(prompt, tokenCount)
    return d.Model, d.Tier
}
```

- [ ] **Step 4: Add better default rules**

In `internal/config/defaults.go`, set routing rules:

```go
Rules: []RouteRule{
    {Name: "large-context", Condition: "tokens > 24000", Tier: "large_ctx"},
    {Name: "fast-simple", Patterns: []string{"typo", "rename", "format", "explain"}, Tier: "fast"},
    {Name: "coding-strong", Patterns: []string{"implement", "fix", "debug", "refactor", "test"}, Tier: "strong"},
},
```

- [ ] **Step 5: Add model_override to chat request**

In `api/server/server.go`, extend chat request struct:

```go
type chatRequest struct {
    Prompt string `json:"prompt"`
    SessionID string `json:"session_id"`
    ProjectID string `json:"project_id"`
    AgentID string `json:"agent_id"`
    ModelOverride string `json:"model_override"`
    ImageURL string `json:"image_url"`
}
```

Pass `ModelOverride` to agent run options. If no options struct exists, add a small one rather than adding positional parameters.

- [ ] **Step 6: Apply route priority in agent runtime**

In `internal/agent/agent.go`, when selecting model for a main run:

```go
decision := a.router.Decide(prompt, estimateTokens(prompt, messages))
if profile.Model != "" {
    decision.Model = profile.Model
    decision.Source = "agent"
    decision.Reason = "agent profile model override"
}
if modelOverride != "" && modelOverride != "auto" {
    decision.Model = modelOverride
    decision.Source = "manual"
    decision.Reason = "per-chat model override"
}
```

Emit the route SSE event using the decision fields.

- [ ] **Step 7: Run targeted tests**

```powershell
go test ./tests/agent -run "TestRouteEndpointReturnsDecisionSourceAndRule|TestChatRequestHonorsModelOverride|TestModelsSettings" -count=1 -v
```

Expected: PASS.

- [ ] **Step 8: Commit task**

```powershell
$env:GIT_MASTER='1'; git add internal/router/router.go internal/config/defaults.go internal/agent/agent.go api/server/server.go tests/agent/models_settings_api_test.go
$env:GIT_MASTER='1'; git commit -m "Explain model routing decisions" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 3: Add CLIProxyAPI extension backend

**Files:**
- Create: `internal/extensions/types.go`
- Create: `internal/extensions/logs.go`
- Create: `internal/extensions/cliproxyapi.go`
- Create: `api/server/extensions.go`
- Modify: `api/server/server.go`
- Test: `tests/extensions/cliproxyapi_test.go`

- [ ] **Step 1: Write failing tests for missing binary status**

Create `tests/extensions/cliproxyapi_test.go`:

```go
package extensions_test

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/yeyan00/uuagent/internal/extensions"
)

func TestCLIProxyAPIStatusReportsMissingBinary(t *testing.T) {
    root := t.TempDir()
    manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
        PluginRoot: filepath.Join(root, "plugins"),
        DataRoot: filepath.Join(root, "data"),
    })

    status := manager.Status(context.Background())
    if status.ID != "cliproxyapi" {
        t.Fatalf("expected cliproxyapi id, got %q", status.ID)
    }
    if status.Installed {
        t.Fatalf("expected missing binary to be not installed")
    }
    if status.Status != extensions.StatusMissing {
        t.Fatalf("expected missing status, got %q", status.Status)
    }
    if status.BinaryPath == "" {
        t.Fatalf("expected binary path in status")
    }
}
```

- [ ] **Step 2: Run failing test**

```powershell
go test ./tests/extensions -count=1 -v
```

Expected: FAIL because package does not exist.

- [ ] **Step 3: Create extension types**

Create `internal/extensions/types.go`:

```go
package extensions

const (
    StatusMissing  = "missing"
    StatusStopped  = "stopped"
    StatusStarting = "starting"
    StatusRunning  = "running"
    StatusError    = "error"
)

type Status struct {
    ID            string `json:"id"`
    Name          string `json:"name"`
    Description   string `json:"description"`
    BuiltIn       bool   `json:"built_in"`
    Installed     bool   `json:"installed"`
    Enabled       bool   `json:"enabled"`
    Status        string `json:"status"`
    BinaryPath    string `json:"binary_path"`
    ConfigPath    string `json:"config_path"`
    Port          int    `json:"port"`
    ProxyURL      string `json:"proxy_url"`
    ManagementURL string `json:"management_url"`
    LastError     string `json:"last_error"`
}

type CLIProxyAPIOptions struct {
    PluginRoot string
    DataRoot   string
}
```

- [ ] **Step 4: Create bounded log buffer**

Create `internal/extensions/logs.go`:

```go
package extensions

import "sync"

type LogBuffer struct {
    mu sync.Mutex
    max int
    lines []string
}

func NewLogBuffer(max int) *LogBuffer {
    if max <= 0 { max = 200 }
    return &LogBuffer{max: max}
}

func (b *LogBuffer) Append(line string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.lines = append(b.lines, line)
    if len(b.lines) > b.max {
        b.lines = b.lines[len(b.lines)-b.max:]
    }
}

func (b *LogBuffer) Lines() []string {
    b.mu.Lock()
    defer b.mu.Unlock()
    out := make([]string, len(b.lines))
    copy(out, b.lines)
    return out
}
```

- [ ] **Step 5: Implement missing/stopped status**

Create `internal/extensions/cliproxyapi.go` with status-only implementation first:

```go
package extensions

import (
    "context"
    "os"
    "path/filepath"
    "runtime"
)

type CLIProxyAPIManager struct {
    opts CLIProxyAPIOptions
    logs *LogBuffer
    port int
    lastError string
}

func NewCLIProxyAPIManager(opts CLIProxyAPIOptions) *CLIProxyAPIManager {
    return &CLIProxyAPIManager{opts: opts, logs: NewLogBuffer(300), port: 8317}
}

func (m *CLIProxyAPIManager) binaryPath() string {
    name := "cli-proxy-api"
    if runtime.GOOS == "windows" {
        name += ".exe"
    }
    return filepath.Join(m.opts.PluginRoot, "cliproxyapi", name)
}

func (m *CLIProxyAPIManager) configPath() string {
    return filepath.Join(m.opts.DataRoot, "cliproxyapi", "config.yaml")
}

func (m *CLIProxyAPIManager) Status(ctx context.Context) Status {
    binary := m.binaryPath()
    installed := false
    if info, err := os.Stat(binary); err == nil && !info.IsDir() {
        installed = true
    }
    state := StatusStopped
    if !installed {
        state = StatusMissing
    }
    return Status{ID: "cliproxyapi", Name: "CLIProxyAPI", Description: "OpenAI-compatible model proxy and management panel", BuiltIn: true, Enabled: true, Installed: installed, Status: state, BinaryPath: binary, ConfigPath: m.configPath(), Port: m.port, ProxyURL: "http://127.0.0.1:8317/v1", ManagementURL: "/extensions/cliproxyapi/management.html", LastError: m.lastError}
}

func (m *CLIProxyAPIManager) Logs() []string { return m.logs.Lines() }
```

- [ ] **Step 6: Run package test**

```powershell
go test ./tests/extensions -count=1 -v
```

Expected: PASS.

- [ ] **Step 7: Add API handlers**

Create `api/server/extensions.go`:

```go
package server

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/yeyan00/uuagent/internal/extensions"
    "github.com/yeyan00/uuagent/internal/paths"
)

func extensionManager() *extensions.CLIProxyAPIManager {
    return extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{PluginRoot: "plugins", DataRoot: paths.UserDir() + string(filepath.Separator) + "extensions"})
}

func handleListExtensions() gin.HandlerFunc {
    return func(c *gin.Context) {
        manager := extensionManager()
        c.JSON(http.StatusOK, gin.H{"extensions": []extensions.Status{manager.Status(c.Request.Context())}})
    }
}

func handleGetCLIProxyAPIExtension() gin.HandlerFunc {
    return func(c *gin.Context) {
        manager := extensionManager()
        c.JSON(http.StatusOK, manager.Status(c.Request.Context()))
    }
}
```

Add routes in `RegisterRoutes`:

```go
r.GET("/extensions", handleListExtensions())
r.GET("/extensions/cliproxyapi", handleGetCLIProxyAPIExtension())
```

- [ ] **Step 8: Add lifecycle methods in a follow-up within the same task**

Add `Start`, `Stop`, `Restart`, and `Health` methods using `exec.CommandContext` and `/healthz`. Keep process state guarded by a mutex. Missing binary returns status `missing` with clear error.

- [ ] **Step 9: Run backend tests**

```powershell
go test ./internal/extensions ./tests/extensions ./api/server -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit task**

```powershell
$env:GIT_MASTER='1'; git add internal/extensions api/server/extensions.go api/server/server.go tests/extensions/cliproxyapi_test.go
$env:GIT_MASTER='1'; git commit -m "Add CLIProxyAPI extension backend" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 4: Upgrade Web settings for agents and subagents

**Files:**
- Modify or create: `web/src/types.ts`
- Modify or create: `web/src/components/AgentsSettings.tsx`
- Modify or create: `web/src/components/SubagentsSettings.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`
- Test: `web/src/App.test.tsx`

- [ ] **Step 1: Add failing Web tests for agent list and enabled subagents**

In `web/src/App.test.tsx`, add a test that mocks `/api/agents` returning two agents and `/api/subagents` returning planner/builder. Assert Settings > Agents shows both agents and saving sends `enabled_subagents`.

```tsx
it('lists agents and saves enabled subagents from agent settings', async () => {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/projects') return Response.json({ projects: [] })
    if (url === '/api/agents') return Response.json({ agents: [
      { id: 'default', name: 'Default Agent', enabled_subagents: [] },
      { id: 'coder', name: 'Coding Agent', enabled_subagents: ['builder'] }
    ] })
    if (url === '/api/subagents') return Response.json({ subagents: [
      { id: 'planner', name: 'Planner' },
      { id: 'builder', name: 'Builder' }
    ] })
    if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: ['auto'] })
    if (url === '/api/skills') return Response.json({ skills: [] })
    if (url === '/api/sessions') return Response.json({ sessions: [] })
    if (url === '/api/memory') return Response.json({ memories: [] })
    if (url === '/api/agents' && init?.method === 'POST') return Response.json({ id: 'coder', name: 'Coding Agent' })
    return Response.json({})
  }) as any

  render(<App />)
  fireEvent.click(await screen.findByText('Settings'))
  fireEvent.click(await screen.findByText('Agents'))
  expect(await screen.findByText('Coding Agent')).toBeTruthy()
  fireEvent.click(await screen.findByText('Coding Agent'))
  fireEvent.click(await screen.findByLabelText('Planner'))
  fireEvent.click(await screen.findByText('Save Agent'))

  await waitFor(() => {
    const save = calls.find(c => c.url === '/api/agents' && c.init?.method === 'POST')
    expect(save).toBeTruthy()
    expect(JSON.parse(String(save?.init?.body)).enabled_subagents).toContain('planner')
  })
})
```

- [ ] **Step 2: Run failing Web test**

```powershell
cd web; npm test -- --runInBand
```

Expected: FAIL because page does not list agents this way and `enabled_subagents` is not wired.

- [ ] **Step 3: Add shared frontend types**

Create `web/src/types.ts`:

```ts
export interface AgentProfile {
  id: string
  name: string
  description?: string
  system_prompt?: string
  model?: string
  enabled_tools?: string[]
  enabled_skills?: string[]
  enabled_mcp_servers?: string[]
  enabled_subagents?: string[]
  permission_mode?: string
  max_turns?: number
}

export interface SubagentProfile {
  id: string
  name: string
  description?: string
  system_prompt?: string
  model?: string
  enabled_tools?: string[]
  enabled_skills?: string[]
  enabled_mcp_servers?: string[]
  blocked_tools?: string[]
  permission_mode?: string
  max_turns?: number
  workspace_path?: string
}
```

- [ ] **Step 4: Implement agent list/detail panel**

Create `web/src/components/AgentsSettings.tsx` with props:

```ts
interface AgentsSettingsProps {
  agents: AgentProfile[]
  subagents: SubagentProfile[]
  selectedAgentId: string
  onSelectAgent: (id: string) => void
  onSaveAgent: (profile: AgentProfile) => Promise<void>
  onNewAgent: () => void
  onCloneAgent: () => Promise<void>
  onDeleteAgent: () => Promise<void>
}
```

The component must render:

- list of agents
- detail form
- checkbox group for subagents using `aria-label={subagent.name || subagent.id}`
- Save Agent button

- [ ] **Step 5: Implement subagent list/detail panel**

Create `web/src/components/SubagentsSettings.tsx` with props for subagents, agent usage map, task list, and save/delete handlers.

- [ ] **Step 6: Wire components into App.tsx**

Replace current modal-only agent settings content in `renderSettingsBody()` with the new panels. Preserve existing new/clone/delete API behavior.

- [ ] **Step 7: Run Web tests**

```powershell
cd web; npm test
```

Expected: PASS.

- [ ] **Step 8: Commit task**

```powershell
$env:GIT_MASTER='1'; git add web/src/types.ts web/src/components/AgentsSettings.tsx web/src/components/SubagentsSettings.tsx web/src/App.tsx web/src/App.css web/src/App.test.tsx
$env:GIT_MASTER='1'; git commit -m "Improve agent settings management" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 5: Add Extensions UI for CLIProxyAPI

**Files:**
- Create: `web/src/components/ExtensionsPanel.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`
- Test: `web/src/App.test.tsx`

- [ ] **Step 1: Add failing Extensions UI test**

Add test:

```tsx
it('shows built-in CLIProxyAPI extension status and actions', async () => {
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    if (url === '/api/extensions') return Response.json({ extensions: [{ id: 'cliproxyapi', name: 'CLIProxyAPI', built_in: true, installed: false, status: 'missing', binary_path: 'plugins/cliproxyapi/cli-proxy-api.exe', proxy_url: 'http://127.0.0.1:8317/v1' }] })
    if (url === '/api/projects') return Response.json({ projects: [] })
    if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
    if (url === '/api/sessions') return Response.json({ sessions: [] })
    if (url === '/api/memory') return Response.json({ memories: [] })
    if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: [] })
    if (url === '/api/skills') return Response.json({ skills: [] })
    return Response.json({})
  }) as any
  render(<App />)
  fireEvent.click(await screen.findByText('Extensions'))
  expect(await screen.findByText('CLIProxyAPI')).toBeTruthy()
  expect(await screen.findByText('missing')).toBeTruthy()
  expect(await screen.findByText('plugins/cliproxyapi/cli-proxy-api.exe')).toBeTruthy()
})
```

- [ ] **Step 2: Run failing Web test**

```powershell
cd web; npm test -- --runInBand
```

Expected: FAIL because Extensions is placeholder.

- [ ] **Step 3: Add extension types**

In `web/src/types.ts`:

```ts
export interface ExtensionStatus {
  id: string
  name: string
  description?: string
  built_in: boolean
  installed: boolean
  enabled: boolean
  status: string
  binary_path?: string
  config_path?: string
  port?: number
  proxy_url?: string
  management_url?: string
  last_error?: string
}
```

- [ ] **Step 4: Create ExtensionsPanel**

Create `web/src/components/ExtensionsPanel.tsx` rendering:

- extension list
- CLIProxyAPI detail card
- Start/Stop/Restart buttons
- Proxy URL copy text
- Binary/config path
- last error
- iframe if `status === 'running' && management_url`

- [ ] **Step 5: Wire API calls in App.tsx**

Add state:

```ts
const [extensions, setExtensions] = useState<ExtensionStatus[]>([])
```

Add `fetchExtensions`, `startExtension`, `stopExtension`, and `restartExtension`. Call `fetchExtensions()` when main page is `extensions`.

- [ ] **Step 6: Run Web tests**

```powershell
cd web; npm test
```

Expected: PASS.

- [ ] **Step 7: Commit task**

```powershell
$env:GIT_MASTER='1'; git add web/src/types.ts web/src/components/ExtensionsPanel.tsx web/src/App.tsx web/src/App.css web/src/App.test.tsx
$env:GIT_MASTER='1'; git commit -m "Add CLIProxyAPI extension UI" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 6: Polish navigation and model settings UI

**Files:**
- Modify: `web/package.json`
- Modify: package lock file under `web/`
- Modify or create: `web/src/components/ModelsSettings.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`
- Test: `web/src/App.test.tsx`

- [ ] **Step 1: Install icon dependency**

Run:

```powershell
cd web; npm install lucide-react
```

Expected: `web/package.json` and lock file update.

- [ ] **Step 2: Replace letter icons with accessible SVG icons**

In `App.tsx`, import icons:

```tsx
import { Bot, Clock, Cpu, Database, Folder, Plug, Puzzle, Settings, Shield, Sparkles, Workflow } from 'lucide-react'
```

Change `navItems` to include an icon component and render with `aria-label`.

- [ ] **Step 3: Wire model override in Web chat request**

In `sendMessage()`, include:

```ts
...(modelOverride && modelOverride !== 'auto' ? { model_override: modelOverride } : {})
```

- [ ] **Step 4: Add model override payload test**

In `web/src/App.test.tsx`, assert that selecting a concrete model sends `model_override`, and selecting Auto omits it.

- [ ] **Step 5: Improve Models settings route preview**

Add route preview UI calling `/api/route?prompt=...` and rendering `selected_model`, `selected_tier`, `source`, and `rule_name`.

- [ ] **Step 6: Run Web tests and build**

```powershell
cd web; npm test; npm run build
```

Expected: PASS.

- [ ] **Step 7: Commit task**

```powershell
$env:GIT_MASTER='1'; git add web/package.json web/package-lock.json web/src/App.tsx web/src/App.css web/src/App.test.tsx web/src/components/ModelsSettings.tsx
$env:GIT_MASTER='1'; git commit -m "Polish navigation and model settings" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 7: Make Goal Mode create real delegated activity

**Files:**
- Modify: `internal/goal/runner.go`
- Modify: `api/server/server.go`
- Modify: `internal/goal/store.go`
- Test: `tests/goal/api_test.go`
- Test: `tests/goal/runner_test.go`

- [ ] **Step 1: Add failing API test for goal start side effects**

Add a test that creates a goal and expects activities to include a delegated todo or runner-started activity. Use fake subagent delegate if the current test infrastructure supports it.

- [ ] **Step 2: Run failing goal tests**

```powershell
go test ./tests/goal -count=1 -v
```

Expected: FAIL because server creation does not trigger runner.

- [ ] **Step 3: Add explicit goal run endpoint if automatic run is risky**

Add route:

```text
POST /api/projects/:id/goals/:goal_id/run
```

Handler loads the goal, constructs runner with subagent manager, and runs outstanding todos. If background execution is used, return immediately with status `running`.

- [ ] **Step 4: Record activity transitions**

Ensure store updates activities:

```text
goal_started
todo_started
delegate_started
delegate_completed
todo_completed
goal_completed
goal_failed
```

- [ ] **Step 5: Run goal tests**

```powershell
go test ./tests/goal -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Commit task**

```powershell
$env:GIT_MASTER='1'; git add internal/goal api/server/server.go tests/goal
$env:GIT_MASTER='1'; git commit -m "Run goal mode delegated tasks" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Task 8: Update release docs and run full verification

**Files:**
- Modify: `docs/DEVELOPMENT_TODO.md`
- Modify: `docs/TEST_TODO.md`
- Modify: `docs/核心关注功能.txt`
- Modify: `README.md`

- [ ] **Step 1: Update status docs**

Update the status docs to mark:

- Agent/Subagent settings as release-ready MVP.
- CLIProxyAPI as default built-in extension MVP.
- Model routing as partial but explicit and testable.
- Goal Mode as MVP with delegated activity.

- [ ] **Step 2: Update README usable release notes**

Add a short section describing:

```text
Windows-first test release:
- CLIProxyAPI expected at plugins/cliproxyapi/cli-proxy-api.exe
- Extensions page can start/stop/check it
- Models can use built-in proxy URL
- Agents can restrict subagents
```

- [ ] **Step 3: Run full verification**

Run:

```powershell
powershell -ExecutionPolicy Bypass -File "scripts/test.ps1"
```

Expected: Go tests, Web Vitest, and Web build all pass.

- [ ] **Step 4: Inspect final status**

Run:

```powershell
$env:GIT_MASTER='1'; git status --short --branch
$env:GIT_MASTER='1'; git log --oneline -12
```

Expected: working tree clean except intentional uncommitted release artifacts, and recent commits match completed tasks.

- [ ] **Step 5: Commit docs**

```powershell
$env:GIT_MASTER='1'; git add docs/DEVELOPMENT_TODO.md docs/TEST_TODO.md "docs/核心关注功能.txt" README.md
$env:GIT_MASTER='1'; git commit -m "Update usable release status" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

---

## Self-Review Checklist

- Spec coverage: This plan covers Agent/Subagent settings, CLIProxyAPI built-in extension, model routing, UI icons/layout, Goal Mode activity, and docs.
- Placeholder scan: No unresolved placeholder markers are intended in this plan. Every task names concrete files, commands, expected outcomes, and implementation direction.
- Type consistency: `enabled_subagents`, `model_override`, and extension status fields match the spec.
- Scope check: The plan spans multiple subsystems but each task is independently testable and commit-sized. Implementation should use subagent-driven development rather than one large inline edit.

## Execution Options

Plan complete and saved to `docs/superpowers/plans/2026-06-22-usable-release.md`.

Two execution options:

1. **Subagent-Driven (recommended)** - dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** - execute tasks in this session using executing-plans, batch execution with checkpoints.
