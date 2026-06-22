# UUAgent Usable Release Design

Date: 2026-06-22
Status: Approved direction - Windows-first usable test release

## 1. Goal

Ship a practical UUAgent build that is usable by external testers without manual wiring. The release should make agent configuration understandable, expose subagent relationships, improve Web UI credibility, include CLIProxyAPI as a default plugin, and make model routing behavior explicit and reliable.

Primary target platform for the first release is Windows. Linux and macOS should remain possible through the same architecture, but they are not the first validation target.

## 2. Current Problems

### 2.1 Agent settings are incomplete as a product surface

The backend already supports agent profile CRUD through `/api/agents`, but the Web settings surface behaves like an editor for the currently selected agent. Most users see only the default agent and do not understand that more profiles can exist.

### 2.2 Agents and subagents are not connected in the UI

Subagents exist globally under `agent.subagent.profiles`. Runtime delegation uses `delegate_task(profile_id, task)`, but the Web UI does not show which subagents an agent can use. This makes Goal Mode and delegated execution look disconnected.

### 2.3 The Web UI looks like an engineering prototype

The current app is functional but visually rough: text-letter navigation icons, simple placeholder pages, weak visual hierarchy, and a very large `App.tsx` that makes UI changes risky.

### 2.4 CLIProxyAPI is not actually embedded

The default config points to `http://localhost:18463/v1`, and `cmd/uuagent/main.go` has a TODO for CLIProxyAPI routes, but UUAgent does not currently start or proxy CLIProxyAPI. The separate CLIProxyAPI repo has a runnable server model and management UI on port `8317`.

### 2.5 Model routing is misleading

The router exists, but default behavior mostly falls back to `strong`; token-based routing is unused because calls pass token count `0`; the Web model dropdown is not sent to `/api/chat`; and route metadata does not explain whether the selection came from manual override, agent profile, subagent profile, rule, or fallback.

### 2.6 Goal Mode needs to feel real

Goal Mode currently has persistence, plan/todo/activity UI, and `delegate_task` support, but for a tester it must show real delegated execution records rather than only stored/displayed plans.

## 3. Release Scope

### P0 - Required for the usable test release

1. Agent settings page lists and edits all agents.
2. Agent profiles can restrict available subagents.
3. Subagent settings page lists and edits all subagents.
4. Web navigation uses real SVG icons and improved layout hierarchy.
5. Extensions page becomes real and includes a default CLIProxyAPI plugin.
6. CLIProxyAPI plugin can detect, start, stop, health-check, and report errors for its sidecar executable.
7. Model settings can use the built-in CLIProxyAPI proxy URL.
8. Chat model override is wired or removed; preferred behavior is to wire it.
9. Model routing explains selection source and uses better default rules.
10. Documentation and release status files are updated.

### P1 - Strongly recommended for the same release if time allows

1. Reverse-proxy CLIProxyAPI management UI into UUAgent.
2. Show CLIProxyAPI logs in Extensions.
3. Trigger Goal Mode runner so goals create real subagent task/activity records.
4. Split the largest Web settings sections into focused React components to reduce `App.tsx` risk.

### P2 - Later release

1. Plugin marketplace.
2. Real MCP stdio/http/sse client beyond mock MCP.
3. Cross-platform CLIProxyAPI binary discovery and packaging.
4. Provider key management UI.
5. Full nested subagent token accounting.

## 4. Architecture Overview

The release keeps UUAgent and CLIProxyAPI as separate executables. UUAgent owns the product shell, project/session/agent/subagent UX, and plugin lifecycle. CLIProxyAPI remains an external OpenAI-compatible model proxy sidecar.

```text
UUAgent
  api/server
    /api/agents
    /api/subagents
    /api/extensions
    /api/models/settings
    /api/chat
  internal/extensions
    registry
    cliproxyapi sidecar manager
    process lifecycle
  web
    Settings pages
    Extensions pages
    Chat / Goal workspace

CLIProxyAPI sidecar
  127.0.0.1:<port>/v1
  127.0.0.1:<port>/healthz
  127.0.0.1:<port>/management.html
  127.0.0.1:<port>/v0/management/*
```

The first Windows release bundles or expects this file:

```text
plugins/cliproxyapi/cli-proxy-api.exe
```

If the executable is missing, the plugin still appears in Extensions with an actionable missing-binary error and the expected path.

## 5. Data Model Changes

### 5.1 AgentProfile

Add optional subagent allow-list:

```go
type AgentProfile struct {
    EnabledSubagents []string `yaml:"enabled_subagents" json:"enabled_subagents"`
}
```

Semantics:

- Empty list means all subagents are allowed, preserving backward compatibility.
- Non-empty list means only listed subagent profile IDs are available to that agent.
- `delegate_task` must reject a `profile_id` not allowed by the active agent profile.

### 5.2 Extension status model

Expose a stable status shape for built-in extensions:

```json
{
  "id": "cliproxyapi",
  "name": "CLIProxyAPI",
  "description": "OpenAI-compatible model proxy and management panel",
  "built_in": true,
  "installed": true,
  "enabled": true,
  "status": "running",
  "binary_path": ".../plugins/cliproxyapi/cli-proxy-api.exe",
  "config_path": ".../.uuagent/extensions/cliproxyapi/config.yaml",
  "port": 8317,
  "proxy_url": "http://127.0.0.1:8317/v1",
  "management_url": "/extensions/cliproxyapi/management.html",
  "last_error": ""
}
```

Status values:

- `missing`
- `stopped`
- `starting`
- `running`
- `error`

### 5.3 Route decision metadata

Route events and route preview should expose:

```json
{
  "selected_model": "claude-sonnet-4",
  "selected_tier": "strong",
  "source": "manual|agent|subagent|rule|fallback|fixed",
  "rule_name": "coding-strong",
  "reason": "matched pattern: implement"
}
```

## 6. Backend Design

### 6.1 Agent/subagent APIs

Reuse existing endpoints:

- `GET /api/agents`
- `POST /api/agents`
- `PATCH /api/agents/:id`
- `DELETE /api/agents/:id`
- `GET /api/subagents`
- `POST /api/subagents`
- `PATCH /api/subagents/:id`
- `DELETE /api/subagents/:id`
- `GET /api/subagent/tasks`

Required backend additions:

- Persist `enabled_subagents` on agent profiles.
- Enforce allowed subagents in `delegate_task` based on the active parent agent profile.
- Preserve current default behavior where an empty allow-list means all subagents.

### 6.2 Extensions API

Add endpoints:

```text
GET  /api/extensions
GET  /api/extensions/cliproxyapi
POST /api/extensions/cliproxyapi/start
POST /api/extensions/cliproxyapi/stop
POST /api/extensions/cliproxyapi/restart
GET  /api/extensions/cliproxyapi/logs
```

The extension manager should be small and explicit, not a marketplace abstraction. This release has one built-in extension: CLIProxyAPI.

### 6.3 CLIProxyAPI sidecar lifecycle

The sidecar manager should:

1. Resolve binary path.
2. Generate a local config if missing.
3. Bind to `127.0.0.1`.
4. Prefer port `8317`, but choose a free port if occupied.
5. Generate a local management secret.
6. Start the child process.
7. Poll `/healthz` until ready or timed out.
8. Capture stdout/stderr to a bounded log buffer and log file.
9. Stop gracefully when requested.
10. Mark errors clearly if the binary is missing or health check fails.

Generated config should use UUAgent-managed extension storage, for example:

```text
~/.uuagent/extensions/cliproxyapi/config.yaml
~/.uuagent/extensions/cliproxyapi/auth
~/.uuagent/extensions/cliproxyapi/logs
```

### 6.4 CLIProxyAPI Web embedding

Preferred P1 implementation is reverse proxy:

```text
/extensions/cliproxyapi/management.html -> http://127.0.0.1:<port>/management.html
/extensions/cliproxyapi/v0/management/* -> http://127.0.0.1:<port>/v0/management/*
```

If path assumptions in the management HTML make reverse proxy risky, P0 can ship an iframe that points directly to `http://127.0.0.1:<port>/management.html`, with a clear fallback button to open in browser.

### 6.5 Model routing

Fix route priority:

```text
manual per-chat model override
> agent profile model
> subagent profile model
> route rule
> fallback tier
> hardcoded fallback
```

Add `/api/chat` request field:

```json
{
  "model_override": "auto|model-id"
}
```

If omitted or `auto`, routing is automatic. If set to a concrete model, it is used and route source is `manual`.

Default routing rules should be added to defaults:

```yaml
rules:
  - name: large-context
    condition: "tokens > 24000"
    tier: large_ctx
  - name: fast-simple
    patterns: ["typo", "rename", "format", "explain"]
    tier: fast
  - name: coding-strong
    patterns: ["implement", "fix", "debug", "refactor", "test"]
    tier: strong
```

Runtime should pass a real token estimate when available. If exact tokenizer support is not available for P0, use a conservative character-based estimate and document it as approximate.

## 7. Frontend Design

### 7.1 Visual direction

Use a clean desktop-app style inspired by Linear/Raycast:

- Clear icon rail.
- Left list/sidebar for collections.
- Main detail panel for editing.
- Cards for status and diagnostics.
- Consistent badges for status.
- SVG icons, not emoji or letters.

Use `lucide-react` for icons.

### 7.2 Navigation

Replace letter icons with icons:

- Projects: `Folder`
- Extensions: `Puzzle`
- Schedules: `Clock`
- Settings: `Settings`
- Agents: `Bot`
- Subagents: `Network` or `Workflow`
- Models: `Cpu`
- Skills: `Sparkles`
- MCP: `Plug`
- Permissions: `Shield`
- Storage: `Database`
- CLIProxyAPI: `ServerCog` or `Cable`

### 7.3 Settings - Agents

Settings Agents page layout:

```text
Agents
  left: profile list + New Agent
  right: detail editor
```

Detail sections:

1. Identity
   - ID
   - Name
   - Description
2. Behavior
   - System Prompt
   - Max Turns
   - Permission Mode
3. Model
   - Auto or fixed model
4. Capabilities
   - Enabled Tools
   - Enabled Skills
   - Enabled MCP Servers
   - Enabled Subagents
5. Actions
   - Save
   - Clone
   - Delete if not default

### 7.4 Settings - Subagents

Settings Subagents page layout:

```text
Subagents
  left: subagent list + New Subagent
  right: detail editor + usage/task cards
```

Detail sections:

1. Identity
2. System Prompt
3. Model / Permission / Max Turns
4. Blocked Tools
5. Enabled Tools / Skills / MCP
6. Workspace Path
7. Used by Agents
8. Recent Tasks

### 7.5 Extensions - CLIProxyAPI

Extensions page layout:

```text
Extensions
  left: extension list
  right: extension detail
```

CLIProxyAPI detail:

- Status card:
  - Installed/missing
  - Running/stopped/error
  - Port
  - Proxy URL
  - Health
- Actions:
  - Start
  - Stop
  - Restart
  - Open Management
  - Copy Proxy URL
- Embedded management panel or browser-open fallback.
- Logs and paths.

### 7.6 Models page

Models page should make routing understandable:

1. Proxy source:
   - Built-in CLIProxyAPI
   - External proxy URL
2. Connection test:
   - `/models` result
   - last tested time
3. Model tiers:
   - fast
   - strong
   - large_ctx
4. Routing preview:
   - prompt input
   - estimated tokens
   - selected model
   - source/rule
5. Manual model behavior:
   - Chat composer model dropdown sends `model_override`.
   - `Auto` means no override.

## 8. Goal Mode Design

For the usable release, Goal Mode must create visible real execution evidence.

Minimum behavior:

1. User creates goal.
2. Backend creates plan/todos.
3. Runner starts or can be manually started.
4. Each todo delegates to the configured subagent profile.
5. Activities record start, delegation, completion, failure, and stop.
6. Goal page can refresh and show current state.

If continuous background execution is too risky for P0, ship manual run/refresh but ensure at least one real subagent delegation happens for started goals.

## 9. Testing Strategy

### Backend tests

- Agent profile persists `enabled_subagents`.
- `delegate_task` rejects disallowed subagents.
- `delegate_task` allows all subagents when allow-list is empty.
- Extension API returns CLIProxyAPI status when binary is missing.
- Extension start handles missing binary with a clear error.
- Extension sidecar config generation uses `127.0.0.1` and local paths.
- Model settings can select built-in CLIProxyAPI proxy URL.
- Chat request honors `model_override`.
- Route metadata reports source/rule correctly.

### Web tests

- Agents settings lists all agents.
- Agent detail edits enabled subagents.
- Subagents page lists default profiles and recent tasks.
- Extensions page shows CLIProxyAPI built-in plugin.
- Missing CLIProxyAPI binary shows actionable error.
- Running CLIProxyAPI status shows proxy URL and actions.
- Model dropdown sends `model_override` for concrete model and omits it for Auto.
- Model settings route preview displays selected source/rule.
- Navigation uses icon buttons with accessible labels.

### Manual release QA

On Windows:

1. Start UUAgent.
2. Open Extensions.
3. Verify CLIProxyAPI appears by default.
4. Start CLIProxyAPI or observe missing-binary guidance.
5. Verify Models can use built-in proxy URL.
6. Create/edit agent.
7. Restrict agent to specific subagents.
8. Start chat with Auto model.
9. Start chat with manual model override.
10. Create a goal and verify visible subagent activity.

## 10. Implementation Order

1. Add `enabled_subagents` schema and enforcement.
2. Rework Agents/Subagents settings pages.
3. Fix chat model override and route metadata.
4. Add better default routing rules and token estimate.
5. Add Extensions backend API and CLIProxyAPI sidecar status model.
6. Add Extensions Web page and CLIProxyAPI status/actions.
7. Add CLIProxyAPI start/stop/health/log behavior.
8. Add management panel embedding or open-in-browser fallback.
9. Polish visual navigation and layout with SVG icons.
10. Wire Goal Mode runner enough to show real delegated activity.
11. Update release docs and run full verification.

## 11. Non-Goals

- Do not merge CLIProxyAPI source into UUAgent for this release.
- Do not build a plugin marketplace.
- Do not store provider API keys in UUAgent config unless a later key-management design is approved.
- Do not implement full cross-platform packaging before Windows is validated.
- Do not rewrite the entire Web app architecture before the usable release.

## 12. Risks and Mitigations

### Risk: CLIProxyAPI management UI may not work behind a path prefix

Mitigation: P0 can use iframe/direct open fallback. P1 can add path-rewriting reverse proxy after validation.

### Risk: Bundled executable missing or wrong platform

Mitigation: Extensions page must show exact expected path and status. Release checklist must include Windows `cli-proxy-api.exe` packaging.

### Risk: App.tsx is already large

Mitigation: Split only the new settings/extensions panels into focused components where practical. Avoid broad unrelated refactors.

### Risk: Model routing still feels arbitrary

Mitigation: Always show route source, rule name, selected model, and whether manual/agent/subagent override applied.

### Risk: Goal Mode overpromises autonomy

Mitigation: Label the first release as Goal Mode MVP, but ensure started goals create real subagent activity records.
