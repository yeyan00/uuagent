# Compact Archive History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a manual Compact action that archives compacted messages, replaces older context with a summary, and shows compact history in the existing Web UI.

**Architecture:** Reuse existing deterministic context compression primitives and project-scoped session API patterns. Extend session persistence with explicit compact archives so the original compacted messages are preserved, add one project-scoped compact endpoint, and wire one minimal workspace-header button plus existing Project Settings context history display.

**Tech Stack:** Go backend (`internal/session`, `internal/contextmgr`, `api/server`, `tests/session`), React/Vitest frontend (`web/src/App.tsx`, `web/src/App.test.tsx`), PowerShell validation on Windows.

---

## Scope Decisions

- Manual compact semantics are **Archive + summary**.
- The compact action preserves removed messages in a persisted archive record.
- Active `Session.Messages` is replaced with a generated summary system message plus the latest kept messages.
- The first UI affordance is a compact button in the active workspace header.
- Compact history initially appears in the existing Project Settings `context` tab, reusing summary/history patterns.
- Restore is not part of this slice. Archive records are persisted so restore/fork can be added later without data loss.

## File Structure

- Modify `internal/contextmgr/context.go`
  - Add a reusable compact result that includes summary, archived messages, and compacted active messages.
  - Keep `CompressOldMessages` compatible by delegating to the new helper.
- Modify `internal/session/store.go`
  - Add `CompactArchive` persistence model.
  - Add `Archives []CompactArchive` to `Session`.
  - Add `CompactArchive(maxTokens int, threshold float64, keepLast int) (CompactArchive, bool)`.
  - Add `ListArchives() []CompactArchive`.
  - Include archives in `Snapshot()` deep copies.
- Modify `api/server/server.go`
  - Register `POST /api/projects/:id/sessions/:session_id/compact`.
  - Add handler using existing project-session lookup pattern.
  - Return updated `context`, `usage`, `summaries`, and `archives`.
- Modify `web/src/App.tsx`
  - Extend frontend `SessionContext` with `archives?: CompactArchive[]`.
  - Add `compactSession()` handler.
  - Add compact button in `workspaceActions` for active chat sessions.
  - Render compact archives in Project Settings context tab under summaries.
- Modify `web/src/App.test.tsx`
  - Add focused compact button/history test.
- Add/modify tests:
  - `tests/session/context_test.go`
  - `tests/session/api_test.go`
  - `tests/session/persistence_test.go`

---

## Task 1: Backend Archive Model and Session Method

**Files:**
- Modify: `internal/contextmgr/context.go`
- Modify: `internal/session/store.go`
- Test: `tests/session/context_test.go`
- Test: `tests/session/persistence_test.go`

### Step 1: Write failing session archive test

Add this test to `tests/session/context_test.go`:

```go
func TestSessionCompactArchivePreservesCompactedMessages(t *testing.T) {
	s := session.NewStoreAt(t.TempDir()).GetOrCreate("compact-archive")
	for i := 0; i < 20; i++ {
		s.Append("user", fmt.Sprintf("old message %02d with enough text to count as context", i))
	}

	archive, ok := s.CompactArchive(80, 0.5, 4)
	if !ok {
		t.Fatal("expected compact archive to be created")
	}

	if archive.ID == "" {
		t.Fatal("expected archive id")
	}
	if archive.Summary.ID == "" {
		t.Fatal("expected summary id")
	}
	if len(archive.Messages) == 0 {
		t.Fatal("expected archived messages to preserve compacted content")
	}
	if len(s.ListArchives()) != 1 {
		t.Fatalf("expected one archive, got %d", len(s.ListArchives()))
	}
	if len(s.ListSummaries()) != 1 {
		t.Fatalf("expected one summary, got %d", len(s.ListSummaries()))
	}
	if len(s.Snapshot().Messages) >= 20 {
		t.Fatalf("expected active context to shrink, got %d messages", len(s.Snapshot().Messages))
	}
}
```

If `fmt` is not already imported in `tests/session/context_test.go`, add it to the import block.

### Step 2: Write failing persistence test

Add this test to `tests/session/persistence_test.go`:

```go
func TestCompactArchivesPersistToJSON(t *testing.T) {
	root := t.TempDir()
	store := session.NewStoreAt(root)
	s := store.GetOrCreate("persist-archive")
	for i := 0; i < 20; i++ {
		s.Append("user", fmt.Sprintf("persisted old message %02d with enough text", i))
	}
	archive, ok := s.CompactArchive(80, 0.5, 4)
	if !ok {
		t.Fatal("expected compact archive")
	}

	reloaded := session.NewStoreAt(root)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load sessions: %v", err)
	}
	loaded, ok := reloaded.Get("persist-archive")
	if !ok {
		t.Fatal("expected session to reload")
	}
	archives := loaded.ListArchives()
	if len(archives) != 1 {
		t.Fatalf("expected one persisted archive, got %d", len(archives))
	}
	if archives[0].ID != archive.ID {
		t.Fatalf("expected archive id %q, got %q", archive.ID, archives[0].ID)
	}
	if len(archives[0].Messages) == 0 {
		t.Fatal("expected archived messages to reload")
	}
}
```

If `fmt` is not already imported in `tests/session/persistence_test.go`, add it to the import block.

### Step 3: Run tests to verify RED

Run:

```powershell
go test ./tests/session -run "CompactArchive|ArchivesPersist" -count=1
```

Expected: FAIL because `Session.CompactArchive` and `Session.ListArchives` do not exist.

### Step 4: Implement reusable context compaction result

In `internal/contextmgr/context.go`, add this type near `Summary`:

```go
type CompactResult struct {
	Messages         []types.Message
	ArchivedMessages []types.Message
	Summary          Summary
	Compacted        bool
}
```

Add this function below `CompressOldMessages` or immediately before it:

```go
func CompactOldMessages(sessionID string, messages []types.Message, keepLast int) CompactResult {
	if len(messages) <= keepLast+1 {
		return CompactResult{Messages: append([]types.Message(nil), messages...)}
	}
	cut := len(messages) - keepLast
	old := append([]types.Message(nil), messages[:cut]...)
	kept := append([]types.Message(nil), messages[cut:]...)
	before := EstimateTokens(messages)
	summaryText := summarizeMessages(old)
	summaryMsg := types.Message{Role: "system", Content: "Previous conversation summary:\n" + summaryText}
	newMessages := append([]types.Message{summaryMsg}, kept...)
	after := EstimateTokens(newMessages)
	summary := Summary{
		ID:            fmt.Sprintf("sum_%d", time.Now().UnixNano()),
		SessionID:     sessionID,
		FromIndex:     0,
		ToIndex:       cut - 1,
		Summary:       summaryText,
		TokenBefore:   before,
		TokenAfter:    after,
		CreatedAt:     time.Now(),
		CompressionBy: "local",
	}
	return CompactResult{Messages: newMessages, ArchivedMessages: old, Summary: summary, Compacted: true}
}
```

Then update existing `CompressOldMessages` to delegate without changing its public signature:

```go
func CompressOldMessages(sessionID string, messages []types.Message, keepLast int) ([]types.Message, Summary, bool) {
	result := CompactOldMessages(sessionID, messages, keepLast)
	return result.Messages, result.Summary, result.Compacted
}
```

### Step 5: Implement session archive persistence

In `internal/session/store.go`, add this type near `Session`-related persisted structs:

```go
type CompactArchive struct {
	ID          string               `json:"id"`
	SessionID   string               `json:"session_id"`
	Summary     contextmgr.Summary   `json:"summary"`
	Messages    []types.Message      `json:"messages"`
	FromIndex   int                  `json:"from_index"`
	ToIndex     int                  `json:"to_index"`
	TokenBefore int                  `json:"token_before"`
	TokenAfter  int                  `json:"token_after"`
	CreatedAt   time.Time            `json:"created_at"`
}
```

Add this field to `Session`:

```go
Archives []CompactArchive `json:"archives,omitempty"`
```

Add these methods near `MaybeCompress` and `ListSummaries`:

```go
func (s *Session) CompactArchive(maxTokens int, threshold float64, keepLast int) (CompactArchive, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !contextmgr.ShouldCompress(s.Messages, maxTokens, threshold) {
		return CompactArchive{}, false
	}
	result := contextmgr.CompactOldMessages(s.ID, s.Messages, keepLast)
	if !result.Compacted {
		return CompactArchive{}, false
	}
	s.Messages = result.Messages
	s.Summaries = append(s.Summaries, result.Summary)
	archive := CompactArchive{
		ID:          fmt.Sprintf("arc_%d", time.Now().UnixNano()),
		SessionID:   s.ID,
		Summary:     result.Summary,
		Messages:    append([]types.Message(nil), result.ArchivedMessages...),
		FromIndex:   result.Summary.FromIndex,
		ToIndex:     result.Summary.ToIndex,
		TokenBefore: result.Summary.TokenBefore,
		TokenAfter:  result.Summary.TokenAfter,
		CreatedAt:   result.Summary.CreatedAt,
	}
	s.Archives = append(s.Archives, archive)
	s.UpdatedAt = time.Now()
	if s.store != nil {
		_ = s.store.saveLocked(s)
	}
	return archive, true
}

func (s *Session) ListArchives() []CompactArchive {
	s.mu.Lock()
	defer s.mu.Unlock()
	archives := make([]CompactArchive, len(s.Archives))
	for i, archive := range s.Archives {
		archives[i] = archive
		archives[i].Messages = append([]types.Message(nil), archive.Messages...)
	}
	return archives
}
```

Update `Snapshot()` so it deep-copies archives:

```go
snap.Archives = make([]CompactArchive, len(s.Archives))
for i, archive := range s.Archives {
	snap.Archives[i] = archive
	snap.Archives[i].Messages = append([]types.Message(nil), archive.Messages...)
}
```

Keep existing summary and message copy behavior unchanged.

### Step 6: Run tests to verify GREEN

Run:

```powershell
go test ./tests/session -run "CompactArchive|ArchivesPersist" -count=1
```

Expected: PASS.

### Step 7: Run broader session tests

Run:

```powershell
go test ./tests/session -count=1
```

Expected: PASS.

### Step 8: Commit backend model slice

Run:

```powershell
$env:GIT_MASTER='1'; git add internal/contextmgr/context.go internal/session/store.go tests/session/context_test.go tests/session/persistence_test.go
$env:GIT_MASTER='1'; git commit -m "Add compact archive persistence" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

Expected: one commit containing backend archive model, methods, and direct tests.

---

## Task 2: Project-Scoped Compact API

**Files:**
- Modify: `api/server/server.go`
- Test: `tests/session/api_test.go`

### Step 1: Write failing API test

Add this test to `tests/session/api_test.go`:

```go
func TestProjectSessionCompactAPIArchivesContext(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	agt := agent.New(agent.Config{Port: 18463})
	router := server.New(agt)

	projectID := createProjectForTest(t, router, "Compact Repo", t.TempDir())
	store, _, ok := agt.ProjectSessions(projectID)
	if !ok {
		t.Fatal("expected project session store")
	}
	s := store.GetOrCreate("s-compact")
	for i := 0; i < 20; i++ {
		s.Append("user", fmt.Sprintf("api compact message %02d with enough text", i))
	}

	body := strings.NewReader(`{"keep_last_messages":4}`)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/sessions/s-compact/compact", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Context  session.ContextStats      `json:"context"`
		Usage    session.TokenUsage        `json:"usage"`
		Summaries []contextmgr.Summary     `json:"summaries"`
		Archives []session.CompactArchive `json:"archives"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode compact response: %v", err)
	}
	if len(got.Summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(got.Summaries))
	}
	if len(got.Archives) != 1 {
		t.Fatalf("expected one archive, got %d", len(got.Archives))
	}
	if len(got.Archives[0].Messages) == 0 {
		t.Fatal("expected archived messages in response")
	}
	if got.Context.EstimatedTokens <= 0 {
		t.Fatalf("expected context stats after compact, got %+v", got.Context)
	}
}
```

Ensure imports include `contextmgr`, `session`, `strings`, and existing packages used by the file.

### Step 2: Run test to verify RED

Run:

```powershell
go test ./tests/session -run TestProjectSessionCompactAPIArchivesContext -count=1
```

Expected: FAIL with 404 because compact route does not exist.

### Step 3: Register compact route

In `api/server/server.go`, add this route next to other project session routes:

```go
api.POST("/projects/:id/sessions/:session_id/compact", s.handleCompactProjectSession)
```

### Step 4: Add request/response types and handler

In `api/server/server.go`, add request type near other small API request structs:

```go
type compactSessionRequest struct {
	KeepLastMessages int     `json:"keep_last_messages"`
	Threshold        float64 `json:"threshold"`
}
```

Add handler near `handleGetProjectSessionContext`:

```go
func (s *Server) handleCompactProjectSession(c *gin.Context) {
	agt := s.agent(c)
	store, _, ok := agt.ProjectSessions(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	sess, ok := store.Get(c.Param("session_id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	var req compactSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := agt.Config().Agent.Context
	keepLast := cfg.KeepLastMessages
	if req.KeepLastMessages > 0 {
		keepLast = req.KeepLastMessages
	}
	threshold := cfg.CompressThreshold
	if req.Threshold > 0 {
		threshold = req.Threshold
	}
	if _, ok := sess.CompactArchive(cfg.MaxTokens, threshold, keepLast); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session context is below compact threshold"})
		return
	}
	snap := sess.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"context":   sess.ContextStats(cfg.MaxTokens),
		"usage":     snap.Usage,
		"summaries": snap.Summaries,
		"archives":  snap.Archives,
	})
}
```

If `ShouldBindJSON` on an empty body returns EOF in this project differently, replace the error condition with an explicit EOF check using `errors.Is(err, io.EOF)` and import `errors`/`io`.

### Step 5: Run API test to verify GREEN

Run:

```powershell
go test ./tests/session -run TestProjectSessionCompactAPIArchivesContext -count=1
```

Expected: PASS.

### Step 6: Run session API suite

Run:

```powershell
go test ./tests/session -count=1
```

Expected: PASS.

### Step 7: Commit API slice

Run:

```powershell
$env:GIT_MASTER='1'; git add api/server/server.go tests/session/api_test.go
$env:GIT_MASTER='1'; git commit -m "Add project session compact API" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

Expected: one commit containing route, handler, and API test.

---

## Task 3: Frontend Compact Button and History

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`

### Step 1: Write failing frontend test

Add this test to `web/src/App.test.tsx` near the active session token usage test:

```tsx
it('compacts the active session and shows compact archive history', async () => {
  Element.prototype.scrollIntoView = vi.fn()
  const calls: string[] = []
  const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push(`${init?.method || 'GET'} ${url}`)
    if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
    if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
    if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Compact session', messages: [{ role: 'user', content: 'hi' }] }] })
    if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Compact session', messages: [{ role: 'user', content: 'hi' }] })
    if (url === '/api/projects/proj-1/sessions/s1/context') return Response.json({
      context: { estimated_tokens: 12000, max_tokens: 32000, percent: 0.375 },
      usage: { input_tokens: 48000, output_tokens: 9000, total_tokens: 57000 },
      summaries: [{ id: 'sum-1', summary: 'Archived older discussion', token_before: 48000, token_after: 12000, created_at: '2026-06-18T00:00:00Z' }],
      archives: [{ id: 'arc-1', summary: { id: 'sum-1', summary: 'Archived older discussion', token_before: 48000, token_after: 12000, created_at: '2026-06-18T00:00:00Z' }, messages: [{ role: 'user', content: 'older message' }] }]
    })
    if (url === '/api/projects/proj-1/sessions/s1/compact') return Response.json({
      context: { estimated_tokens: 12000, max_tokens: 32000, percent: 0.375 },
      usage: { input_tokens: 48000, output_tokens: 9000, total_tokens: 57000 },
      summaries: [{ id: 'sum-1', summary: 'Archived older discussion', token_before: 48000, token_after: 12000, created_at: '2026-06-18T00:00:00Z' }],
      archives: [{ id: 'arc-1', summary: { id: 'sum-1', summary: 'Archived older discussion', token_before: 48000, token_after: 12000, created_at: '2026-06-18T00:00:00Z' }, messages: [{ role: 'user', content: 'older message' }] }]
    })
    if (url === '/api/memory') return Response.json({ memories: [] })
    if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
    if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
    return Response.json({})
  })
  globalThis.fetch = fetchMock

  render(<App />)
  fireEvent.click(await screen.findByText('Compact session'))
  fireEvent.click(await screen.findByRole('button', { name: 'Compact' }))

  await waitFor(() => expect(calls).toContain('POST /api/projects/proj-1/sessions/s1/compact'))
  fireEvent.click(await screen.findByText('Settings'))
  fireEvent.click(await screen.findByText('Context'))

  expect(await screen.findByText('Compact Archives')).toBeTruthy()
  expect(await screen.findByText('Archived older discussion')).toBeTruthy()
  expect(await screen.findByText('48k → 12k')).toBeTruthy()
})
```

### Step 2: Run test to verify RED

Run:

```powershell
cd web
npm test -- --run -t "compacts the active session"
```

Expected: FAIL because the Compact button does not exist.

### Step 3: Add frontend archive types

In `web/src/App.tsx`, add this type near `Summary`:

```tsx
interface CompactArchive {
  id: string
  summary: Summary
  messages?: Message[]
  token_before?: number
  token_after?: number
  created_at?: string
}
```

Extend `SessionContext`:

```tsx
interface SessionContext { context?: ContextStats; usage?: TokenUsage; summaries?: Summary[]; archives?: CompactArchive[] }
```

### Step 4: Add compact handler

In `web/src/App.tsx`, add this function near existing session action handlers:

```tsx
const compactSession = async () => {
  if (!projectId || !sessionId) return
  setStatus('Compacting session...')
  const result = await api<SessionContext>(`/api/projects/${projectId}/sessions/${sessionId}/compact`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  })
  setSessionContext(result || {})
  setSummaries(result.summaries || [])
  setStatus('Session compacted')
  await refresh()
}
```

If existing `api()` already injects JSON headers elsewhere, match the existing local style rather than creating a second convention.

### Step 5: Add header compact button

In `web/src/App.tsx`, inside `<div className="workspaceActions">`, add a button independent from the token pill:

```tsx
{workspaceMode !== 'settings' && projectId && sessionId && <button className="softButton" onClick={compactSession}>Compact</button>}
```

Place it near the session token pill so it reads as an active-session action.

### Step 6: Render compact archives in Project Settings context tab

In the context tab block that currently renders summaries, add this section after the summaries list:

```tsx
<h3>Compact Archives</h3>
{!(sessionContext.archives || []).length && <p className="emptyState">No compact archives yet.</p>}
{(sessionContext.archives || []).map((archive) => <details key={archive.id} className="summaryCard"><summary>{formatTokens(archive.summary?.token_before || archive.token_before)} → {formatTokens(archive.summary?.token_after || archive.token_after)}</summary><pre>{archive.summary?.summary}</pre></details>)}
```

Keep existing `Compression Summaries` rendering unchanged.

### Step 7: Run focused frontend test to verify GREEN

Run:

```powershell
cd web
npm test -- --run -t "compacts the active session"
```

Expected: PASS.

### Step 8: Run frontend suite and build

Run:

```powershell
cd web
npm test -- --run
npm run build
```

Expected: `24+` tests pass and production build succeeds.

### Step 9: Commit frontend slice

Run:

```powershell
$env:GIT_MASTER='1'; git add web/src/App.tsx web/src/App.test.tsx
$env:GIT_MASTER='1'; git commit -m "Add compact archive UI" -m "Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-openagent)" -m "Co-authored-by: Sisyphus <clio-agent@sisyphuslabs.ai>"
```

Expected: one commit containing UI handler, button, history rendering, and frontend test.

---

## Task 4: Full Validation

**Files:**
- No source edits expected.

### Step 1: Run full validation script

Run from repository root:

```powershell
powershell -ExecutionPolicy Bypass -File "scripts/test.ps1"
```

Expected:
- Go packages pass.
- Web Vitest suite passes.
- Web build passes.
- `npm audit` reports no vulnerabilities or only pre-existing findings explicitly unrelated to this slice.

### Step 2: Check git status and diff hygiene

Run:

```powershell
$env:GIT_MASTER='1'; git status --short
$env:GIT_MASTER='1'; git diff --check
$env:GIT_MASTER='1'; git log -6 --oneline
```

Expected:
- `git status --short` shows no uncommitted source changes except intentional ignored local fixtures.
- `git diff --check` has no output.
- Recent commits include:
  - `Add compact archive persistence`
  - `Add project session compact API`
  - `Add compact archive UI`

### Step 3: Report implementation evidence

Report these exact evidence items:

```text
Validation:
- go test ./tests/session -count=1: PASS
- web npm test -- --run: PASS
- web npm run build: PASS
- scripts/test.ps1: PASS

Commits:
- Add compact archive persistence
- Add project session compact API
- Add compact archive UI
```

---

## Self-Review Notes

- Spec coverage: Archive + summary semantics are covered by Task 1 model/persistence, Task 2 API, and Task 3 UI/history.
- Placeholder scan: This plan contains no open-ended implementation placeholders; every task has concrete files, code shape, commands, and expected results.
- Type consistency: Backend archive type is `session.CompactArchive`; frontend archive type is `CompactArchive`; API returns `archives` alongside existing `context`, `usage`, and `summaries`.
