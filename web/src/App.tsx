import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import './App.css'

interface ChatEvent { type: string; run_id?: string; model?: string; tier?: string; text?: string; tool_name?: string; tool_id?: string }
interface Message { role: 'user' | 'assistant' | 'system' | 'tool'; content: string; model?: string; tier?: string }
interface Project { id: string; name: string; workspace_path: string; temporary: boolean }
interface AgentProfile { id: string; name: string; description?: string; system_prompt?: string; model?: string; enabled_tools?: string[]; enabled_skills?: string[]; enabled_mcp_servers?: string[]; permission_mode?: string; max_turns?: number }
interface Session { id: string; title?: string; project_id?: string; project_path?: string; parent_id?: string; messages: Message[] }
interface MemoryEntry { id: string; content: string; status: string; source: string; project: string; scope: string }
interface Summary { id: string; summary: string; token_before: number; token_after: number; created_at: number }
interface ContextStats { estimated_tokens: number; max_tokens: number; percent: number }
interface TokenUsage { input_tokens?: number; output_tokens?: number; total_tokens?: number; estimated_input_tokens?: number; estimated_output_tokens?: number; estimated?: boolean }
interface SessionContext { context?: ContextStats; usage?: TokenUsage; summaries?: Summary[] }
interface ApprovalPayload { approval_required?: boolean; tool?: string; path?: string; reason?: string; run_id?: string; status?: string }
interface ToolEventPayload { kind?: 'tool_start' | 'tool_result'; tool?: string; tool_id?: string; text?: string }

type MainPage = 'projects' | 'extensions' | 'schedules' | 'settings'
type ProjectSettingsTab = 'memory' | 'context' | 'config'

const navItems: Array<{ id: MainPage; label: string; icon: string }> = [
  { id: 'projects', label: 'Projects', icon: 'P' },
  { id: 'extensions', label: 'Extensions', icon: 'X' },
  { id: 'schedules', label: 'Schedules', icon: 'Q' },
  { id: 'settings', label: 'Settings', icon: 'S' },
]

const settingsTabs = ['Agents', 'Subagents', 'Models', 'Skills', 'MCP', 'Permissions', 'Storage']

const api = async <T,>(url: string, init?: RequestInit): Promise<T> => {
  const res = await fetch(url, init)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

const joinList = (value?: string[]) => (value || []).join(', ')
const parseList = (value: string) => value.split(',').map(x => x.trim()).filter(Boolean)
const parseApprovalPayload = (text?: string): ApprovalPayload | null => {
  if (!text) return null
  try {
    const payload = JSON.parse(text) as ApprovalPayload
    return payload?.approval_required ? payload : null
  } catch {
    return null
  }
}

const parseToolEventPayload = (text?: string): ToolEventPayload | null => {
  if (!text) return null
  try {
    const payload = JSON.parse(text) as ToolEventPayload
    return payload?.kind === 'tool_start' || payload?.kind === 'tool_result' ? payload : null
  } catch {
    return null
  }
}

const toolPreview = (text?: string) => {
  const value = (text || '').trim()
  return value.length > 800 ? `${value.slice(0, 800)}...` : value
}

const renderInlineMarkdown = (text: string): ReactNode[] => {
  const nodes: ReactNode[] = []
  const pattern = /(\*\*[^*]+\*\*|`[^`]+`)/g
  let last = 0
  for (const match of text.matchAll(pattern)) {
    if (match.index === undefined) continue
    if (match.index > last) nodes.push(text.slice(last, match.index))
    const token = match[0]
    if (token.startsWith('**')) nodes.push(<strong key={`${match.index}-b`}>{token.slice(2, -2)}</strong>)
    else nodes.push(<code key={`${match.index}-c`}>{token.slice(1, -1)}</code>)
    last = match.index + token.length
  }
  if (last < text.length) nodes.push(text.slice(last))
  return nodes
}

const renderMarkdown = (content: unknown) => {
  const text = typeof content === 'string' ? content : JSON.stringify(content, null, 2)
  const blocks = text.split(/\n{2,}/)
  return <div className="markdownBody">{blocks.map((block, i) => {
    const trimmed = block.trimEnd()
    if (!trimmed) return null
    if (trimmed.startsWith('```')) {
      const code = trimmed.replace(/^```[^\n]*\n?/, '').replace(/\n?```$/, '')
      return <pre key={i} className="codeBlock"><code>{code}</code></pre>
    }
    const lines = trimmed.split('\n')
    if (lines.every(line => /^\s*[-*]\s+/.test(line))) {
      return <ul key={i}>{lines.map((line, j) => <li key={j}>{renderInlineMarkdown(line.replace(/^\s*[-*]\s+/, ''))}</li>)}</ul>
    }
    if (/^#{1,3}\s+/.test(trimmed)) {
      const level = Math.min(3, trimmed.match(/^#+/)?.[0].length || 2)
      const body = trimmed.replace(/^#{1,3}\s+/, '')
      return level === 1 ? <h1 key={i}>{renderInlineMarkdown(body)}</h1> : level === 2 ? <h2 key={i}>{renderInlineMarkdown(body)}</h2> : <h3 key={i}>{renderInlineMarkdown(body)}</h3>
    }
    return <p key={i}>{lines.map((line, j) => <span key={j}>{j > 0 && <br />}{renderInlineMarkdown(line)}</span>)}</p>
  })}</div>
}

const renderApprovalCard = (approval: ApprovalPayload, onApprove: (runID: string) => void, onDeny: (runID: string) => void) => {
  const tool = approval.tool || 'Tool'
  const acted = approval.status === 'approved' || approval.status === 'denied'
  return <div className="approvalCard">
    <div className="approvalBadge">{approval.status === 'approved' ? 'Approved' : approval.status === 'denied' ? 'Denied' : 'Approval required'}</div>
    <h3>{tool} wants access</h3>
    <div className="approvalPath">{approval.path || 'Protected path'}</div>
    <p>{approval.reason || 'This action requires approval.'}</p>
    <div className="approvalActions">
      <button className="primaryButton" disabled={acted || !approval.run_id} onClick={() => approval.run_id && onApprove(approval.run_id)}>Approve</button>
      <button disabled={acted || !approval.run_id} onClick={() => approval.run_id && onDeny(approval.run_id)}>Deny</button>
    </div>
  </div>
}

const renderToolEventCard = (event: ToolEventPayload) => <div className="toolEventCard">
  <div className="toolEventBadge">{event.kind === 'tool_start' ? 'Tool running' : 'Tool result'}</div>
  <h3>{event.kind === 'tool_start' ? `Running ${event.tool || 'tool'}` : `${event.tool || 'Tool'} result`}</h3>
  {event.text && <pre>{toolPreview(event.text)}</pre>}
</div>

function App() {
  const [mainPage, setMainPage] = useState<MainPage>('projects')
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [routeInfo, setRouteInfo] = useState<{ model: string; tier: string } | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [agents, setAgents] = useState<AgentProfile[]>([])
  const [projectSessions, setProjectSessions] = useState<Record<string, Session[]>>({})
  const [memories, setMemories] = useState<MemoryEntry[]>([])
  const [summaries, setSummaries] = useState<Summary[]>([])
  const [sessionContext, setSessionContext] = useState<SessionContext>({})
  const [projectId, setProjectId] = useState('')
  const [agentId, setAgentId] = useState('default')
  const [sessionId, setSessionId] = useState('default')
  const [modelOverride, setModelOverride] = useState('auto')
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectPath, setNewProjectPath] = useState('')
  const [memoryText, setMemoryText] = useState('')
  const [status, setStatus] = useState('Ready')
  const [currentRunId, setCurrentRunId] = useState('')
  const [agentDraft, setAgentDraft] = useState<AgentProfile | null>(null)
  const [isAgentSettingsOpen, setAgentSettingsOpen] = useState(false)
  const [settingsProjectId, setSettingsProjectId] = useState('')
  const [projectSettingsTab, setProjectSettingsTab] = useState<ProjectSettingsTab>('memory')
  const [attachmentNotice, setAttachmentNotice] = useState('')
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  const activeProject = projects.find(p => p.id === projectId)
  const activeAgent = agents.find(a => a.id === agentId)
  const activeProjectSessions = projectId ? (projectSessions[projectId] || []) : []
  const activeSession = activeProjectSessions.find(s => s.id === sessionId)
  const activeSessionLocked = !!activeSession && ((activeSession.messages?.length || 0) > 0 || !!activeSession.project_id)
  const availableModels = useMemo(() => ['auto', ...Array.from(new Set(agents.map(a => a.model).filter(Boolean) as string[]))], [agents])
  const memoryURL = projectId ? `/api/memory?project=${encodeURIComponent(projectId)}` : '/api/memory'
  const formatTokens = (n?: number) => {
    const value = n || 0
    if (value >= 1000) return `${Number((value / 1000).toFixed(1))}k`
    return `${value}`
  }

  const refresh = async () => {
    const [p, a, m] = await Promise.all([
      api<{projects: Project[]}>('/api/projects'),
      api<{agents: AgentProfile[]}>('/api/agents'),
      api<{memories: MemoryEntry[]}>(memoryURL),
    ])
    const projectList = p.projects || []
    setProjects(projectList)
    setAgents(a.agents || [])
    setMemories(m.memories || [])
    const pairs = await Promise.all(projectList.map(async p => {
      const r = await api<{sessions: Session[]}>(`/api/projects/${encodeURIComponent(p.id)}/sessions`).catch(() => ({ sessions: [] }))
      return [p.id, r.sessions || []] as const
    }))
    setProjectSessions(Object.fromEntries(pairs))
  }

  useEffect(() => { refresh().catch(err => setStatus(String(err))) }, [])
  useEffect(() => { api<{memories: MemoryEntry[]}>(memoryURL).then(m => setMemories(m.memories || [])).catch(err => setStatus(String(err))) }, [memoryURL])
  useEffect(() => { messagesEndRef.current?.scrollIntoView?.({ behavior: 'smooth' }) }, [messages])
  useEffect(() => {
    if (!projectId || !sessionId) return
    api<SessionContext>(`/api/projects/${encodeURIComponent(projectId)}/sessions/${encodeURIComponent(sessionId)}/context`)
      .then(r => { setSessionContext(r || {}); setSummaries(r.summaries || []) }).catch(() => { setSessionContext({}); setSummaries([]) })
  }, [projectId, sessionId, isStreaming])
  useEffect(() => {
    const profile = agents.find(a => a.id === agentId)
    setAgentDraft(profile ? { ...profile } : null)
  }, [agents, agentId])

  const createProject = async () => {
    const p = await api<Project>('/api/projects', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ name: newProjectName || 'Untitled', workspace_path: newProjectPath }) })
    setProjectId(p.id); setNewProjectName(''); setNewProjectPath(''); setStatus(`Opened ${p.name}`); await refresh()
  }

  const openProject = async (id: string) => {
    if (!id) return
    const r = await api<any>(`/api/projects/${encodeURIComponent(id)}/open`, { method: 'POST' })
    setProjectId(id); setStatus(`Opened project. Config: ${(r.config_sources || []).join(', ') || 'default'}`); await refresh()
  }

  const createSession = async (pid = projectId) => {
    if (!pid) { setStatus('Select a project first'); return }
    const id = `s-${Date.now()}`
    const s = await api<Session>(`/api/projects/${encodeURIComponent(pid)}/sessions`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ id }) })
    setProjectId(pid); setSessionId(s.id); setMessages([]); setSummaries([]); setStatus(`New session ${s.id}`); await refresh()
  }
  const loadSession = async (id: string, pid = projectId) => {
    if (!pid) return
    setProjectId(pid)
    setSessionId(id)
    const s = await api<Session>(`/api/projects/${encodeURIComponent(pid)}/sessions/${encodeURIComponent(id)}`)
    setMessages(s.messages || [])
    const ctx = await api<SessionContext>(`/api/projects/${encodeURIComponent(pid)}/sessions/${encodeURIComponent(id)}/context`).catch((): SessionContext => ({}))
    setSessionContext(ctx); setSummaries(ctx.summaries || [])
    setStatus(`Loaded ${s.title || s.id}`)
  }
  const forkSession = async (id = sessionId, pid = projectId) => {
    if (!pid || !id) return
    const child = await api<Session>(`/api/projects/${encodeURIComponent(pid)}/sessions/${encodeURIComponent(id)}/fork`, { method: 'POST' })
    setProjectId(pid); setSessionId(child.id); setMessages(child.messages || []); setStatus(`Forked ${child.id}`); await refresh()
  }
  const renameSession = async (id = sessionId, pid = projectId) => {
    if (!pid || !id) return
    const current = (projectSessions[pid] || []).find(s => s.id === id)
    const title = prompt('Session title?', current?.title || id)
    if (title == null) return
    const s = await api<Session>(`/api/projects/${encodeURIComponent(pid)}/sessions/${encodeURIComponent(id)}`, { method:'PATCH', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ title }) })
    setStatus(`Renamed session ${s.title || s.id}`); await refresh()
  }
  const deleteSession = async (id = sessionId, pid = projectId) => {
    if (!pid || !id || !confirm(`Delete session ${id}?`)) return
    await api(`/api/projects/${encodeURIComponent(pid)}/sessions/${encodeURIComponent(id)}`, { method:'DELETE' })
    if (id === sessionId) { setSessionId(''); setMessages([]) }
    setStatus(`Deleted session ${id}`); await refresh()
  }

  const selectAgent = (id: string) => {
    setAgentId(id)
    const profile = agents.find(a => a.id === id)
    setAgentDraft(profile ? { ...profile } : null)
    if (profile?.model) setModelOverride(profile.model)
  }

  const updateAgentDraft = (patch: Partial<AgentProfile>) => setAgentDraft(prev => ({ ...(prev || { id: `agent-${Date.now()}`, name: 'New Agent' }), ...patch }))
  const newAgent = () => { setAgentDraft({ id:`agent-${Date.now()}`, name:'New Agent', enabled_tools:['read','list_dir'], enabled_mcp_servers:['mock'], permission_mode:'workspace-write', max_turns:20 }); setAgentSettingsOpen(true) }
  const saveAgent = async () => {
    if (!agentDraft) return
    const saved = await api<AgentProfile>('/api/agents', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(agentDraft) })
    setAgentId(saved.id); setAgentDraft(saved); setStatus(`Saved agent ${saved.id}`); await refresh()
  }
  const cloneAgent = async () => {
    const id = prompt('New agent id?') || ''
    if (!id) return
    const name = prompt('New agent name?', `${agentDraft?.name || agentId} Copy`) || id
    const cloned = await api<AgentProfile>(`/api/agents/${encodeURIComponent(agentId)}/clone`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ id, name }) })
    setAgentId(cloned.id); setAgentDraft(cloned); setStatus(`Cloned agent ${cloned.id}`); await refresh()
  }
  const deleteAgent = async () => {
    if (!agentId || agentId === 'default' || !confirm(`Delete agent ${agentId}?`)) return
    await api(`/api/agents/${encodeURIComponent(agentId)}`, { method:'DELETE' })
    setAgentId('default'); setAgentSettingsOpen(false); setStatus(`Deleted agent ${agentId}`); await refresh()
  }

  const addMemory = async () => {
    if (!memoryText.trim()) return
    await api('/api/memory', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ content: memoryText, project: projectId, status: 'confirmed', source: 'user', scope: 'project' }) })
    setMemoryText(''); const m = await api<{memories: MemoryEntry[]}>(memoryURL); setMemories(m.memories || []); setStatus('Memory saved')
  }

  const updateApprovalStatus = (runID: string, approvalStatus: 'approved' | 'denied') => {
    setMessages(prev => prev.map(msg => {
      const approval = msg.role === 'system' ? parseApprovalPayload(String(msg.content || '')) : null
      if (!approval || approval.run_id !== runID) return msg
      return { ...msg, content: JSON.stringify({ ...approval, status: approvalStatus }) }
    }))
  }

  const appendToolEvent = (kind: 'tool_start' | 'tool_result', evt: ChatEvent) => {
    const payload: ToolEventPayload = { kind, tool: evt.tool_name || evt.tool_id || 'tool', tool_id: evt.tool_id, text: evt.text }
    setMessages(prev => [...prev, { role: 'tool', content: JSON.stringify(payload) }])
  }

  const appendAssistantContent = (text: string, model = routeInfo?.model || '') => {
    setMessages(prev => {
      const updated = [...prev]
      const last = updated[updated.length - 1]
      if (last?.role === 'assistant') updated[updated.length - 1] = { ...last, content: `${last.content}${text}` }
      else updated.push({ role: 'assistant', content: text, model })
      return updated
    })
  }

  const consumeEventStream = async (response: Response, options: { streamRunId?: string; model?: string; approvalFinalStatus?: string } = {}) => {
    const reader = response.body?.getReader()
    const decoder = new TextDecoder()
    let streamRunId = options.streamRunId || ''
    let approvalPending = false
    if (!reader) return { approvalPending }
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      const lines = decoder.decode(value).split('\n')
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        try {
          const evt: ChatEvent = JSON.parse(line.slice(6))
          if (evt.type === 'run') { streamRunId = evt.run_id || ''; setCurrentRunId(streamRunId) }
          else if (evt.type === 'route') { setRouteInfo({ model: evt.model || '', tier: evt.tier || '' }) }
          else if (evt.type === 'content') appendAssistantContent(evt.text || '', evt.model || options.model || routeInfo?.model || '')
          else if (evt.type === 'tool_start') { appendToolEvent('tool_start', evt); setStatus(`Running ${evt.tool_name || evt.tool_id || 'tool'}`) }
          else if (evt.type === 'tool_result') {
            const approval = parseApprovalPayload(evt.text)
            if (approval) {
              approvalPending = true
              const payload = { ...approval, tool: approval.tool || evt.tool_name || evt.tool_id || 'Tool', run_id: evt.run_id || streamRunId }
              setMessages(prev => [...prev, { role: 'system', content: JSON.stringify(payload) }])
              setStatus('Approval required')
            } else {
              appendToolEvent('tool_result', evt)
              setStatus(`Tool result: ${evt.tool_name || evt.tool_id || ''}`)
            }
          }
          else if (evt.type === 'status') setStatus(evt.text || evt.type)
          else if (evt.type === 'error') setMessages(prev => [...prev, { role: 'system', content: `Error: ${evt.text}` }])
          else if (evt.type === 'done') setStatus(options.approvalFinalStatus || 'Done')
        } catch { /* skip malformed stream line */ }
      }
    }
    return { approvalPending }
  }

  const approveRun = async (runID: string) => {
    setStatus('Approving...')
    updateApprovalStatus(runID, 'approved')
    const response = await fetch(`/api/runs/${encodeURIComponent(runID)}/approve/stream`, { method: 'POST' })
    if (!response.ok) throw new Error(await response.text())
    await consumeEventStream(response, { streamRunId: runID, model: routeInfo?.model, approvalFinalStatus: 'Approved' })
    await refresh()
  }

  const denyRun = async (runID: string) => {
    setStatus('Denying...')
    await api(`/api/runs/${encodeURIComponent(runID)}/deny`, { method: 'POST' })
    updateApprovalStatus(runID, 'denied')
    setStatus('Denied')
    await refresh()
  }

  const onAttachClick = () => fileInputRef.current?.click()
  const onFilesSelected = (files: FileList | null) => {
    if (!files || files.length === 0) return
    const names = Array.from(files).map(f => f.name).join(', ')
    setAttachmentNotice(`Selected: ${names}. Attachment upload will be wired to chat next.`)
  }

  const sendMessage = async () => {
    if (!input.trim() || isStreaming) return
    const prompt = input
    const controller = new AbortController()
    abortRef.current = controller
    setMessages(prev => [...prev, { role: 'user', content: prompt }])
    setInput(''); setIsStreaming(true); setCurrentRunId(''); setRouteInfo(null); setStatus('Thinking...')
    try {
      const sid = sessionId || `s-${Date.now()}`
      if (!sessionId) setSessionId(sid)
      const url = `/api/chat?prompt=${encodeURIComponent(prompt)}&session_id=${encodeURIComponent(sid)}&agent_id=${encodeURIComponent(agentId)}${projectId ? `&project_id=${encodeURIComponent(projectId)}` : ''}`
      const response = await fetch(url, { signal: controller.signal })
      const { approvalPending } = await consumeEventStream(response)
      await refresh(); setStatus(approvalPending ? 'Approval required' : 'Done')
    } catch (err) {
      if ((err as Error)?.name === 'AbortError') setStatus('Stopped')
      else { setMessages(prev => [...prev, { role: 'system', content: `Error: ${err}` }]); setStatus(String(err)) }
    }
    finally { setIsStreaming(false); setCurrentRunId(''); abortRef.current = null }
  }

  const stopRun = async () => {
    const runId = currentRunId
    if (runId) await fetch(`/api/runs/${encodeURIComponent(runId)}/stop`, { method: 'POST' }).catch(() => undefined)
    abortRef.current?.abort()
    setStatus('Stopping...')
  }

  const renderSidebarBody = () => {
    if (mainPage !== 'projects') {
      if (mainPage === 'settings') return <>
        <div className="itemList">
          {settingsTabs.map(tab => <button key={tab} className={tab === 'Agents' ? 'listItem active' : 'listItem'} onClick={() => tab === 'Agents' ? setAgentSettingsOpen(true) : setStatus(`${tab} settings are planned`)}><span>{tab}</span><small>{tab === 'Agents' ? 'Prompts, models, tools and MCP access' : 'Coming soon'}</small></button>)}
        </div>
      </>
      const text = mainPage === 'extensions'
        ? 'Manage marketplace extensions and installable capability packs.'
        : 'Create recurring project scans, tests, reports and knowledge refresh jobs.'
      return <div className="sidePlaceholder"><h3>{navItems.find(n => n.id === mainPage)?.label}</h3><p>{text}</p><button className="softButton">Coming soon</button></div>
    }

    return <div className="projectDrawerList">
      {projects.length === 0 && <div className="emptyPanel">No projects yet.</div>}
      {projects.map(p => {
        const list = projectSessions[p.id] || []
        const expanded = p.id === projectId || list.length > 0
        return <section key={p.id} className="projectDrawer">
          <div className={p.id === projectId ? 'projectDrawerHead active' : 'projectDrawerHead'}>
            <button className="projectToggle" onClick={() => openProject(p.id).catch(err=>setStatus(String(err)))}><span>{expanded ? '▾' : '▸'}</span><strong>{p.name}</strong><small>{p.workspace_path}</small></button>
            <button className="miniButton" title="New session" onClick={() => createSession(p.id).catch(err=>setStatus(String(err)))}>＋</button>
            <button className="miniButton" title="Project settings" onClick={() => { setProjectId(p.id); setSettingsProjectId(p.id); setProjectSettingsTab('memory') }}>⚙</button>
          </div>
          {expanded && <div className="sessionList">
            {list.length === 0 && <div className="emptyMini">No sessions</div>}
            {list.map(s => <div key={s.id} className={s.id === sessionId ? 'sessionRow active' : 'sessionRow'}>
              <button className="sessionTitle" onClick={() => loadSession(s.id, p.id).catch(err=>setStatus(String(err)))}><span>{s.title || s.id}</span><small>{s.messages?.length || 0} messages</small></button>
              <button className="miniButton" title="Fork" onClick={() => forkSession(s.id, p.id).catch(err=>setStatus(String(err)))}>⎇</button>
              <button className="miniButton" title="Rename" onClick={() => renameSession(s.id, p.id).catch(err=>setStatus(String(err)))}>✎</button>
              <button className="miniButton" title="Delete" onClick={() => deleteSession(s.id, p.id).catch(err=>setStatus(String(err)))}>×</button>
            </div>)}
          </div>}
        </section>
      })}
    </div>

  }

  return <div className="appDesktop">
    <div className="appWindow">
      <aside className="navRail">
        <div className="brandBlock"><div className="brandMark">U</div><div className="brandMini">UA</div></div>
        <nav className="navList">
          {navItems.map(item => <button key={item.id} className={mainPage === item.id ? 'navItem active' : 'navItem'} onClick={() => setMainPage(item.id)} title={item.label}><span className="navIcon">{item.icon}</span><span>{item.label}</span></button>)}
        </nav>
      </aside>

      <aside className="contextSidebar">
        <div className="sidebarHeader">
          <h2>{mainPage === 'projects' ? 'Project' : navItems.find(n => n.id === mainPage)?.label}</h2>
          <p>{activeProject?.name || 'No project selected'}</p>
        </div>
        {mainPage === 'projects' && <>
          <div className="projectPicker">
            <input value={newProjectName} onChange={e=>setNewProjectName(e.target.value)} placeholder="Project name" />
            <input value={newProjectPath} onChange={e=>setNewProjectPath(e.target.value)} placeholder="Workspace path optional" />
            <button onClick={createProject}>Create</button>
          </div>
        </>}
        <div className="sidebarBody">{renderSidebarBody()}</div>
        <div className="statusStrip"><span className="pulse" />{status}</div>
      </aside>

      <main className="workspace">
        <header className="workspaceHeader">
          <div><h1>{activeSession?.title || sessionId}</h1><p>{activeProject?.workspace_path || 'Local workspace'} · {activeAgent?.name || agentId}</p></div>
          {routeInfo && <div className="routePill">{routeInfo.model}<span>{routeInfo.tier}</span></div>}
        </header>

        <section className="messagesPane">
          {messages.length === 0 && <div className="emptyState"><div>✨</div><h2>Start a coding session</h2><p>Open a project, choose an agent and ask UUAgent to inspect, explain or modify your code.</p></div>}
          {messages.map((msg,i)=>{
            const approval = msg.role === 'system' ? parseApprovalPayload(String(msg.content || '')) : null
            const toolEvent = msg.role === 'tool' ? parseToolEventPayload(String(msg.content || '')) : null
            return <div key={i} className={`messageBubble ${msg.role}`}><div className="messageMeta">{msg.role==='user'?'You':msg.role==='system'?'System':msg.role==='tool'?'Tool':msg.model || 'Assistant'}</div>{approval ? renderApprovalCard(approval, approveRun, denyRun) : toolEvent ? renderToolEventCard(toolEvent) : renderMarkdown(msg.content)}</div>
          })}
          <div ref={messagesEndRef}/>
        </section>

        <footer className="composerShell">
          {attachmentNotice && <div className="attachmentNotice">{attachmentNotice}</div>}
          <div className="composerBox">
            <textarea value={input} onChange={e=>setInput(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && (e.ctrlKey || e.metaKey)){ e.preventDefault(); sendMessage() } }} placeholder="Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send" />
            <button className="attachButton" onClick={onAttachClick} title="Attach image or file">＋</button>
            {isStreaming ? <button className="sendButton" onClick={stopRun}>Stop</button> : <button className="sendButton" onClick={sendMessage} disabled={!input.trim()}>Send</button>}
            <input ref={fileInputRef} type="file" multiple accept="image/*,.txt,.md,.json,.yaml,.yml,.go,.ts,.tsx" hidden onChange={e=>onFilesSelected(e.target.files)} />
          </div>
          <div className="composerMeta">
            <label>Project<select value={projectId} onChange={e=>openProject(e.target.value)} disabled={activeSessionLocked}><option value="">None</option>{projects.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select>{activeSessionLocked && <span>locked</span>}</label>
            <label>Agent<select value={agentId} onChange={e=>selectAgent(e.target.value)}>{agents.map(a=><option key={a.id} value={a.id}>{a.name || a.id}</option>)}</select></label>
            <label>Model<select value={modelOverride} onChange={e=>setModelOverride(e.target.value)}>{availableModels.map(m=><option key={m} value={m}>{m === 'auto' ? 'Auto' : m}</option>)}</select></label>
          </div>
        </footer>
      </main>
    </div>

    {isAgentSettingsOpen && <div className="modalBackdrop" onMouseDown={()=>setAgentSettingsOpen(false)}>
      <div className="modal" onMouseDown={e=>e.stopPropagation()}>
        <div className="modalHeader"><div><h2>Agent Settings</h2><p>Configure prompt, model routing, tools, skills and MCP access.</p></div><button className="iconButton" onClick={()=>setAgentSettingsOpen(false)}>×</button></div>
        {agentDraft && <div className="settingsGrid">
          <label>ID<input value={agentDraft.id || ''} onChange={e=>updateAgentDraft({ id:e.target.value })} disabled={agentDraft.id === 'default'} /></label>
          <label>Name<input value={agentDraft.name || ''} onChange={e=>updateAgentDraft({ name:e.target.value })} /></label>
          <label className="wide">Description<input value={agentDraft.description || ''} onChange={e=>updateAgentDraft({ description:e.target.value })} /></label>
          <label className="wide">System Prompt<textarea value={agentDraft.system_prompt || ''} onChange={e=>updateAgentDraft({ system_prompt:e.target.value })} /></label>
          <label>Model<input value={agentDraft.model || ''} onChange={e=>updateAgentDraft({ model:e.target.value })} placeholder="empty = route automatically" /></label>
          <label>Permission<input value={agentDraft.permission_mode || ''} onChange={e=>updateAgentDraft({ permission_mode:e.target.value })} /></label>
          <label>Max Turns<input type="number" value={agentDraft.max_turns || 0} onChange={e=>updateAgentDraft({ max_turns:Number(e.target.value) })} /></label>
          <label className="wide">Tools<input value={joinList(agentDraft.enabled_tools)} onChange={e=>updateAgentDraft({ enabled_tools:parseList(e.target.value) })} placeholder="read, write, grep, list_dir" /></label>
          <label className="wide">Skills<input value={joinList(agentDraft.enabled_skills)} onChange={e=>updateAgentDraft({ enabled_skills:parseList(e.target.value) })} placeholder="mock-planner" /></label>
          <label className="wide">MCP Servers<input value={joinList(agentDraft.enabled_mcp_servers)} onChange={e=>updateAgentDraft({ enabled_mcp_servers:parseList(e.target.value) })} placeholder="mock" /></label>
        </div>}
        <div className="modalActions"><button onClick={newAgent}>New</button><button onClick={cloneAgent}>Clone</button><button onClick={deleteAgent} disabled={agentDraft?.id === 'default'}>Delete</button><button className="primaryButton" onClick={saveAgent}>Save Agent</button></div>
      </div>
    </div>}
    {settingsProjectId && <div className="modalBackdrop" onMouseDown={()=>setSettingsProjectId('')}>
      <div className="modal" onMouseDown={e=>e.stopPropagation()}>
        <div className="modalHeader"><div><h2>Project Settings</h2><p>{projects.find(p=>p.id===settingsProjectId)?.name || settingsProjectId}</p></div><button className="iconButton" onClick={()=>setSettingsProjectId('')}>×</button></div>
        <div className="tabGrid">{(['memory','context','config'] as ProjectSettingsTab[]).map(tab => <button key={tab} className={projectSettingsTab === tab ? 'tab active' : 'tab'} onClick={()=>setProjectSettingsTab(tab)}>{tab}</button>)}</div>
        {projectSettingsTab === 'memory' && <div className="settingsPanel"><textarea className="sideTextarea" value={memoryText} onChange={e=>setMemoryText(e.target.value)} placeholder="Add confirmed memory..." /><button className="primaryButton" onClick={addMemory}>Add Memory</button><div className="itemList memoryItems">{memories.map(m => <div key={m.id} className="listItem static"><span>{m.content}</span><small>{m.status} · {m.scope}</small></div>)}</div></div>}
        {projectSettingsTab === 'context' && <div className="settingsPanel">
          <div className="contextStats"><h3>Current Context</h3><strong>{formatTokens(sessionContext.context?.estimated_tokens)} / {formatTokens(sessionContext.context?.max_tokens)} tokens</strong><span>{Math.round((sessionContext.context?.percent || 0) * 100)}%</span></div>
          <div className="contextStats"><h3>Session Token Usage</h3><span>Input: {formatTokens(sessionContext.usage?.input_tokens || sessionContext.usage?.estimated_input_tokens)}</span><span>Output: {formatTokens(sessionContext.usage?.output_tokens || sessionContext.usage?.estimated_output_tokens)}</span><span>Total: {formatTokens(sessionContext.usage?.total_tokens)}</span></div>
          {summaries.length === 0 && <div className="emptyPanel">No compression summaries yet.</div>}
          {summaries.map(s => <details key={s.id} className="summaryCard"><summary>{formatTokens(s.token_before)} → {formatTokens(s.token_after)}</summary><pre>{s.summary}</pre></details>)}
        </div>}
        {projectSettingsTab === 'config' && <div className="settingsPanel"><div className="emptyPanel"><strong>Workspace</strong><br />{projects.find(p=>p.id===settingsProjectId)?.workspace_path || ''}</div></div>}
      </div>
    </div>}
  </div>
}

export default App
