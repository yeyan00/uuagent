import { useEffect, useMemo, useRef, useState } from 'react'
import './App.css'

interface ChatEvent { type: string; model?: string; tier?: string; text?: string; tool_name?: string; tool_id?: string }
interface Message { role: 'user' | 'assistant' | 'system' | 'tool'; content: string; model?: string; tier?: string }
interface Project { id: string; name: string; workspace_path: string; temporary: boolean }
interface AgentProfile { id: string; name: string; description?: string; system_prompt?: string; model?: string; enabled_tools?: string[]; enabled_skills?: string[]; enabled_mcp_servers?: string[]; permission_mode?: string; max_turns?: number }
interface Session { id: string; title?: string; parent_id?: string; messages: Message[] }
interface MemoryEntry { id: string; content: string; status: string; source: string; project: string; scope: string }
interface Summary { id: string; summary: string; token_before: number; token_after: number; created_at: number }

type MainPage = 'projects' | 'extensions' | 'schedules' | 'settings'
type ProjectTab = 'sessions' | 'memory' | 'context'

const navItems: Array<{ id: MainPage; label: string; icon: string }> = [
  { id: 'projects', label: 'Projects', icon: 'P' },
  { id: 'extensions', label: 'Extensions', icon: 'X' },
  { id: 'schedules', label: 'Schedules', icon: 'Q' },
  { id: 'settings', label: 'Settings', icon: 'S' },
]

const projectTabs: Array<{ id: ProjectTab; label: string }> = [
  { id: 'sessions', label: 'Sessions' },
  { id: 'memory', label: 'Memory' },
  { id: 'context', label: 'Context' },
]

const settingsTabs = ['Agents', 'Subagents', 'Models', 'Skills', 'MCP', 'Permissions', 'Storage']

const api = async <T,>(url: string, init?: RequestInit): Promise<T> => {
  const res = await fetch(url, init)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

const joinList = (value?: string[]) => (value || []).join(', ')
const parseList = (value: string) => value.split(',').map(x => x.trim()).filter(Boolean)

function App() {
  const [mainPage, setMainPage] = useState<MainPage>('projects')
  const [projectTab, setProjectTab] = useState<ProjectTab>('sessions')
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [routeInfo, setRouteInfo] = useState<{ model: string; tier: string } | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [agents, setAgents] = useState<AgentProfile[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [memories, setMemories] = useState<MemoryEntry[]>([])
  const [summaries, setSummaries] = useState<Summary[]>([])
  const [projectId, setProjectId] = useState('')
  const [agentId, setAgentId] = useState('default')
  const [sessionId, setSessionId] = useState('default')
  const [modelOverride, setModelOverride] = useState('auto')
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectPath, setNewProjectPath] = useState('')
  const [memoryText, setMemoryText] = useState('')
  const [status, setStatus] = useState('Ready')
  const [agentDraft, setAgentDraft] = useState<AgentProfile | null>(null)
  const [isAgentSettingsOpen, setAgentSettingsOpen] = useState(false)
  const [attachmentNotice, setAttachmentNotice] = useState('')
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const activeProject = projects.find(p => p.id === projectId)
  const activeAgent = agents.find(a => a.id === agentId)
  const activeSession = sessions.find(s => s.id === sessionId)
  const availableModels = useMemo(() => ['auto', ...Array.from(new Set(agents.map(a => a.model).filter(Boolean) as string[]))], [agents])

  const refresh = async () => {
    const [p, a, s, m] = await Promise.all([
      api<{projects: Project[]}>('/api/projects'),
      api<{agents: AgentProfile[]}>('/api/agents'),
      api<{sessions: Session[]}>('/api/sessions'),
      api<{memories: MemoryEntry[]}>('/api/memory'),
    ])
    setProjects(p.projects || [])
    setAgents(a.agents || [])
    setSessions(s.sessions || [])
    setMemories(m.memories || [])
  }

  useEffect(() => { refresh().catch(err => setStatus(String(err))) }, [])
  useEffect(() => { messagesEndRef.current?.scrollIntoView?.({ behavior: 'smooth' }) }, [messages])
  useEffect(() => {
    api<{summaries: Summary[]}>(`/api/sessions/${encodeURIComponent(sessionId)}/summaries`)
      .then(r => setSummaries(r.summaries || [])).catch(() => setSummaries([]))
  }, [sessionId, isStreaming])
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

  const createSession = () => { const id = `s-${Date.now()}`; setSessionId(id); setMessages([]); setSummaries([]); setStatus(`New session ${id}`) }
  const loadSession = async (id: string) => {
    setSessionId(id)
    const s = await api<Session>(`/api/sessions/${encodeURIComponent(id)}`)
    setMessages(s.messages || [])
    setStatus(`Loaded ${s.title || s.id}`)
  }
  const forkSession = async () => { const child = await api<Session>(`/api/sessions/${encodeURIComponent(sessionId)}/fork`, { method: 'POST' }); setSessionId(child.id); setMessages(child.messages || []); setStatus(`Forked ${child.id}`); await refresh() }
  const renameSession = async () => {
    const title = prompt('Session title?', activeSession?.title || sessionId)
    if (title == null) return
    const s = await api<Session>(`/api/sessions/${encodeURIComponent(sessionId)}`, { method:'PATCH', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ title }) })
    setStatus(`Renamed session ${s.title || s.id}`); await refresh()
  }
  const deleteSession = async () => {
    if (!confirm(`Delete session ${sessionId}?`)) return
    await api(`/api/sessions/${encodeURIComponent(sessionId)}`, { method:'DELETE' })
    setSessionId('default'); setMessages([]); setStatus(`Deleted session ${sessionId}`); await refresh()
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
    await api('/api/memory', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ content: memoryText, status: 'confirmed', source: 'user', scope: 'project' }) })
    setMemoryText(''); const m = await api<{memories: MemoryEntry[]}>('/api/memory'); setMemories(m.memories || []); setStatus('Memory saved')
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
    setMessages(prev => [...prev, { role: 'user', content: prompt }])
    setInput(''); setIsStreaming(true); setRouteInfo(null); setStatus('Thinking...')
    try {
      const url = `/api/chat?prompt=${encodeURIComponent(prompt)}&session_id=${encodeURIComponent(sessionId)}&agent_id=${encodeURIComponent(agentId)}`
      const response = await fetch(url)
      const reader = response.body?.getReader(); const decoder = new TextDecoder(); if (!reader) return
      let assistantContent = ''; let currentModel = ''
      while (true) {
        const { done, value } = await reader.read(); if (done) break
        const lines = decoder.decode(value).split('\n')
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          try {
            const evt: ChatEvent = JSON.parse(line.slice(6))
            if (evt.type === 'route') { currentModel = evt.model || ''; setRouteInfo({ model: evt.model || '', tier: evt.tier || '' }) }
            else if (evt.type === 'content') {
              assistantContent += evt.text || ''
              setMessages(prev => { const updated = [...prev]; const last = updated[updated.length - 1]; if (last?.role === 'assistant') updated[updated.length - 1] = { ...last, content: assistantContent }; else updated.push({ role: 'assistant', content: assistantContent, model: currentModel }); return updated })
            } else if (evt.type === 'tool_result') setStatus(`Tool result: ${evt.tool_name || evt.tool_id || ''}`)
            else if (evt.type === 'status') setStatus(evt.text || evt.type)
            else if (evt.type === 'error') setMessages(prev => [...prev, { role: 'system', content: `Error: ${evt.text}` }])
          } catch { /* skip malformed stream line */ }
        }
      }
      await refresh(); setStatus('Done')
    } catch (err) { setMessages(prev => [...prev, { role: 'system', content: `Error: ${err}` }]); setStatus(String(err)) }
    finally { setIsStreaming(false) }
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

    if (projectTab === 'sessions') return <>
      <div className="sideActions"><button onClick={createSession}>New</button><button onClick={forkSession}>Fork</button></div>
      <div className="sideActions"><button onClick={renameSession}>Rename</button><button onClick={deleteSession}>Delete</button></div>
      <div className="itemList">
        {[sessionId, ...sessions.map(s=>s.id)].filter((v,i,a)=>v&&a.indexOf(v)===i).map(id => {
          const s = sessions.find(x => x.id === id)
          return <button key={id} className={id === sessionId ? 'listItem active' : 'listItem'} onClick={() => loadSession(id).catch(err=>setStatus(String(err)))}><span>{s?.title || id}</span><small>{s?.messages?.length || 0} messages</small></button>
        })}
      </div>
    </>

    if (projectTab === 'agents') return <>
      <div className="sideActions"><button onClick={newAgent}>New Agent</button><button onClick={()=>setAgentSettingsOpen(true)}>Edit</button></div>
      <div className="itemList">
        {agents.map(a => <button key={a.id} className={a.id === agentId ? 'listItem active' : 'listItem'} onClick={() => selectAgent(a.id)}><span>{a.name || a.id}</span><small>{a.description || a.id}</small></button>)}
      </div>
    </>

    if (projectTab === 'memory') return <>
      <textarea className="sideTextarea" value={memoryText} onChange={e=>setMemoryText(e.target.value)} placeholder="Add confirmed memory..." />
      <button className="primaryButton" onClick={addMemory}>Add Memory</button>
      <div className="itemList memoryItems">{memories.map(m => <div key={m.id} className="listItem static"><span>{m.content}</span><small>{m.status} · {m.scope}</small></div>)}</div>
    </>

    if (projectTab === 'context') return <>
      {summaries.length === 0 && <div className="emptyPanel">No compression summaries yet.</div>}
      {summaries.map(s => <details key={s.id} className="summaryCard"><summary>{s.token_before} → {s.token_after}</summary><pre>{s.summary}</pre></details>)}
    </>

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
            <select value={projectId} onChange={e => openProject(e.target.value)}><option value="">Select project</option>{projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select>
            <input value={newProjectName} onChange={e=>setNewProjectName(e.target.value)} placeholder="Project name" />
            <input value={newProjectPath} onChange={e=>setNewProjectPath(e.target.value)} placeholder="Workspace path optional" />
            <button onClick={createProject}>Create</button>
          </div>
          <div className="tabGrid">{projectTabs.map(tab => <button key={tab.id} className={projectTab === tab.id ? 'tab active' : 'tab'} onClick={() => setProjectTab(tab.id)}>{tab.label}</button>)}</div>
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
          {messages.map((msg,i)=><div key={i} className={`messageBubble ${msg.role}`}><div className="messageMeta">{msg.role==='user'?'You':msg.role==='system'?'System':msg.role==='tool'?'Tool':msg.model || 'Assistant'}</div><pre>{msg.content}</pre></div>)}
          <div ref={messagesEndRef}/>
        </section>

        <footer className="composerShell">
          {attachmentNotice && <div className="attachmentNotice">{attachmentNotice}</div>}
          <div className="composerBox">
            <textarea value={input} onChange={e=>setInput(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && (e.ctrlKey || e.metaKey)){ e.preventDefault(); sendMessage() } }} placeholder="Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send" />
            <button className="attachButton" onClick={onAttachClick} title="Attach image or file">＋</button>
            <button className="sendButton" onClick={sendMessage} disabled={isStreaming || !input.trim()}>{isStreaming?'Working':'Send'}</button>
            <input ref={fileInputRef} type="file" multiple accept="image/*,.txt,.md,.json,.yaml,.yml,.go,.ts,.tsx" hidden onChange={e=>onFilesSelected(e.target.files)} />
          </div>
          <div className="composerMeta">
            <label>Project<select value={projectId} onChange={e=>openProject(e.target.value)}><option value="">None</option>{projects.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
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
  </div>
}

export default App
