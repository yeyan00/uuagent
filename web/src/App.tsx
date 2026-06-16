import { useEffect, useRef, useState } from 'react'

interface ChatEvent { type: string; model?: string; tier?: string; text?: string; tool_name?: string; tool_id?: string }
interface Message { role: 'user' | 'assistant' | 'system'; content: string; model?: string; tier?: string }
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
  const [status, setStatus] = useState('')
  const [agentDraft, setAgentDraft] = useState<AgentProfile | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)

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
  useEffect(() => { messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages])
  useEffect(() => {
    api<{summaries: Summary[]}>(`/api/sessions/${encodeURIComponent(sessionId)}/summaries`)
      .then(r => setSummaries(r.summaries || [])).catch(() => setSummaries([]))
  }, [sessionId, isStreaming])

  const createProject = async () => {
    const p = await api<Project>('/api/projects', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ name: newProjectName || 'Untitled', workspace_path: newProjectPath }) })
    setProjectId(p.id); setNewProjectName(''); setNewProjectPath(''); await refresh()
  }

  const openProject = async (id: string) => {
    if (!id) return
    const r = await api<any>(`/api/projects/${encodeURIComponent(id)}/open`, { method: 'POST' })
    setProjectId(id); setStatus(`Opened project. Config: ${(r.config_sources || []).join(', ')}`); await refresh()
  }

  const createSession = () => { const id = `s-${Date.now()}`; setSessionId(id); setMessages([]); setSummaries([]) }
  const loadSession = async (id: string) => {
    setSessionId(id)
    const s = await api<Session>(`/api/sessions/${encodeURIComponent(id)}`)
    setMessages(s.messages || [])
  }
  const forkSession = async () => { const child = await api<Session>(`/api/sessions/${encodeURIComponent(sessionId)}/fork`, { method: 'POST' }); setSessionId(child.id); setMessages(child.messages || []); await refresh() }
  const renameSession = async () => {
    const title = prompt('Session title?', sessions.find(s=>s.id===sessionId)?.title || sessionId)
    if (title == null) return
    const s = await api<Session>(`/api/sessions/${encodeURIComponent(sessionId)}`, { method:'PATCH', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ title }) })
    setStatus(`Renamed session ${s.id}`); await refresh()
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

  useEffect(() => {
    const profile = agents.find(a => a.id === agentId)
    setAgentDraft(profile ? { ...profile } : null)
  }, [agents, agentId])

  const splitList = (value?: string[]) => (value || []).join(', ')
  const parseList = (value: string) => value.split(',').map(x => x.trim()).filter(Boolean)
  const updateAgentDraft = (patch: Partial<AgentProfile>) => setAgentDraft(prev => ({ ...(prev || { id: `agent-${Date.now()}`, name: 'New Agent' }), ...patch }))

  const saveAgent = async () => {
    if (!agentDraft) return
    const saved = await api<AgentProfile>('/api/agents', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(agentDraft) })
    setAgentId(saved.id); setStatus(`Saved agent ${saved.id} to config.yaml`); await refresh()
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
    setAgentId('default'); setStatus(`Deleted agent ${agentId}`); await refresh()
  }

  const addMemory = async () => {
    if (!memoryText.trim()) return
    await api('/api/memory', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ content: memoryText, status: 'confirmed', source: 'user', scope: 'project' }) })
    setMemoryText(''); const m = await api<{memories: MemoryEntry[]}>('/api/memory'); setMemories(m.memories || [])
  }

  const sendMessage = async () => {
    if (!input.trim() || isStreaming) return
    const prompt = input
    setMessages(prev => [...prev, { role: 'user', content: prompt }])
    setInput(''); setIsStreaming(true); setRouteInfo(null)
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
            } else if (evt.type === 'tool_result' || evt.type === 'status') setStatus(evt.text || evt.type)
            else if (evt.type === 'error') setMessages(prev => [...prev, { role: 'system', content: `Error: ${evt.text}` }])
          } catch { /* skip */ }
        }
      }
      await refresh()
    } catch (err) { setMessages(prev => [...prev, { role: 'system', content: `Error: ${err}` }]) }
    finally { setIsStreaming(false) }
  }

  return <div style={{ display: 'grid', gridTemplateColumns: '300px 1fr 320px', height: '100vh', color: '#eee', background: '#0f1117' }}>
    <aside style={{ borderRight: '1px solid #333', padding: 12, overflow: 'auto' }}>
      <h2>UUAgent</h2>
      <h3>Projects</h3>
      <select value={projectId} onChange={e => openProject(e.target.value)} style={{ width: '100%' }}><option value=''>Select project</option>{projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select>
      <input placeholder='New project name' value={newProjectName} onChange={e=>setNewProjectName(e.target.value)} style={{ width:'100%', marginTop:8 }} />
      <input placeholder='Workspace path (optional)' value={newProjectPath} onChange={e=>setNewProjectPath(e.target.value)} style={{ width:'100%', marginTop:4 }} />
      <button onClick={createProject} style={{ width:'100%', marginTop:4 }}>Create Project</button>
      <h3>Sessions</h3>
      <button onClick={createSession}>New</button> <button onClick={forkSession}>Fork</button> <button onClick={renameSession}>Rename</button> <button onClick={deleteSession}>Delete</button>
      <select value={sessionId} onChange={e=>loadSession(e.target.value).catch(err=>setStatus(String(err)))} style={{ width:'100%', marginTop:8 }}>{[sessionId, ...sessions.map(s=>s.id)].filter((v,i,a)=>v&&a.indexOf(v)===i).map(id=>{ const s=sessions.find(x=>x.id===id); return <option key={id} value={id}>{s?.title || id}</option> })}</select>
      <h3>Agent</h3>
      <select value={agentId} onChange={e=>selectAgent(e.target.value)} style={{ width:'100%' }}>{agents.map(a=><option key={a.id} value={a.id}>{a.name || a.id}</option>)}</select>
      <button onClick={()=>setAgentDraft({ id:`agent-${Date.now()}`, name:'New Agent', enabled_tools:['read','list_dir'], enabled_mcp_servers:['mock'], permission_mode:'workspace-write', max_turns:20 })} style={{ width:'100%', marginTop:4 }}>New Agent</button>
      <pre style={{ whiteSpace:'pre-wrap', fontSize:11, color:'#aaa' }}>{status}</pre>
    </aside>

    <main style={{ display:'flex', flexDirection:'column', minWidth:0 }}>
      <header style={{ padding: 12, borderBottom:'1px solid #333' }}>Chat {routeInfo && <span style={{ color:'#6f6' }}> — {routeInfo.model} ({routeInfo.tier})</span>}</header>
      <div style={{ flex:1, overflow:'auto', padding:16 }}>{messages.map((msg,i)=><div key={i} style={{ marginBottom:12, padding:10, borderRadius:8, background: msg.role==='user'?'#1a3a5c':msg.role==='system'?'#5c1a1a':'#1a1a2e' }}><b>{msg.role==='user'?'👤 You':msg.role==='system'?'⚠ System':`🤖 ${msg.model || 'Assistant'}`}</b><pre style={{ whiteSpace:'pre-wrap', fontFamily:'inherit' }}>{msg.content}</pre></div>)}<div ref={messagesEndRef}/></div>
      <div style={{ padding:12, borderTop:'1px solid #333', display:'flex', gap:8 }}><input value={input} onChange={e=>setInput(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter'&&!e.shiftKey){ e.preventDefault(); sendMessage() } }} placeholder='Type a message...' style={{ flex:1, padding:10 }} /><button onClick={sendMessage} disabled={isStreaming || !input.trim()}>{isStreaming?'...':'Send'}</button></div>
    </main>

    <aside style={{ borderLeft:'1px solid #333', padding:12, overflow:'auto' }}>
      <h3>Agent Config</h3>
      {agentDraft && <div style={{ display:'grid', gap:6 }}>
        <label>ID<input value={agentDraft.id || ''} onChange={e=>updateAgentDraft({ id:e.target.value })} style={{ width:'100%' }} disabled={agentDraft.id === 'default'} /></label>
        <label>Name<input value={agentDraft.name || ''} onChange={e=>updateAgentDraft({ name:e.target.value })} style={{ width:'100%' }} /></label>
        <label>Description<input value={agentDraft.description || ''} onChange={e=>updateAgentDraft({ description:e.target.value })} style={{ width:'100%' }} /></label>
        <label>System Prompt<textarea value={agentDraft.system_prompt || ''} onChange={e=>updateAgentDraft({ system_prompt:e.target.value })} style={{ width:'100%', height:90 }} /></label>
        <label>Model<input value={agentDraft.model || ''} onChange={e=>updateAgentDraft({ model:e.target.value })} placeholder='empty = route automatically' style={{ width:'100%' }} /></label>
        <label>Tools<input value={splitList(agentDraft.enabled_tools)} onChange={e=>updateAgentDraft({ enabled_tools:parseList(e.target.value) })} placeholder='read, write, grep, list_dir' style={{ width:'100%' }} /></label>
        <label>Skills<input value={splitList(agentDraft.enabled_skills)} onChange={e=>updateAgentDraft({ enabled_skills:parseList(e.target.value) })} placeholder='mock-planner' style={{ width:'100%' }} /></label>
        <label>MCP Servers<input value={splitList(agentDraft.enabled_mcp_servers)} onChange={e=>updateAgentDraft({ enabled_mcp_servers:parseList(e.target.value) })} placeholder='mock' style={{ width:'100%' }} /></label>
        <label>Permission<input value={agentDraft.permission_mode || ''} onChange={e=>updateAgentDraft({ permission_mode:e.target.value })} style={{ width:'100%' }} /></label>
        <label>Max Turns<input type='number' value={agentDraft.max_turns || 0} onChange={e=>updateAgentDraft({ max_turns:Number(e.target.value) })} style={{ width:'100%' }} /></label>
        <div><button onClick={saveAgent}>Save</button> <button onClick={cloneAgent}>Clone</button> <button onClick={deleteAgent} disabled={agentDraft.id === 'default'}>Delete</button></div>
      </div>}
      <h3>Memory</h3>
      <textarea value={memoryText} onChange={e=>setMemoryText(e.target.value)} placeholder='Add memory' style={{ width:'100%', height:60 }} />
      <button onClick={addMemory}>Add</button>
      {memories.map(m=><div key={m.id} style={{ border:'1px solid #333', marginTop:8, padding:8 }}><small>{m.status}/{m.scope}</small><div>{m.content}</div></div>)}
      <h3>Compression</h3>
      {summaries.map(s=><details key={s.id} style={{ marginBottom:8 }}><summary>{s.token_before} → {s.token_after}</summary><pre style={{ whiteSpace:'pre-wrap' }}>{s.summary}</pre></details>)}
    </aside>
  </div>
}

export default App
