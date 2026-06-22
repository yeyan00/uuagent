# CLIProxyAPI Usability and Top-Level Chat Design

## Goal

Make the Windows-first test release feel usable without hidden setup traps: Extensions should clearly manage CLIProxyAPI as a built-in sidecar service, and Chat should be a first-class top-level navigation item rather than being buried behind Projects/workspace settings.

## Current State

- `internal/extensions/cliproxyapi.go` expects the binary at `plugins/cliproxyapi/cli-proxy-api.exe` on Windows and `plugins/cliproxyapi/cli-proxy-api` elsewhere.
- The backend already supports status/start/stop/restart/logs for CLIProxyAPI.
- The Web Extensions panel already has Start, Stop, and Restart controls, but when the binary is absent it only shows `Missing` guidance.
- No `plugins/cliproxyapi` binary exists in this repository, so a fresh checkout always shows CLIProxyAPI as missing.
- `web/src/App.tsx` currently defines top-level navigation as Projects, Extensions, Schedules, and Settings. Chat is a workspace mode inside Projects, making Settings/Chat switching less direct than the product needs.

## Design Summary

### Recommended Approach

Use a repository-managed plugin path with a clear runtime contract:

1. Add `plugins/cliproxyapi/README.md` and `.gitkeep` so the expected binary directory exists in source control.
2. Keep the binary name/path stable: `plugins/cliproxyapi/cli-proxy-api.exe` on Windows.
3. If a real binary is available during release assembly, place it at that exact path. Do not generate or fake an executable in code.
4. Improve the missing state so it is actionable: show the exact path, state that the binary must be copied there for the test build, and keep Start disabled while missing.
5. Keep backend process management as the source of truth for service status.
6. Promote Chat to top-level navigation next to Projects.

This preserves honest behavior: if the executable is absent, we do not pretend the service is installed; if the executable is present, the existing start/stop lifecycle works and the UI becomes useful.

## Alternatives Considered

### Option A: Commit the real CLIProxyAPI binary directly

This gives the best out-of-box test experience. It is acceptable for an internal Windows test release if the binary is available and license/size are acceptable. The implementation should support this by creating the plugin folder and tests, but the binary itself can only be committed when provided.

### Option B: Auto-download CLIProxyAPI from a release URL

This avoids large binaries in git, but it requires a trusted release source, checksum verification, network permissions, and failure UX. That is too much for the immediate usability fix.

### Option C: Show only better missing guidance

This is easy but does not address the user complaint that Extensions should be usable. It is not sufficient by itself.

## Backend Design

The backend remains centered on `CLIProxyAPIManager`.

Required changes:

- Make the expected plugin directory explicit in repository docs and tests.
- Ensure `Status` returns:
  - `missing` when the binary does not exist.
  - `stopped` when the binary exists and no process is running.
  - `running` when a process is active and health succeeds.
  - `error` when start/stop/health fails.
- Keep `Start` blocked when missing. Do not create a fake process.
- Keep generated config under the extension data root, not inside the plugin directory.
- Add tests that create a fake executable at the plugin path and verify status changes from `missing` to `stopped`.

## Frontend Design

### Extensions UX

The Extensions panel should read as a service manager:

- Header: CLIProxyAPI, Built-in badge, current status.
- Primary action:
  - Missing: disabled Start button plus install/copy guidance.
  - Stopped/Error: Start button.
  - Running/Starting: Stop button.
  - Running: Restart button.
- Detail rows: Binary Path, Config Path, Port, Proxy URL, Management URL when running, Last Error.
- Logs should remain visible or easy to refresh.

### Top-Level Navigation

Add Chat as a top-level item:

```ts
type MainPage = 'projects' | 'chat' | 'extensions' | 'schedules' | 'settings'
```

Navigation order:

1. Projects
2. Chat
3. Extensions
4. Schedules
5. Settings

Chat page behavior:

- Shows the active project/session chat workspace.
- If no project is selected, show an empty state telling the user to choose or create a project from Projects.
- Settings remains a separate top-level page.
- Project settings tabs remain inside Projects or project workspace settings; they should not hide Chat.

## Data Flow

CLIProxyAPI:

1. Web loads `/api/extensions`.
2. Backend checks the plugin binary path and process state.
3. User clicks Start.
4. Web posts `/api/extensions/cliproxyapi/start`.
5. Backend writes config, starts the process, waits for `/healthz`, and returns updated status.
6. Web refreshes status and displays Proxy URL/logs.

Chat navigation:

1. User selects project/session from Projects.
2. Selection state remains global in `App.tsx`.
3. User clicks Chat top-level nav.
4. Chat renders the selected workspace chat.

## Error Handling

- Missing binary: status `missing`, no process start attempt, exact path shown.
- Start failure: status `error`, last error and logs shown.
- Stop failure: status `error`, last error shown.
- Chat without selected project: friendly empty state, no API call.

## Testing

Backend:

- CLIProxyAPI status is `missing` when no binary exists.
- CLIProxyAPI status becomes `stopped` when a fake binary exists.
- Start/stop lifecycle tests continue to pass.

Frontend:

- Extensions panel shows missing guidance with exact binary path.
- Extensions panel enables Start when status is stopped and disables it when missing.
- Chat appears as a top-level nav item.
- Clicking Chat renders chat workspace or no-project empty state.
- Settings remains independent from Chat.

## Non-Goals

- Do not implement automatic download in this iteration.
- Do not fake CLIProxyAPI if the executable is missing.
- Do not implement real MCP as part of this work.
- Do not redesign the entire Web shell.

## Implementation Gate

The repository can implement all UX/backend support without the real binary. To make Extensions stop showing `missing` in a working copy, a real `cli-proxy-api.exe` must be placed at `plugins/cliproxyapi/cli-proxy-api.exe` before running UUAgent on Windows.
