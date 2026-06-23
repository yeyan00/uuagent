# Chat Workspace UX Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade UUAgent Chat workspace tabs with visible/overflow management, tab actions, running indicators, and a resizable context sidebar while preserving Projects/Chat separation.

**Architecture:** Keep the existing `App.tsx` shell for Phase 1, but add focused helpers and narrowly scoped UI state so the work remains safe. Implement behavior test-first in `web/src/App.test.tsx`, then update `web/src/App.tsx` and `web/src/App.css`. Do not introduce a full per-session cached chat-state rewrite in this phase.

**Tech Stack:** React + TypeScript, Vite, Vitest + Testing Library, Lucide React icons, existing UUAgent REST/SSE endpoints.

---

## File Structure

**Modify:**
- `web/src/App.tsx` — extend `ChatTab`, add visible/overflow helper, tab menu state/actions, sidebar resize state, render updated Chat tab strip.
- `web/src/App.css` — style lighter tabs, overflow dropdown, running dot, context menu, sidebar resize handle.
- `web/src/App.test.tsx` — add regression tests for tab overflow, More dropdown activation, close semantics, rename, and sidebar resize.

**Do not modify in this phase:**
- Backend APIs unless a test proves an existing frontend action has no callable endpoint.
- Settings/Extensions/Schedules implementations except for non-Chat chrome assertions if needed.
- Full Chat state extraction into a new `ChatWorkspace`.

---

## Task 1: Add visible/overflow tab behavior tests

**Files:**
- Modify: `web/src/App.test.tsx`

- [ ] **Step 1: Write failing tests for tab overflow and active/recent ordering**

Add tests in the existing `describe('App', ...)` block near current Chat tab tests. Use the existing mock API patterns in this file. Create six sessions in one project, open them, and assert only five tabs are directly visible while the sixth is in a More menu.

Test intent:

```tsx
it('keeps active and recent chat sessions visible and moves older tabs into More', async () => {
  render(<App />)

  // Arrange using existing project/session mock helpers already in this test file.
  // Open six project sessions from Projects so each becomes a Chat tab.

  // Assert Chat page has a tablist.
  expect(screen.getByRole('tablist', { name: /open chat sessions/i })).toBeInTheDocument()

  // Assert visible role=tab count is capped at 5.
  expect(screen.getAllByRole('tab')).toHaveLength(5)

  // Assert a More button exists and reveals overflow sessions.
  await userEvent.click(screen.getByRole('button', { name: /more chat sessions/i }))
  expect(screen.getByRole('menu', { name: /overflow chat sessions/i })).toBeInTheDocument()

  // Assert the oldest opened session is inside overflow.
  expect(screen.getByRole('menuitem', { name: /session 1/i })).toBeInTheDocument()
})
```

Use actual session titles and helpers from `App.test.tsx`; do not invent new mock infrastructure if existing helpers can be extended.

- [ ] **Step 2: Write failing test for selecting overflow session**

```tsx
it('promotes an overflow chat session when selected from More', async () => {
  render(<App />)

  // Open six sessions.
  // Click More.
  await userEvent.click(screen.getByRole('button', { name: /more chat sessions/i }))
  await userEvent.click(screen.getByRole('menuitem', { name: /session 1/i }))

  expect(screen.getByRole('tab', { name: /session 1/i })).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByRole('menu', { name: /overflow chat sessions/i })).not.toBeInTheDocument()
})
```

- [ ] **Step 3: Run focused tests and verify RED**

Run:

```powershell
npm test -- --run App.test.tsx -t "keeps active and recent chat sessions visible|promotes an overflow chat session"
```

Expected: both tests fail because `More chat sessions` does not exist and all tabs are currently horizontal-scroll tabs.

- [ ] **Step 4: Commit red tests only if your workflow requires checkpoints**

Do not commit failing tests to the shared branch unless explicitly using a red-test checkpoint branch. Normal flow: keep them uncommitted and proceed to Task 2.

---

## Task 2: Implement visible/overflow tab helper and rendering

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`

- [ ] **Step 1: Extend `ChatTab` with `lastActiveAt` and `isStreaming`**

In `web/src/App.tsx`, update the `ChatTab` interface:

```ts
interface ChatTab {
  id: string
  projectId: string
  sessionId: string
  projectName: string
  title: string
  lastActiveAt: number
  isStreaming?: boolean
}
```

- [ ] **Step 2: Update `upsertChatTab` to refresh `lastActiveAt`**

Update the existing helper so every open/select/create operation refreshes recency:

```ts
const upsertChatTab = useCallback((pid: string, sid: string, title: string) => {
  const tabId = `${pid}:${sid}`
  const projectName = projects.find(project => project.id === pid)?.name ?? pid
  const now = Date.now()
  setOpenChatTabs(current => {
    const existing = current.find(tab => tab.id === tabId)
    if (existing) {
      return current.map(tab => tab.id === tabId ? { ...tab, projectName, title, lastActiveAt: now } : tab)
    }
    return [...current, { id: tabId, projectId: pid, sessionId: sid, projectName, title, lastActiveAt: now }]
  })
}, [projects])
```

Keep existing project/session title behavior intact.

- [ ] **Step 3: Add visible/overflow helper near tab helpers**

```ts
function getVisibleChatTabs(tabs: ChatTab[], activeTabId: string): { visibleTabs: ChatTab[]; overflowTabs: ChatTab[] } {
  const active = tabs.find(tab => tab.id === activeTabId)
  const ordered = [
    ...(active ? [active] : []),
    ...tabs
      .filter(tab => tab.id !== activeTabId)
      .sort((left, right) => right.lastActiveAt - left.lastActiveAt),
  ]
  return {
    visibleTabs: ordered.slice(0, 5),
    overflowTabs: ordered.slice(5),
  }
}
```

- [ ] **Step 4: Add overflow menu state**

```ts
const [showChatTabOverflow, setShowChatTabOverflow] = useState(false)
const { visibleTabs, overflowTabs } = useMemo(
  () => getVisibleChatTabs(openChatTabs, activeChatTabId),
  [activeChatTabId, openChatTabs],
)
```

- [ ] **Step 5: Render visible tabs and More dropdown**

Replace the current `openChatTabs.map(...)` render with `visibleTabs.map(...)` and add More when `overflowTabs.length > 0`:

```tsx
{isChatPage && openChatTabs.length > 0 && <div className="chatTabStrip" role="tablist" aria-label="Open chat sessions">
  {visibleTabs.map(tab => (
    <div key={tab.id} className={tab.id === activeChatTabId ? 'chatTab active' : 'chatTab'}>
      <button type="button" role="tab" aria-selected={tab.id === activeChatTabId} className="chatTabButton" onClick={() => selectChatTab(tab).catch(err => setStatus(String(err)))}>
        <span>{tab.projectName}</span>
        <strong>{tab.title}</strong>
        {tab.isStreaming && <i className="chatTabRunningDot" aria-label="Session is running" />}
      </button>
      <button type="button" className="chatTabClose" aria-label={`Close ${tab.projectName} ${tab.title}`} onClick={() => closeChatTab(tab.id)}><X size={13} /></button>
    </div>
  ))}
  {overflowTabs.length > 0 && <div className="chatTabOverflow">
    <button type="button" className="chatTabMore" aria-label="More chat sessions" onClick={() => setShowChatTabOverflow(value => !value)}>More</button>
    {showChatTabOverflow && <div className="chatTabOverflowMenu" role="menu" aria-label="Overflow chat sessions">
      {overflowTabs.map(tab => (
        <button key={tab.id} type="button" role="menuitem" className="chatTabOverflowItem" onClick={() => { setShowChatTabOverflow(false); selectChatTab(tab).catch(err => setStatus(String(err))) }}>
          <span>{tab.projectName}</span>
          <strong>{tab.title}</strong>
          {tab.isStreaming && <i className="chatTabRunningDot" aria-hidden="true" />}
        </button>
      ))}
    </div>}
  </div>}
</div>}
```

Avoid nested buttons.

- [ ] **Step 6: Update CSS for overflow and running dot**

Add/adjust in `web/src/App.css`:

```css
.chatTabStrip { position: relative; display: flex; gap: 8px; align-items: center; overflow: visible; padding: 0 20px 10px; border-bottom: 1px solid rgba(132,146,170,.14); }
.chatTab { flex: 0 0 clamp(150px, 18vw, 220px); min-width: 0; max-width: 220px; }
.chatTabButton { position: relative; }
.chatTabRunningDot { width: 7px; height: 7px; border-radius: 999px; background: #4c78ff; box-shadow: 0 0 0 4px rgba(76,120,255,.14); display: inline-block; }
.chatTabOverflow { position: relative; flex: 0 0 auto; }
.chatTabMore { border: 1px solid rgba(132,146,170,.2); border-radius: 999px; background: #edf2fb; color: #31415d; padding: 8px 12px; font-size: 12px; font-weight: 800; }
.chatTabOverflowMenu { position: absolute; top: calc(100% + 8px); right: 0; z-index: 20; min-width: 230px; max-height: 320px; overflow-y: auto; border: 1px solid #dde3ee; border-radius: 14px; background: #fff; box-shadow: 0 16px 36px rgba(24,36,58,.14); padding: 6px; }
.chatTabOverflowItem { width: 100%; border: 0; border-radius: 10px; background: transparent; color: #25324b; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 8px; text-align: left; padding: 9px 10px; }
.chatTabOverflowItem:hover { background: #eef4ff; }
.chatTabOverflowItem span, .chatTabOverflowItem strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chatTabOverflowItem span { color: #62738f; font-size: 11px; }
.chatTabOverflowItem strong { font-size: 13px; }
```

- [ ] **Step 7: Run focused tests and verify GREEN**

Run:

```powershell
npm test -- --run App.test.tsx -t "keeps active and recent chat sessions visible|promotes an overflow chat session"
```

Expected: tests pass.

---

## Task 3: Add close semantics and running indicator tests

**Files:**
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`

- [ ] **Step 1: Write failing test for closing active tab**

```tsx
it('activates the next most recent tab when closing the active chat tab', async () => {
  render(<App />)

  // Open at least three sessions: A, B, C. C is active.
  // Close C.
  await userEvent.click(screen.getByRole('button', { name: /close .*session c/i }))

  expect(screen.getByRole('tab', { name: /session b/i })).toHaveAttribute('aria-selected', 'true')
})
```

Use exact session titles created by the test helpers.

- [ ] **Step 2: Write failing test for running indicator**

Mock a streaming response that keeps a run active long enough for the UI to render the running state. Assert the active tab has an accessible running indicator:

```tsx
it('shows a running indicator on the active chat tab while streaming', async () => {
  render(<App />)

  // Open a session and submit a prompt using existing chat test helpers.
  await userEvent.type(screen.getByRole('textbox', { name: /message/i }), 'hello')
  await userEvent.click(screen.getByRole('button', { name: /send/i }))

  expect(await screen.findByLabelText(/session is running/i)).toBeInTheDocument()
})
```

- [ ] **Step 3: Run tests and verify RED**

Run:

```powershell
npm test -- --run App.test.tsx -t "activates the next most recent tab|shows a running indicator"
```

Expected: close ordering and/or running indicator test fails.

- [ ] **Step 4: Update `closeChatTab` to use recency ordering**

When closing active tab, select the most recent remaining tab:

```ts
const closeChatTab = useCallback((tabId: string) => {
  setOpenChatTabs(current => {
    const remaining = current.filter(tab => tab.id !== tabId)
    if (tabId === activeChatTabId) {
      const next = [...remaining].sort((left, right) => right.lastActiveAt - left.lastActiveAt)[0]
      if (next) {
        void selectChatTab(next)
      } else {
        setSessionId('')
        setMessages([])
        setContext(null)
        setRouteInfo(null)
      }
    }
    return remaining
  })
}, [activeChatTabId, selectChatTab])
```

Adapt names to current state variables in `App.tsx`; preserve existing cleanup behavior.

- [ ] **Step 5: Update streaming state on tabs**

When a run starts for the active session, update the active tab:

```ts
setOpenChatTabs(current => current.map(tab =>
  tab.id === activeChatTabId ? { ...tab, isStreaming: true, lastActiveAt: Date.now() } : tab
))
```

When run completes/errors/stops, clear it:

```ts
setOpenChatTabs(current => current.map(tab =>
  tab.id === activeChatTabId ? { ...tab, isStreaming: false } : tab
))
```

Place these updates in the same code paths that set `isStreaming` true/false for Chat.

- [ ] **Step 6: Run focused tests and verify GREEN**

Run:

```powershell
npm test -- --run App.test.tsx -t "activates the next most recent tab|shows a running indicator"
```

Expected: tests pass.

---

## Task 4: Add tab context menu with Rename, Fork, Compact, Export, Close

**Files:**
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`

- [ ] **Step 1: Write failing context menu render test**

```tsx
it('opens a chat tab context menu with session actions', async () => {
  render(<App />)

  // Open a session as a Chat tab.
  fireEvent.contextMenu(screen.getByRole('tab', { name: /session/i }))

  expect(screen.getByRole('menu', { name: /chat tab actions/i })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: /rename/i })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: /fork/i })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: /compact/i })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: /export context/i })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: /close tab/i })).toBeInTheDocument()
})
```

- [ ] **Step 2: Write failing rename action test**

```tsx
it('renames a chat tab after session rename succeeds', async () => {
  render(<App />)

  // Open a session as a Chat tab.
  fireEvent.contextMenu(screen.getByRole('tab', { name: /session/i }))
  await userEvent.click(screen.getByRole('menuitem', { name: /rename/i }))
  await userEvent.clear(screen.getByRole('textbox', { name: /session title/i }))
  await userEvent.type(screen.getByRole('textbox', { name: /session title/i }), 'Renamed tab')
  await userEvent.keyboard('{Enter}')

  expect(await screen.findByRole('tab', { name: /renamed tab/i })).toBeInTheDocument()
})
```

Use the existing session rename endpoint mock already used by sidebar rename tests; if no helper exists, add one in the same style as existing API mocks.

- [ ] **Step 3: Run tests and verify RED**

Run:

```powershell
npm test -- --run App.test.tsx -t "opens a chat tab context menu|renames a chat tab"
```

Expected: tests fail because context menu does not exist.

- [ ] **Step 4: Add context menu state**

```ts
const [chatTabMenu, setChatTabMenu] = useState<{ tabId: string; x: number; y: number } | null>(null)
const [renamingChatTabId, setRenamingChatTabId] = useState<string | null>(null)
const [renamingChatTabTitle, setRenamingChatTabTitle] = useState('')
```

- [ ] **Step 5: Dismiss menu on outside click/Escape**

Add an effect:

```ts
useEffect(() => {
  if (!chatTabMenu) return
  const close = () => setChatTabMenu(null)
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') setChatTabMenu(null)
  }
  document.addEventListener('click', close)
  document.addEventListener('keydown', onKeyDown)
  return () => {
    document.removeEventListener('click', close)
    document.removeEventListener('keydown', onKeyDown)
  }
}, [chatTabMenu])
```

- [ ] **Step 6: Add context menu trigger to tab button**

```tsx
onContextMenu={event => {
  event.preventDefault()
  setChatTabMenu({ tabId: tab.id, x: event.clientX, y: event.clientY })
}}
```

- [ ] **Step 7: Render context menu**

```tsx
{chatTabMenu && <div className="chatTabContextMenu" role="menu" aria-label="Chat tab actions" style={{ left: chatTabMenu.x, top: chatTabMenu.y }} onClick={event => event.stopPropagation()}>
  <button type="button" role="menuitem" onClick={() => startRenameChatTab(chatTabMenu.tabId)}>Rename</button>
  <button type="button" role="menuitem" onClick={() => forkChatTab(chatTabMenu.tabId)}>Fork</button>
  <button type="button" role="menuitem" onClick={() => compactChatTab(chatTabMenu.tabId)} disabled={openChatTabs.find(tab => tab.id === chatTabMenu.tabId)?.isStreaming}>Compact</button>
  <button type="button" role="menuitem" onClick={() => exportChatTabContext(chatTabMenu.tabId)}>Export Context</button>
  <button type="button" role="menuitem" onClick={() => { closeChatTab(chatTabMenu.tabId); setChatTabMenu(null) }}>Close Tab</button>
</div>}
```

- [ ] **Step 8: Implement rename helper minimally**

Use the existing session rename API/helper currently used by sidebar session rename. After success, update both session list and tab title:

```ts
const finishRenameChatTab = useCallback(async (tabId: string, nextTitle: string) => {
  const tab = openChatTabs.find(item => item.id === tabId)
  if (!tab) return
  const title = nextTitle.trim()
  if (!title) return
  await renameSession(tab.projectId, tab.sessionId, title)
  setOpenChatTabs(current => current.map(item => item.id === tabId ? { ...item, title } : item))
  await refreshProjects()
  setRenamingChatTabId(null)
}, [openChatTabs, refreshProjects])
```

Adapt helper names to the actual code. Do not create duplicate API logic if a helper already exists.

- [ ] **Step 9: Implement Fork/Compact/Export by delegating to existing helpers**

For each action:

- Load/select the tab if the existing helper requires active `projectId/sessionId`.
- Then call the existing helper.
- If the existing helper cannot operate on non-active tabs without large changes, select the tab first and perform the action.

Document any deferred action in code comments only if absolutely necessary; prefer working action wiring.

- [ ] **Step 10: Add CSS for context menu and rename input**

```css
.chatTabContextMenu { position: fixed; z-index: 50; min-width: 160px; border: 1px solid rgba(132,146,170,.2); border-radius: 12px; background: #fff; box-shadow: 0 16px 36px rgba(24,36,58,.16); padding: 6px; }
.chatTabContextMenu button { width: 100%; border: 0; border-radius: 9px; background: transparent; color: #25324b; text-align: left; padding: 8px 10px; }
.chatTabContextMenu button:hover:not(:disabled) { background: #eef4ff; }
.chatTabContextMenu button:disabled { opacity: .55; cursor: not-allowed; }
.chatTabRenameInput { width: 100%; border: 0; outline: 0; background: transparent; color: inherit; font: inherit; font-weight: 850; }
```

- [ ] **Step 11: Run focused tests and verify GREEN**

Run:

```powershell
npm test -- --run App.test.tsx -t "opens a chat tab context menu|renames a chat tab"
```

Expected: tests pass.

---

## Task 5: Add resizable context sidebar

**Files:**
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.css`

- [ ] **Step 1: Write failing sidebar resize test**

```tsx
it('resizes the context sidebar with the drag handle', async () => {
  render(<App />)

  const sidebar = screen.getByLabelText(/context sidebar/i)
  const handle = screen.getByRole('separator', { name: /resize context sidebar/i })

  expect(sidebar).toHaveStyle({ width: '320px' })

  fireEvent.pointerDown(handle, { clientX: 320, pointerId: 1 })
  fireEvent.pointerMove(handle, { clientX: 400, pointerId: 1 })
  fireEvent.pointerUp(handle, { pointerId: 1 })

  expect(sidebar).toHaveStyle({ width: '400px' })
})
```

If Testing Library style assertions need exact inline width, render width as an inline style on `.contextSidebar`.

- [ ] **Step 2: Run test and verify RED**

Run:

```powershell
npm test -- --run App.test.tsx -t "resizes the context sidebar"
```

Expected: fails because there is no separator/drag behavior.

- [ ] **Step 3: Add sidebar width state and refs**

```ts
const [contextSidebarWidth, setContextSidebarWidth] = useState(320)
const sidebarDrag = useRef<{ startX: number; startWidth: number } | null>(null)
```

- [ ] **Step 4: Add pointer handlers**

```ts
const startContextSidebarResize = useCallback((event: React.PointerEvent) => {
  sidebarDrag.current = { startX: event.clientX, startWidth: contextSidebarWidth }
  event.currentTarget.setPointerCapture(event.pointerId)
}, [contextSidebarWidth])

const moveContextSidebarResize = useCallback((event: React.PointerEvent) => {
  if (!sidebarDrag.current) return
  const delta = event.clientX - sidebarDrag.current.startX
  setContextSidebarWidth(Math.min(480, Math.max(240, sidebarDrag.current.startWidth + delta)))
}, [])

const stopContextSidebarResize = useCallback(() => {
  sidebarDrag.current = null
}, [])
```

- [ ] **Step 5: Apply inline width and render separator**

```tsx
<aside className="contextSidebar" aria-label="Context sidebar" style={{ width: contextSidebarWidth }}>
  ...
</aside>
<div
  className="contextSidebarResizeHandle"
  role="separator"
  aria-label="Resize context sidebar"
  aria-orientation="vertical"
  onPointerDown={startContextSidebarResize}
  onPointerMove={moveContextSidebarResize}
  onPointerUp={stopContextSidebarResize}
/>
```

Because `.appWindow` currently uses grid columns, update layout to support fixed nav + variable sidebar + workspace. One simple approach:

```css
.appWindow { grid-template-columns: 92px auto minmax(520px, 1fr); }
.contextSidebar { width: 320px; }
```

Inline style overrides the default width.

- [ ] **Step 6: Style resize handle**

```css
.contextSidebarResizeHandle { width: 6px; margin-left: -3px; cursor: col-resize; background: transparent; position: relative; z-index: 4; }
.contextSidebarResizeHandle::after { content: ''; position: absolute; top: 0; bottom: 0; left: 2px; width: 1px; background: rgba(132,146,170,.18); }
.contextSidebarResizeHandle:hover::after { background: rgba(76,120,255,.5); }
```

Ensure it does not visually create a fourth bulky column.

- [ ] **Step 7: Run focused test and verify GREEN**

Run:

```powershell
npm test -- --run App.test.tsx -t "resizes the context sidebar"
```

Expected: test passes.

---

## Task 6: Guard non-Chat page chrome and run full verification

**Files:**
- Modify: `web/src/App.test.tsx` if assertions need updates

- [ ] **Step 1: Add/extend non-Chat regression test**

Ensure the existing non-Chat chrome test covers the new controls:

```tsx
it('keeps Chat tabs and Chat controls out of non-Chat pages', async () => {
  render(<App />)

  for (const page of ['Projects', 'Extensions', 'Schedules', 'Settings']) {
    await userEvent.click(screen.getByRole('button', { name: page }))
    expect(screen.queryByRole('tablist', { name: /open chat sessions/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /more chat sessions/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('menu', { name: /chat tab actions/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /message/i })).not.toBeInTheDocument()
    expect(screen.queryByText(/session tokens/i)).not.toBeInTheDocument()
  }
})
```

- [ ] **Step 2: Run full App tests**

Run:

```powershell
npm test -- --run App.test.tsx
```

Expected: all App tests pass.

- [ ] **Step 3: Run production build**

Run:

```powershell
npm run build
```

Expected: TypeScript and Vite build succeed.

- [ ] **Step 4: Browser QA at running UUAgent UI**

Use local UI, typically:

```text
http://localhost:18463/ui/
```

Verify manually or via Playwright:

- Projects opens with no Chat composer/tabs.
- Click a project session; Chat opens with tab visible.
- Create/open 6+ sessions; only five visible tabs and More appears.
- Select a More item; it becomes active visible tab.
- Right-click a tab; menu opens and Escape dismisses it.
- Rename tab; title updates.
- Close active tab; next recent tab activates.
- Resize sidebar; width clamps between 240px and 480px.
- Extensions/Schedules/Settings show no Chat-only chrome.

- [ ] **Step 5: Visual QA**

Use `visual-qa` after UI changes. Required evidence:

- screenshot at 1280x900 with multiple Chat tabs and More dropdown
- screenshot at 1120x800 showing sidebar and tabs still usable
- screenshot of non-Chat page proving no Chat chrome leak

Expected: visual QA PASS or documented non-blocking findings accepted by user.

---

## Task 7: Final git review and commit strategy

**Files:**
- Review all changed files.

- [ ] **Step 1: Inspect status and diff**

Run:

```powershell
$env:GIT_MASTER='1'; git status --short --branch
$env:GIT_MASTER='1'; git diff --stat
$env:GIT_MASTER='1'; git diff --check
```

Expected:

- Only intended UI/test files changed for this feature.
- `git diff --check` has no whitespace errors.

- [ ] **Step 2: Split commits by concern**

Recommended commits:

1. `Add Chat tab overflow behavior`
   - `web/src/App.tsx`
   - `web/src/App.css`
   - `web/src/App.test.tsx`
   - Includes visible/overflow, More dropdown, close ordering, running dot.

2. `Add Chat tab session actions`
   - `web/src/App.tsx`
   - `web/src/App.css`
   - `web/src/App.test.tsx`
   - Includes context menu and rename/fork/compact/export/close actions.

3. `Make context sidebar resizable`
   - `web/src/App.tsx`
   - `web/src/App.css`
   - `web/src/App.test.tsx`
   - Includes drag handle and tests.

If implementation keeps all code interdependent in one file and splitting would break tests, use two commits instead:

1. `Add Chat tab overflow and actions`
2. `Make context sidebar resizable`

- [ ] **Step 3: Commit with repo plain-English style**

Use `$env:GIT_MASTER='1';` prefix for every git command.

Example:

```powershell
$env:GIT_MASTER='1'; git add web/src/App.tsx web/src/App.css web/src/App.test.tsx
$env:GIT_MASTER='1'; git commit -m "Add Chat tab overflow behavior" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

Do not push unless the user explicitly asks.

---

## Self-Review

Spec coverage:

- Visible/overflow tabs: Task 1, Task 2.
- Active/recent ordering: Task 1, Task 2, Task 3.
- Running indicators: Task 3.
- Tab context menu/actions: Task 4.
- Resizable sidebar: Task 5.
- Projects vs Chat separation and non-Chat chrome: Task 6.
- Testing/build/browser QA: Task 6.
- Commit/review workflow: Task 7.

Placeholder scan:

- No TBD/TODO placeholders are used.
- Deferred full per-session cache remains intentionally out of scope.

Type consistency:

- `ChatTab.lastActiveAt` and `ChatTab.isStreaming` are introduced before use.
- `getVisibleChatTabs()` return names match rendering steps.
- ARIA labels used by tests match rendering snippets.
