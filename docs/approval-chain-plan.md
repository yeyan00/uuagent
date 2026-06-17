# Approval Chain Development and Test Plan

## Goal
Complete the Web approval flow so approving a protected tool call continues the full agent loop and the Web UI displays intermediate tool activity instead of silently jumping to the final assistant text.

## Current State
- Initial `/api/chat` streams SSE events: `run`, `content`, `tool_start`, `tool_result`, `status`, `done`.
- `approval_required` tool results pause the run and render an approval card.
- `POST /api/runs/:id/approve` currently continues the agent loop and returns a JSON final `content`.
- Web approval click appends only the returned final assistant content.
- Normal `tool_start` / `tool_result` events are only reflected in the status strip, not the chat transcript.

## Target Behavior
1. Initial chat stream shows tool activity as chat events:
   - `tool_start`: compact tool card such as `Running list_dir`.
   - normal `tool_result`: compact result card with tool name and short preview.
   - `approval_required`: approval card with Approve/Deny actions.
2. Clicking Approve starts an eventful continuation:
   - The approved tool executes once with `approved:true`.
   - Any follow-up model content streams into the chat.
   - Any follow-up tool calls/results display as tool cards.
   - If another approval is needed, a new approval card appears.
   - If no more tools are needed, final assistant answer appears and run status becomes `Done` / `Approved` as appropriate.
3. Clicking Deny marks the card denied and does not continue execution.

## Implementation Tasks

### Task 1: Backend approval streaming endpoint
Files:
- Modify `internal/agent/agent.go`
- Modify `api/server/server.go`
- Add/modify tests in `tests/agent/tool_loop_test.go` and `tests/session/api_test.go`

Steps:
- Add an eventful approval resume method, e.g. `ApproveRunEvents(ctx, runID) (<-chan Event, error)`.
- Reuse the same continuation logic as `ApproveRun` but emit:
  - `status` before/after approved tool execution.
  - `tool_start` and `tool_result` for approved and follow-up tools.
  - `content` for assistant text.
  - `done` when the loop completes.
  - `status: approval required` and a `tool_result` approval payload if a second approval is encountered.
- Add route `GET /api/runs/:id/approve/stream` or `POST /api/runs/:id/approve/stream`.
- Prefer `GET` for EventSource-like browser consumption, but current code uses `fetch` streams, so either is acceptable. Use `POST` if preserving mutation semantics is preferred.
- Keep existing JSON `POST /api/runs/:id/approve` for compatibility.

Tests:
- Backend test must simulate:
  1. Initial protected tool call pauses.
  2. Approval stream emits approved tool start/result.
  3. Model asks for a follow-up normal tool.
  4. Approval stream emits follow-up tool start/result.
  5. Final assistant content and `done` are emitted.

### Task 2: Web tool event model
Files:
- Modify `web/src/App.tsx`
- Modify `web/src/App.css`
- Modify `web/src/App.test.tsx`

Steps:
- Extend frontend message model to include tool event system messages encoded as JSON:
  - `{ "kind":"tool_start", "tool":"list_dir", "tool_id":"..." }`
  - `{ "kind":"tool_result", "tool":"list_dir", "tool_id":"...", "text":"..." }`
  - Existing approval payload remains `{ "approval_required": true, ... }`.
- Add render helpers:
  - `parseToolEventPayload()`.
  - `renderToolEventCard()`.
- Render normal tool activity as compact cards, not raw text.
- Keep approval card rendering separate.

Tests:
- Simulate `/api/chat` stream with `tool_start`, normal `tool_result`, and final content.
- Assert the UI shows `Running list_dir`, a result preview, and final assistant content.

### Task 3: Web approve stream handling
Files:
- Modify `web/src/App.tsx`
- Modify `web/src/App.test.tsx`

Steps:
- Change Approve button handler from JSON fetch to stream fetch.
- Process the approve stream with the same event handling function used by `/api/chat`.
- Mark the clicked approval card as approved immediately after successful stream start or first event.
- Append any continuation content/tool cards to the current chat transcript.
- Preserve the current run status through `Approval required`, `Approved`, `Running tool`, and `Done`.

Tests:
- Existing approval test should mock approve stream with:
  - `tool_start` for approved tool.
  - `tool_result` for approved tool.
  - `content` final answer.
  - `done`.
- Assert Approve calls the streaming endpoint, buttons disable, tool cards appear, and final content appears.

### Task 4: Verification
Commands:
- `go test ./tests/agent -run 'ApproveRun|Approval|ToolLoop' -count=1`
- `go test ./tests/session -run Approval -count=1` if session/API approval tests are added.
- `npm test -- --run`
- `powershell -ExecutionPolicy Bypass -File "scripts/test.ps1"`

Expected:
- Go approval/tool-loop tests pass.
- Vitest passes.
- Frontend TypeScript build passes.
- Full project script passes except existing npm audit vulnerability report remains unchanged.
