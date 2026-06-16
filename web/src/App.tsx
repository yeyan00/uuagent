import { useEffect, useRef, useState } from 'react'
import './App.css'

interface ChatEvent { type: string; model?: string; tier?: string; text?: string; tool_name?: string; tool_id?: string }
interface Message { role: 'user' | 'assistant' | 'system' | 'tool'; content: string; model?: string; tier?: string }
interface Project { id: string; name: string; workspace_path: string; temporary: boolean }
interface AgentProfile { id: string; name: string; description?: string; system_prompt?: string; model?: string; enabled_tools?: string[]; enabled_skills?: string[]; enabled_mcp_servers?: string[]; permission_mode?: string; max_turns?: number }
interface Session { id: string; title?: string; parent_id?: string; messages: Message[] }
interface MemoryEntry { id: string; content: string; status: string; source: string; project: string; scope: string }
interface Summary { id: string; summary: string; token_before: number; token_after: number; created_at: number }

const api = async <T,>(url: string, init?: RequestInit): Promise<T> => {
  const res = await fetch(url, init)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

const joinList = (value?: string[]) => (value || []).join(', ')
const parseList = (value: string) => value.split(',').map(x => x.trim()).filter(Boolean)

function App() {
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
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectPath, setNewProjectPath] = useState('')
  const [memoryText, setMemoryText] = useState('')
  const [status, setStatus] = useState('Ready')
  const [agentDraft, setAgentDraft] = useState<AgentProfile | null>(null)
  const [isAgentSettingsOpen, setAgentSettingsOpen] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  const activeAgent = agents.find(a => a.id === agentId)
  const activeSession = sessions.find(s => s.id === sessionId)

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

  return <div className="appShell">
    <aside className="sidebar">
      <div className="brandCard">
        <div className="brandIcon">◎</div>
        <div><h1>UUAgent</h1><p>Web-first coding agent</p></div>
      </div>

      <section className="panel">
        <div className="panelTitle"><span>📁 Projects</span></div>
        <select value={projectId} onChange={e => openProject(e.target.value)}><option value=''>Select project</option>{projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select>
        <div className="compactForm">
          <input placeholder='Project name' value={newProjectName} onChange={e=>setNewProjectName(e.target.value)} />
          <input placeholder='Workspace path (optional)' value={newProjectPath} onChange={e=>setNewProjectPath(e.target.value)} />
          <button className="secondaryButton" onClick={createProject}>Create Project</button>
        </div>
      </section>

      <section className="panel">
        <div className="panelTitle"><span>💬 Sessions</span></div>
        <div className="buttonRow"><button onClick={createSession}>New</button><button onClick={forkSession}>Fork</button><button onClick={renameSession}>Rename</button><button onClick={deleteSession}>Delete</button></div>
        <select value={sessionId} onChange={e=>loadSession(e.target.value).catch(err=>setStatus(String(err)))}>{[sessionId, ...sessions.map(s=>s.id)].filter((v,i,a)=>v&&a.indexOf(v)===i).map(id=>{ const s=sessions.find(x=>x.id===id); return <option key={id} value={id}>{s?.title || id}</option> })}</select>
      </section>

      <section className="panel">
        <div className="panelTitle"><span>🤖 Agent</span><button className="iconButton" title="Agent settings" onClick={()=>setAgentSettingsOpen(true)}>⚙</button></div>
        <select value={agentId} onChange={e=>selectAgent(e.target.value)}>{agents.map(a=><option key={a.id} value={a.id}>{a.name || a.id}</option>)}</select>
        <div className="agentSummary"><b>{activeAgent?.name || agentId}</b><span>{activeAgent?.description || 'General-purpose assistant'}</span></div>
        <div className="buttonRow"><button onClick={newAgent}>New Agent</button><button onClick={()=>setAgentSettingsOpen(true)}>Settings</button></div>
      </section>

      <div className="statusCard"><span className="pulse"/> {status}</div>
    </aside>

    <main className="chatColumn">
      <header className="chatHeader">
        <div><h2>{activeSession?.title || sessionId}</h2><p>Agent: {activeAgent?.name || agentId}</p></div>
        {routeInfo && <div className="routePill">{routeInfo.model} <span>{routeInfo.tier}</span></div>}
      </header>
      <div className="messagesPane">
        {messages.length === 0 && <div className="emptyState"><div>✨</div><h3>Start a focused coding session</h3><p>Choose an agent, open a project, then ask UUAgent to inspect, edit, or explain your code.</p></div>}
        {messages.map((msg,i)=><div key={i} className={`messageBubble ${msg.role}`}><div className="messageMeta">{msg.role==='user'?'👤 You':msg.role==='system'?'⚠ System':msg.role==='tool'?'🛠 Tool':`🤖 ${msg.model || 'Assistant'}`}</div><pre>{msg.content}</pre></div>)}
        <div ref={messagesEndRef}/>
      </div>
      <div className="composer"><input value={input} onChange={e=>setInput(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter'&&!e.shiftKey){ e.preventDefault(); sendMessage() } }} placeholder='Ask UUAgent to work on your code...' /><button className="primaryButton" onClick={sendMessage} disabled={isStreaming || !input.trim()}>{isStreaming?'Working...':'Send'}</button></div>
    </main>

    <aside className="inspector">
      <section className="panel glass">
        <div className="panelTitle"><span>🧠 Memory</span></div>
        <textarea value={memoryText} onChange={e=>setMemoryText(e.target.value)} placeholder='Add a confirmed memory for future runs...' />
        <button className="secondaryButton" onClick={addMemory}>Add Memory</button>
        <div className="memoryList">{memories.slice(0, 8).map(m=><div key={m.id} className="memoryItem"><small>{m.status} · {m.scope}</small><div>{m.content}</div></div>)}</div>
      </section>
      <section className="panel glass">
        <div className="panelTitle"><span>🗜 Compression</span></div>
        {summaries.length === 0 && <p className="muted">No summaries yet.</p>}
        {summaries.map(s=><details key={s.id} className="summaryItem"><summary>{s.token_before} → {s.token_after}</summary><pre>{s.summary}</pre></details>)}
      </section>
    </aside>

    {isAgentSettingsOpen && <div className="modalBackdrop" onMouseDown={()=>setAgentSettingsOpen(false)}>
      <div className="modal" onMouseDown={e=>e.stopPropagation()}>
        <div className="modalHeader"><div><h2>Agent Settings</h2><p>Configure prompt, model routing, tools, skills and MCP access.</p></div><button className="iconButton" onClick={()=>setAgentSettingsOpen(false)}>✕</button></div>
        {agentDraft && <div className="settingsGrid">
          <label>ID<input value={agentDraft.id || ''} onChange={e=>updateAgentDraft({ id:e.target.value })} disabled={agentDraft.id === 'default'} /></label>
          <label>Name<input value={agentDraft.name || ''} onChange={e=>updateAgentDraft({ name:e.target.value })} /></label>
          <label className="wide">Description<input value={agentDraft.description || ''} onChange={e=>updateAgentDraft({ description:e.target.value })} /></label>
          <label className="wide">System Prompt<textarea value={agentDraft.system_prompt || ''} onChange={e=>updateAgentDraft({ system_prompt:e.target.value })} /></label>
          <label>Model<input value={agentDraft.model || ''} onChange={e=>updateAgentDraft({ model:e.target.value })} placeholder='empty = route automatically' /></label>
          <label>Permission<input value={agentDraft.permission_mode || ''} onChange={e=>updateAgentDraft({ permission_mode:e.target.value })} /></label>
          <label>Max Turns<input type='number' value={agentDraft.max_turns || 0} onChange={e=>updateAgentDraft({ max_turns:Number(e.target.value) })} /></label>
          <label className="wide">Tools<input value={joinList(agentDraft.enabled_tools)} onChange={e=>updateAgentDraft({ enabled_tools:parseList(e.target.value) })} placeholder='read, write, grep, list_dir' /></label>
          <label className="wide">Skills<input value={joinList(agentDraft.enabled_skills)} onChange={e=>updateAgentDraft({ enabled_skills:parseList(e.target.value) })} placeholder='mock-planner' /></label>
          <label className="wide">MCP Servers<input value={joinList(agentDraft.enabled_mcp_servers)} onChange={e=>updateAgentDraft({ enabled_mcp_servers:parseList(e.target.value) })} placeholder='mock' /></label>
        </div>}
        <div className="modalActions"><button onClick={newAgent}>New</button><button onClick={cloneAgent}>Clone</button><button onClick={deleteAgent} disabled={agentDraft?.id === 'default'}>Delete</button><button className="primaryButton" onClick={saveAgent}>Save Agent</button></div>
      </div>
    </div>}
  </div>
}

export default App
