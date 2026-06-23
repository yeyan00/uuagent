# UUAgent Chat Workspace UX Upgrade Design

Date: 2026-06-23
Status: Proposed
Scope: Phase 1 medium-range UI upgrade, inspired by `C:\\00_work\\greenvalley\\code\\llm\\nowork\\web`

## Goal

Improve the UUAgent Chat workspace so active project sessions feel like a real browser-like working set, while keeping Projects as the project/session library and avoiding a risky full chat-state rewrite.

This phase adopts the most useful `nowork/web` ideas:

- visible/overflow session tabs
- active/recent tab ordering
- per-tab running indicators
- tab context menu for common session actions
- resizable left context sidebar
- lighter, more readable Chat tab styling

This phase does **not** extract a full `ChatWorkspace` component or introduce a complete per-session message/draft cache. Those are explicitly deferred to a later architecture phase.

## Non-Goals

- Do not remove the top-level Chat page.
- Do not merge Projects and Chat into one page.
- Do not redesign Settings, Extensions, or Schedules beyond avoiding layout regressions.
- Do not replace UUAgent's current soft blue desktop design system.
- Do not port `nowork/web` wholesale.
- Do not implement full per-session cached messages/drafts in this phase.
- Do not change backend session APIs unless a frontend action already requires an existing endpoint.

## Current State

UUAgent currently renders a three-column shell in `web/src/App.tsx` and `web/src/App.css`:

```text
appWindow
├─ navRail          92px
├─ contextSidebar   320px fixed
└─ workspace        main
```

Chat tabs currently use:

- `openChatTabs: ChatTab[]`
- `upsertChatTab(projectId, sessionId, title)`
- `selectChatTab(tab)`
- `closeChatTab(tabId)`
- `.chatTabStrip`, `.chatTab`, `.chatTabButton`, `.chatTabClose`

Existing behavior is correct but basic:

- opening a project session switches to Chat and creates/activates a tab
- creating a new project session opens a tab
- multiple sessions can be switched
- tabs can be closed without deleting sessions

Weaknesses:

- tabs are only horizontally scrollable; there is no visible/overflow policy
- no running indicator per tab
- tab actions are limited to select and close
- session row actions remain disconnected from tab affordances
- context sidebar is fixed width
- `App.tsx` is large and tightly coupled, so this phase should avoid broad extraction

## Reference Pattern from nowork/web

The reference app keeps global navigation separate from Chat's internal workspace. In Chat, it maintains cached worker/session state and renders session tabs with:

- active session pinned first
- recent sessions next
- top 5 visible tabs
- remaining sessions in overflow dropdown
- running dot on streaming sessions
- double-click rename
- right-click context menu
- actions such as rename, clone, compact, export

This design borrows the tab management and sidebar ergonomics, but adapts them to UUAgent's project/session model.

## Proposed UX

### 1. Chat Tabs: Visible + Overflow Working Set

Replace the unbounded horizontal-only tab strip with a visible/overflow model:

- Active tab is always first.
- Other tabs are ordered by `lastActiveAt` descending.
- Up to 5 tabs are shown directly.
- Remaining open tabs appear under a `More` dropdown.
- The dropdown shows project name, session title, and running status.
- Selecting an overflow item activates it and moves it into the visible set.

Tab state should extend the current `ChatTab` shape:

```ts
interface ChatTab {
  id: string;
  projectId: string;
  sessionId: string;
  projectName: string;
  title: string;
  lastActiveAt: number;
  isStreaming?: boolean;
}
```

`lastActiveAt` updates when:

- a session is opened from Projects
- a new session is created
- a tab is selected
- a streaming run starts in that session

`isStreaming` updates based on the active run/session. This phase only needs accurate indicators for sessions opened in the current frontend working set.

### 2. Tab Visual Styling

Tabs should become lighter and closer to the `nowork` density:

- compact height
- pill/card hybrid shape
- clear active state
- project name as muted metadata
- session title as primary text
- close action visible on hover/focus or as a subtle icon
- running dot shown next to title or on the right edge

The tab strip remains Chat-only and must never appear on Projects, Extensions, Schedules, or Settings.

### 3. Tab Context Menu

Add right-click or overflow action menu for each tab. Initial actions:

- Rename
- Fork
- Compact
- Export Context
- Close Tab

Action semantics:

- Rename updates the backend session title and updates the tab title.
- Fork uses the existing fork behavior where available and opens the forked session as an active Chat tab.
- Compact uses existing compact behavior and should be disabled when the tab's session is streaming.
- Export Context uses existing context/session data if already available; otherwise it may fetch and download/export through the existing frontend behavior if present.
- Close Tab removes the tab from the working set only; it does not delete the session.

If an action lacks an existing backend/frontend helper, the implementation plan must either wire the existing endpoint or defer that action rather than introducing a large backend change.

### 4. Resizable Context Sidebar

Make the left context sidebar width adjustable in the desktop shell:

- default width: 320px
- min width: 240px
- max width: 480px
- drag handle between sidebar and workspace
- preserve the nav rail width
- keep non-Chat pages usable
- store width in component state for this phase

Persistence to localStorage can be added if simple, but is not required for Phase 1.

### 5. Preserve Projects vs Chat Separation

Projects remains the project/session library:

- creating projects
- browsing project sessions
- opening sessions into Chat

Chat remains the active conversation workspace:

- active/open tabs
- messages
- composer
- model selector
- route/token status
- compact/run controls

Non-Chat pages must not show Chat-only chrome:

- composer
- Session Tokens
- Compact button
- route/model pill
- Chat tab strip
- goal controls

## Component and State Design

This phase should avoid a full component extraction, but can introduce small helpers to keep `App.tsx` from growing further.

Recommended additions:

```ts
function getVisibleChatTabs(tabs: ChatTab[], activeTabId: string): {
  visibleTabs: ChatTab[];
  overflowTabs: ChatTab[];
}
```

Rules:

1. active tab first if present
2. remaining tabs sorted by `lastActiveAt` descending
3. visible count capped at 5
4. overflow contains remaining tabs

Optional small component extraction:

```tsx
<ChatTabStrip
  tabs={openChatTabs}
  activeTabId={activeChatTabId}
  onSelect={selectChatTab}
  onClose={closeChatTab}
  onRename={renameSessionFromTab}
  onFork={forkSessionFromTab}
  onCompact={compactSessionFromTab}
  onExport={exportSessionFromTab}
/>
```

This extraction is allowed if it keeps props narrow and does not force a larger chat rewrite.

## Error Handling

- If selecting a tab fails to load the session, keep the current session visible and show status/error text.
- If rename fails, restore the previous title.
- If fork fails, do not create a tab.
- If compact fails, show the existing error/status path.
- If export fails, show a non-blocking error/status.
- If closing the active tab, activate the next visible/recent tab; if no tabs remain, clear active session state and keep the user on Chat empty state.

## Accessibility

- Tab strip uses `role="tablist"`.
- Selectable tab controls use `role="tab"` and `aria-selected`.
- Close/menu buttons have explicit accessible names.
- Dropdown menu items are keyboard reachable.
- Context menu must be dismissible with Escape and outside click.
- Avoid nested `button` elements.

## Testing Strategy

Add/update frontend tests in `web/src/App.test.tsx`:

1. Opening more than 5 sessions shows active/recent tabs and moves the rest into More.
2. Selecting a session from More activates it and makes it visible.
3. Streaming state displays a running indicator on the active/open tab.
4. Closing an active tab activates the next most recent tab.
5. Tab rename updates the tab title after backend success.
6. Tab Close does not delete the session from Projects.
7. Non-Chat pages do not render Chat tab strip or Chat-only controls.
8. Resizable sidebar changes width through pointer events.

Verification commands:

```powershell
npm test -- --run App.test.tsx
npm run build
```

Browser QA:

- desktop 1280px viewport
- narrower desktop around 1120px
- Projects → session → Chat tab
- create 6+ sessions and verify overflow
- switch overflow tab
- close active tab
- verify non-Chat pages remain clean

## Rollout Plan

1. Add tests for tab overflow and context menu behaviors.
2. Extend `ChatTab` state with `lastActiveAt` and `isStreaming`.
3. Implement visible/overflow helper.
4. Update tab strip rendering and CSS.
5. Add tab context menu actions using existing session helpers.
6. Add resizable context sidebar.
7. Run tests, build, and browser QA.

## Deferred Phase 2

A future phase should extract a real `ChatWorkspace` and introduce per-session cached state:

```ts
sessionStates[sessionId] = {
  messages,
  draft,
  context,
  isStreaming,
  runId,
  routeInfo,
  error,
  lastActiveAt,
}
```

That phase would make tab switching instant and preserve drafts/messages without full reloads. It is intentionally deferred to keep this phase safe.

## Implementation Flexibility

The approved scope is the medium approach. Implementation may choose between inline helpers and a small `ChatTabStrip` extraction based on code fit, but must not perform a broad chat-state rewrite in Phase 1.
