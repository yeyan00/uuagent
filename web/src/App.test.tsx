import React from 'react'
import { describe, expect, it, vi, afterEach } from 'vitest'
import { fireEvent, render, screen, waitFor, cleanup, within } from '@testing-library/react'
import App from './App'

afterEach(() => {
  cleanup()
})

describe('App', () => {
  it('renders rail navigation and opens agent settings from Settings page', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent', system_prompt: 'test system', enabled_tools: ['read'] }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any
    render(<App />)
    expect(await screen.findByText('Projects')).toBeTruthy()
    expect(await screen.findByText('Extensions')).toBeTruthy()
    expect(await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')).toBeTruthy()
    expect(screen.getByText('Start a coding session')).toBeTruthy()
    expect(screen.queryByText('Agent Settings')).toBeNull()
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Agents' }))
    expect(await screen.findByText('Configure prompt, model routing, tools, skills and MCP access.')).toBeTruthy()
    expect(await screen.findByDisplayValue('test system')).toBeTruthy()
  })

  it('stops an active agent run through the Stop API', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    let controller: ReadableStreamDefaultController<Uint8Array>
    const calls: string[] = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push(`${init?.method || 'GET'} ${url}`)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url.startsWith('/api/runs/run-ui/stop')) return Response.json({ status: 'stopping' })
      if (url.startsWith('/api/chat')) {
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            controller = c
            c.enqueue(encoder.encode('data: {"type":"run","run_id":"run-ui"}\n\n'))
            c.enqueue(encoder.encode('data: {"type":"status","text":"thinking..."}\n\n'))
          },
          cancel() {},
        })
        init?.signal?.addEventListener('abort', () => controller.close())
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'run a long task' } })
    fireEvent.click(screen.getByText('Send'))
    fireEvent.click(await screen.findByText('Stop'))
    await waitFor(() => expect(calls).toContain('POST /api/runs/run-ui/stop'))
  })

  it('opens Settings in the main workspace without interrupting active chat streaming', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    let controller!: ReadableStreamDefaultController<Uint8Array>
    const calls: string[] = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push(`${init?.method || 'GET'} ${url}`)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', enabled: true, scope: 'global' }], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/runs/run-overlay/stop')) return Response.json({ status: 'stopping' })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url.startsWith('/api/chat')) {
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            controller = c
            c.enqueue(encoder.encode('data: {"type":"run","run_id":"run-overlay"}\n\n'))
            c.enqueue(encoder.encode('data: {"type":"content","text":"partial answer"}\n\n'))
          },
          cancel() {},
        })
        init?.signal?.addEventListener('abort', () => controller.close())
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'keep streaming while configuring' } })
    fireEvent.click(screen.getByText('Send'))
    expect(await screen.findByText('partial answer')).toBeTruthy()
    fireEvent.click(await screen.findByTitle('Settings'))
    const navRail = document.querySelector('.navRail')
    expect(navRail).toBeTruthy()
    const chatNav = within(navRail as HTMLElement).getByRole('button', { name: 'Chat' })
    expect(chatNav).toBeTruthy()
    expect(await screen.findByText('Stop')).toBeTruthy()
    controller.enqueue(encoder.encode('data: {"type":"content","text":" after settings"}\n\n'))
    fireEvent.click(chatNav)
    expect(await screen.findByText('partial answer after settings')).toBeTruthy()
    fireEvent.click(await screen.findByText('Stop'))
    await waitFor(() => expect(calls).toContain('POST /api/runs/run-overlay/stop'))
  })

  it('shows approval-required tool results in the chat', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/runs/run-approval/approve/stream') {
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'tool_start', run_id: 'run-approval', tool_name: 'read', tool_id: 'call-1', args: '{"path":"C:/outside/file.txt"}' })}\n\n`))
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'tool_result', run_id: 'run-approval', tool_name: 'read', tool_id: 'call-1', text: 'approved file contents' })}\n\n`))
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'tool_start', run_id: 'run-approval', tool_name: 'grep', tool_id: 'call-2' })}\n\n`))
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'tool_result', run_id: 'run-approval', tool_name: 'grep', tool_id: 'call-2', text: 'matched important code' })}\n\n`))
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'content', run_id: 'run-approval', text: 'final streamed analysis' })}\n\n`))
            c.enqueue(encoder.encode('data: {"type":"done","run_id":"run-approval"}\n\n'))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      if (url === '/api/runs/run-approval/deny') return Response.json({ status: 'denied' })
      if (url.startsWith('/api/chat')) {
        const approval = JSON.stringify({ approval_required: true, tool: 'read', path: 'C:/outside/file.txt', reason: 'path is outside workspace and requires approval' })
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode('data: {"type":"run","run_id":"run-approval"}\n\n'))
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'tool_result', tool_id: 'call-1', text: approval })}\n\n`))
            c.enqueue(encoder.encode('data: {"type":"done"}\n\n'))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'read outside file' } })
    fireEvent.click(screen.getByText('Send'))

    expect((await screen.findAllByText('Approval required')).length).toBeGreaterThanOrEqual(2)
    expect(await screen.findByText('read wants access')).toBeTruthy()
    expect(await screen.findByText('C:/outside/file.txt')).toBeTruthy()
    expect(await screen.findByText('path is outside workspace and requires approval')).toBeTruthy()
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }))
    await waitFor(() => expect(calls.some(c => c.url === '/api/runs/run-approval/approve/stream' && c.init?.method === 'POST')).toBe(true))
    expect(await screen.findByText('Running read')).toBeTruthy()
    expect(await screen.findByText('path: C:/outside/file.txt')).toBeTruthy()
    expect(await screen.findByText('approved file contents')).toBeTruthy()
    expect(await screen.findByText('Running grep')).toBeTruthy()
    expect(await screen.findByText('matched important code')).toBeTruthy()
    expect(await screen.findByText('final streamed analysis')).toBeTruthy()
    await waitFor(() => expect(screen.getAllByText('Approved').length).toBeGreaterThanOrEqual(1))
    expect(await screen.findByRole('button', { name: 'Deny' })).toBeTruthy()
    expect(screen.queryByText('Done')).toBeNull()
  })

  it('renders reasoning and richer markdown in assistant messages', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url.startsWith('/api/chat')) {
        const md = ['> important note', '', '1. first item', '2. second item', '', '| Name | Value |', '| --- | --- |', '| Go | backend |'].join('\n')
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'reasoning', text: 'checking files' })}\n\n`))
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'content', text: md })}\n\n`))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'show markdown' } })
    fireEvent.click(screen.getByText('Send'))
    expect(await screen.findByText('Thinking')).toBeTruthy()
    expect(await screen.findByText('checking files')).toBeTruthy()
    expect(await screen.findByText('important note')).toBeTruthy()
    expect(await screen.findByText('first item')).toBeTruthy()
    expect(await screen.findByText('Name')).toBeTruthy()
    expect(await screen.findByText('backend')).toBeTruthy()
  })

  it('parses SSE events split across network chunks', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url.startsWith('/api/chat')) {
        const event = `data: ${JSON.stringify({ type: 'content', text: 'split streamed answer' })}\n\n`
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode(event.slice(0, 12)))
            c.enqueue(encoder.encode(event.slice(12)))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'stream split test' } })
    fireEvent.click(screen.getByText('Send'))
    expect(await screen.findByText('split streamed answer')).toBeTruthy()
  })

  it('groups loaded session tool history inside the user turn', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Analyze repo', messages: [{ role: 'user', content: 'Analyze repo' }] }] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Analyze repo', messages: [
        { role: 'user', content: 'Analyze repo' },
        { role: 'assistant', content: '', tool_calls: [
          { id: 'tc-list', function: { name: 'ls', arguments: '{"path":"C:/repo"}' } },
          { id: 'tc-read', function: { name: 'read', arguments: '{"path":"C:/repo/go.mod"}' } },
        ] },
        { role: 'tool', tool_call_id: 'tc-list', tool_name: 'ls', content: 'README.md\ninternal' },
        { role: 'tool', tool_call_id: 'tc-read', tool_name: 'read', content: 'module example' },
        { role: 'assistant', content: 'Final repo summary' },
      ] })
      if (url.startsWith('/api/projects/proj-1/sessions/s1/context')) return Response.json({ summaries: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    await waitFor(() => expect(screen.getAllByText('Repo').length).toBeGreaterThan(0))
    fireEvent.click((await screen.findAllByText('Analyze repo'))[0])
    await waitFor(() => expect(screen.getAllByText('Analyze repo').length).toBeGreaterThanOrEqual(2))
    expect(await screen.findByText('Tool activity · 2')).toBeTruthy()
    await waitFor(() => expect(document.querySelector('details.toolActivityGroup')?.hasAttribute('open')).toBe(false))
    expect(await screen.findByText('ls result')).toBeTruthy()
    expect(await screen.findByText('path: C:/repo')).toBeTruthy()
    expect(await screen.findByText('read result')).toBeTruthy()
    expect(await screen.findByText('path: C:/repo/go.mod')).toBeTruthy()
    expect(await screen.findByText('Final repo summary')).toBeTruthy()
    expect(screen.queryByText('Tool')).toBeNull()
  })

  it('loads and creates memory for the selected project', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [{ id: 'm1', content: 'Existing project memory', status: 'confirmed', source: 'markdown', project: 'C:/repo', scope: 'project' }] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/memory' && init?.method === 'POST') return Response.json({ id: 'm2', content: 'New project memory' })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.change(await screen.findByDisplayValue('None'), { target: { value: 'proj-1' } })
    fireEvent.click(await screen.findByTitle('Project settings'))
    fireEvent.click(await screen.findByText('memory'))
    expect(await screen.findByText('Existing project memory')).toBeTruthy()

    fireEvent.change(await screen.findByPlaceholderText('Add confirmed memory...'), { target: { value: 'New project memory' } })
    fireEvent.click(await screen.findByText('Add Memory'))

    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/memory' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ project: 'proj-1', content: 'New project memory' })
    })
    expect(calls.some(c => c.url === '/api/memory?project=proj-1')).toBe(true)
  })

  it('shows project drawers with project-scoped sessions', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: string[] = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push(`${init?.method || 'GET'} ${url}`)
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: '当前目录是什么', messages: [{ role: 'user', content: '当前目录是什么' }] }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: '当前目录是什么', messages: [{ role: 'user', content: '当前目录是什么' }] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    await waitFor(() => expect(screen.getAllByText('Repo').length).toBeGreaterThan(0))
    expect(await screen.findByText('当前目录是什么')).toBeTruthy()
    fireEvent.click(await screen.findByText('当前目录是什么'))
    await waitFor(() => expect(calls).toContain('GET /api/projects/proj-1/sessions/s1'))
    expect(calls).toContain('GET /api/projects/proj-1/sessions')
  })

  it('shows discovered skills and diagnostics in Settings Skills', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [
        { name: 'review', description: 'Review code', scope: 'global', enabled: true },
        { name: 'docx', description: 'Work with Word documents', scope: 'global', enabled: true },
      ], diagnostics: [{ path: 'bad/SKILL.md', message: 'missing description' }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByText('Skills'))
    expect((await screen.findAllByText('review')).length).toBeGreaterThan(0)
    expect(await screen.findByText(/Review code/)).toBeTruthy()
    expect((await screen.findAllByText('docx')).length).toBeGreaterThan(0)
    expect(await screen.findByText(/Work with Word documents/)).toBeTruthy()
    expect(await screen.findByText('missing description')).toBeTruthy()
  })

  it('saves agent skill checkbox selection and treats empty as all skills', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents' && init?.method === 'POST') return Response.json(JSON.parse(String(init.body)))
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent', enabled_skills: [] }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [
        { name: 'review', description: 'Review code', enabled: true, scope: 'global' },
        { name: 'docx', description: 'Work with docs', enabled: true, scope: 'global' },
      ], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Agents' }))
    fireEvent.click(await screen.findByRole('button', { name: /Edit/ }))
    expect(await screen.findByText('All skills')).toBeTruthy()
    fireEvent.click(await screen.findByLabelText('review'))
    fireEvent.click(await screen.findByRole('button', { name: 'Save Agent' }))
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/agents' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ enabled_skills: ['review'] })
    })
  })

  it('manages subagent editing from Settings Subagents', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents' && init?.method === 'POST') return Response.json(JSON.parse(String(init.body)))
      if (url === '/api/subagents') return Response.json({ subagents: [{ id: 'reviewer', name: 'Reviewer', enabled_skills: [] }] })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', enabled: true, scope: 'global' }], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Subagents' }))
    fireEvent.click(screen.getAllByText('Reviewer')[0])
    fireEvent.click(await screen.findByRole('button', { name: /Edit/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/subagents' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ id: 'reviewer' })
    })
  })

  it('forces a single selected skill when sending from composer', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const encoder = new TextEncoder()
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', enabled: true, scope: 'global' }], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/chat') {
        const stream = new ReadableStream<Uint8Array>({ start(c) { c.enqueue(encoder.encode('data: {"type":"content","text":"ok"}\n\n')); c.close() } })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.change(await screen.findByLabelText('Skill'), { target: { value: 'review' } })
    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'inspect code' } })
    fireEvent.click(screen.getByText('Send'))
    await waitFor(() => {
      const chatCall = calls.find(c => c.url === '/api/chat')
      expect(chatCall).toBeTruthy()
      const body = JSON.parse(chatCall?.init?.body as string)
      expect(body.prompt).toContain('/skill:review inspect code')
    })
  })

  it('keeps Settings navigation visible and previews skill content', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', scope: 'global', enabled: true }], diagnostics: [] })
      if (url === '/api/skills/review/content') return Response.json({ name: 'review', content: 'FULL REVIEW SKILL BODY' })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByText('Skills'))
    fireEvent.click((await screen.findAllByText('review'))[0])
    expect(await screen.findByText('FULL REVIEW SKILL BODY')).toBeTruthy()
    fireEvent.click(await screen.findByText('Agents'))
    expect(await screen.findByText('Configure prompt, model routing, tools, skills and MCP access.')).toBeTruthy()
    fireEvent.click(await screen.findByText('Skills'))
    expect((await screen.findAllByText('review')).length).toBeGreaterThan(0)
  })

  it('bulk deletes selected skills from the Skills grid after confirmation', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if ((url === '/api/skills/review' || url === '/api/skills/write') && init?.method === 'DELETE') return Response.json({ status: 'ok' })
      if (url === '/api/skills') return Response.json({ skills: [
        { name: 'review', description: 'Review code', scope: 'global', enabled: true },
        { name: 'write', description: 'Write code', scope: 'global', enabled: true },
      ], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByTitle('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Skills' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    fireEvent.click(await screen.findByLabelText('Select review'))
    fireEvent.click(await screen.findByLabelText('Select write'))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete selected' }))
    await waitFor(() => expect(window.confirm).toHaveBeenCalled())
    await waitFor(() => expect(calls.some(c => c.url === '/api/skills/review' && c.init?.method === 'DELETE')).toBe(true))
    await waitFor(() => expect(calls.some(c => c.url === '/api/skills/write' && c.init?.method === 'DELETE')).toBe(true))
  })

  it('uses sidebar settings navigation and manages skills through grid cards and modals', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills' && init?.method === 'POST') return Response.json({ name: 'new-skill' })
      if (url === '/api/skills/review' && init?.method === 'DELETE') return Response.json({ status: 'ok' })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', scope: 'global', enabled: true }], diagnostics: [] })
      if (url === '/api/skills/review/content') return Response.json({ name: 'review', content: 'FULL REVIEW SKILL BODY' })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByTitle('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Skills' }))
    expect(document.querySelector('.settingsSideMenu')).toBeTruthy()
    expect(document.querySelector('.skillCardGrid')).toBeTruthy()
    fireEvent.click(await screen.findByLabelText('Add skill'))
    expect(await screen.findByText('Add Skill')).toBeTruthy()
    fireEvent.change(await screen.findByPlaceholderText('Skill name'), { target: { value: 'new-skill' } })
    fireEvent.change(await screen.findByPlaceholderText('Paste SKILL.md content or instructions'), { target: { value: 'NEW BODY' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Create Skill' }))
    await waitFor(() => expect(calls.some(c => c.url === '/api/skills' && c.init?.method === 'POST')).toBe(true))
    fireEvent.click(await screen.findByRole('button', { name: /review/ }))
    expect(await screen.findByText('FULL REVIEW SKILL BODY')).toBeTruthy()
    expect(await screen.findByText('SKILL.md')).toBeTruthy()
  })

  it('creates and deletes skills from Settings Skills', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills' && init?.method === 'POST') return Response.json({ name: 'new-skill' })
      if (url === '/api/skills/new-skill' && init?.method === 'DELETE') return Response.json({ status: 'ok' })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'new-skill', description: 'New skill', scope: 'global', enabled: true }], diagnostics: [] })
      if (url === '/api/skills/new-skill/content') return Response.json({ name: 'new-skill', content: 'NEW BODY' })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByTitle('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Skills' }))
    fireEvent.click(await screen.findByLabelText('Add skill'))
    fireEvent.change(await screen.findByPlaceholderText('Skill name'), { target: { value: 'new-skill' } })
    fireEvent.change(await screen.findByPlaceholderText('Skill description'), { target: { value: 'New skill' } })
    fireEvent.change(await screen.findByPlaceholderText('Paste SKILL.md content or instructions'), { target: { value: 'NEW BODY' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Create Skill' }))
    await waitFor(() => expect(calls.some(c => c.url === '/api/skills' && c.init?.method === 'POST')).toBe(true))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    fireEvent.click(await screen.findByLabelText('Select new-skill'))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete selected' }))
    await waitFor(() => expect(calls.some(c => c.url === '/api/skills/new-skill' && c.init?.method === 'DELETE')).toBe(true))
  })

  it('creates edits and deletes subagents with full fields', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents' && init?.method === 'POST') return Response.json(JSON.parse(String(init.body)))
      if (url === '/api/subagents/reviewer' && init?.method === 'DELETE') return Response.json({ status: 'ok' })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', enabled: true, scope: 'global' }], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Subagents' }))
    fireEvent.click(await screen.findByText('+ New Subagent'))
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/subagents' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
    })
  })

  it('shows current context and session token usage in project settings', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Token session', messages: [{ role: 'user', content: 'hi' }] }] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Token session', messages: [{ role: 'user', content: 'hi' }] })
      if (url === '/api/projects/proj-1/sessions/s1/context') return Response.json({ context: { estimated_tokens: 18400, max_tokens: 32000, percent: 0.575 }, usage: { input_tokens: 48000, output_tokens: 9000, total_tokens: 57000 }, summaries: [{ id: 'sum1', token_before: 32000, token_after: 8000, summary: 'Previous summary' }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Token session'))
    fireEvent.click(await screen.findByTitle('Project settings'))
    fireEvent.click(await screen.findByText('context'))

    expect(await screen.findByText('18.4k / 32k tokens')).toBeTruthy()
    expect(await screen.findByText('57%')).toBeTruthy()
    expect(await screen.findByText('Input: 48k')).toBeTruthy()
    expect(await screen.findByText('Output: 9k')).toBeTruthy()
    expect(await screen.findByText('Total: 57k')).toBeTruthy()
    expect(await screen.findByText('Previous summary')).toBeTruthy()
    expect(screen.queryByText(/threshold/i)).toBeNull()
  })

  it('shows active session token usage without opening project settings', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Token session', messages: [{ role: 'user', content: 'hi' }] }] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Token session', messages: [{ role: 'user', content: 'hi' }] })
      if (url === '/api/projects/proj-1/sessions/s1/context') return Response.json({ context: { estimated_tokens: 18400, max_tokens: 32000, percent: 0.575 }, usage: { input_tokens: 48000, output_tokens: 9000, total_tokens: 57000 }, summaries: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Token session'))

    expect(await screen.findByText('Session Tokens')).toBeTruthy()
    expect(await screen.findByText('Input 48k')).toBeTruthy()
    expect(await screen.findByText('Output 9k')).toBeTruthy()
    expect(await screen.findByText('Total 57k')).toBeTruthy()
    expect(screen.queryByText('Project Settings')).toBeNull()
  })

  it('loads and displays Models Settings with proxy URL, fallback tier, and routing configuration', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings') return Response.json({
        proxy_url: 'http://localhost:8080/v1',
        fallback_tier: 'strong',
        routing_tiers: {
          fast: ['gpt-4o-mini', 'deepseek-chat'],
          strong: ['claude-sonnet-4', 'gpt-4o'],
          large_ctx: ['gemini-2.5-pro']
        },
        model_ids: ['gpt-4o-mini', 'deepseek-chat', 'claude-sonnet-4', 'gpt-4o', 'gemini-2.5-pro']
      })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByText('Models'))

    expect(await screen.findByDisplayValue('http://localhost:8080/v1')).toBeTruthy()
    expect(await screen.findByDisplayValue('strong')).toBeTruthy()
    expect(await screen.findByText('fast')).toBeTruthy()
    expect(await screen.findByText('strong')).toBeTruthy()
    expect(await screen.findByText('large_ctx')).toBeTruthy()
  })

  it('saves Models Settings via PUT /api/models/settings', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings' && init?.method === 'PUT') return Response.json({ status: 'saved' })
      if (url === '/api/models/settings') return Response.json({
        proxy_url: 'http://localhost:8080/v1',
        fallback_tier: 'strong',
        routing_tiers: { fast: ['gpt-4o-mini'], strong: ['gpt-4o'] },
        model_ids: ['gpt-4o-mini', 'gpt-4o']
      })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByText('Models'))

    fireEvent.change(await screen.findByDisplayValue('http://localhost:8080/v1'), { target: { value: 'http://new-proxy:9000/v1' } })
    fireEvent.click(await screen.findByText('Save Settings'))

    await waitFor(() => {
      const put = calls.find(c => c.url === '/api/models/settings' && c.init?.method === 'PUT')
      expect(put).toBeTruthy()
      expect(JSON.parse(String(put?.init?.body))).toMatchObject({
        proxy_url: 'http://new-proxy:9000/v1',
        fallback_tier: 'strong',
        routing_tiers: { fast: ['gpt-4o-mini'], strong: ['gpt-4o'] }
      })
    })
  })

  it('tests model connection via POST /api/models/test and displays results', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings') return Response.json({
        proxy_url: 'http://localhost:8080/v1',
        proxy_api_key: 'sk-uuagent-test-connection',
        fallback_tier: 'strong',
        routing_tiers: { fast: ['gpt-4o-mini'] },
        model_ids: ['gpt-4o-mini']
      })
      if (url === '/api/models/test') return Response.json({
        success: true,
        model_ids: ['gpt-4o-mini', 'gpt-4o'],
        error: null
      })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByText('Models'))
    fireEvent.click(await screen.findByText('Test Connection'))

    expect(await screen.findByText('Available models: gpt-4o-mini, gpt-4o')).toBeTruthy()
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/models/test' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({
        proxy_url: 'http://localhost:8080/v1',
        proxy_api_key: 'sk-uuagent-test-connection',
      })
    })
  })

  it('includes configured model IDs in chat composer model dropdown', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent', model: 'gpt-4o' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings') return Response.json({
        proxy_url: 'http://localhost:8080/v1',
        fallback_tier: 'strong',
        routing_tiers: { fast: ['gpt-4o-mini'], strong: ['gpt-4o'] },
        model_ids: ['gpt-4o-mini', 'gpt-4o', 'claude-sonnet-4']
      })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    await waitFor(() => expect(screen.getAllByText('gpt-4o-mini').length).toBeGreaterThan(0))
    expect(screen.getAllByText('claude-sonnet-4').length).toBeGreaterThan(0)
  })

  it('selecting an image file shows preview and enables send even when text input is empty', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    const fileInput = await screen.findByLabelText('Attach image or file')
    const file = new File(['test'], 'test-image.png', { type: 'image/png' })
    fireEvent.change(fileInput, { target: { files: [file] } })

    expect(await screen.findByText('test-image.png')).toBeTruthy()
    expect(await screen.findByRole('img', { name: 'Attachment preview' })).toBeTruthy()
    expect((screen.getByText('Send') as HTMLButtonElement).disabled).toBe(false)
  })

  it('removing an attachment removes preview and disables send again when no text', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    const fileInput = await screen.findByLabelText('Attach image or file')
    const file = new File(['test'], 'test-image.png', { type: 'image/png' })
    fireEvent.change(fileInput, { target: { files: [file] } })

    expect(await screen.findByText('test-image.png')).toBeTruthy()
    fireEvent.click(await screen.findByRole('button', { name: 'Remove attachment' }))
    expect(screen.queryByText('test-image.png')).toBeNull()
    expect((screen.getByText('Send') as HTMLButtonElement).disabled).toBe(true)
  })

  it('sending text and selected image sends image_url in POST body, not URL params', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/chat') {
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode('data: {"type":"content","text":"I see the image"}\n\n'))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    const fileInput = await screen.findByLabelText('Attach image or file')
    const file = new File(['test'], 'test-image.png', { type: 'image/png' })
    fireEvent.change(fileInput, { target: { files: [file] } })
    expect(await screen.findByText('test-image.png')).toBeTruthy()

    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'What is in this image?' } })
    fireEvent.click(screen.getByText('Send'))

    await waitFor(() => expect(calls.some(c => c.url === '/api/chat')).toBe(true))
    const chatCall = calls.find(c => c.url === '/api/chat')
    expect(chatCall?.init?.method).toBe('POST')
    expect((chatCall?.init?.headers as Record<string, string>)?.['Content-Type']).toBe('application/json')
    const body = JSON.parse(chatCall?.init?.body as string)
    expect(body.image_url).toBeDefined()
    expect(body.image_url.length).toBeGreaterThan(0)
    expect(chatCall?.url).not.toContain('image_url=')
    expect(await screen.findByText('test-image.png')).toBeTruthy()
    expect(await screen.findByRole('img', { name: 'Attachment preview' })).toBeTruthy()
  })

  it('pasting an image into the composer adds it as an attachment preview', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    const textarea = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    const file = new File(['test'], 'pasted-image.png', { type: 'image/png' })
    const clipboardData = { files: [file], getData: () => '', items: [{ kind: 'file', type: 'image/png', getAsFile: () => file }] }
    fireEvent.paste(textarea, { clipboardData })

    expect(await screen.findByText('pasted-image.png')).toBeTruthy()
    expect(await screen.findByRole('img', { name: 'Attachment preview' })).toBeTruthy()
  })

  it('compacts the active session and shows compact archive history', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Session to compact', messages: [{ role: 'user', content: 'hi' }] }] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Session to compact', messages: [{ role: 'user', content: 'hi' }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      if (url === '/api/projects/proj-1/sessions/s1/context') return Response.json({ context: { estimated_tokens: 50000, max_tokens: 32000, percent: 1.56 }, usage: { input_tokens: 48000, output_tokens: 9000, total_tokens: 57000 }, summaries: [], archives: [] })
      if (url === '/api/projects/proj-1/sessions/s1/compact' && init?.method === 'POST') {
        return Response.json({
          context: { estimated_tokens: 8000, max_tokens: 32000, percent: 0.25 },
          usage: { input_tokens: 7000, output_tokens: 1000, total_tokens: 8000 },
          summaries: [{ id: 'sum1', token_before: 50000, token_after: 8000, summary: 'Compacted conversation', created_at: Date.now() }],
          archives: [{ id: 'arch1', summary: { id: 'arch1-summary', summary: 'Archived conversation block', token_before: 50000, token_after: 8000, created_at: Date.now() }, token_before: 50000, token_after: 8000, created_at: new Date().toISOString() }]
        })
      }
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Session to compact'))
    await waitFor(() => expect(calls.some(c => c.url === '/api/projects/proj-1/sessions/s1')).toBe(true))

    // Compact button should be visible in workspace actions
    const compactButton = await screen.findByRole('button', { name: 'Compact' })
    expect(compactButton).toBeTruthy()

    // Click compact button
    fireEvent.click(compactButton)

    // Verify compact API was called
    await waitFor(() => expect(calls.some(c => c.url === '/api/projects/proj-1/sessions/s1/compact' && c.init?.method === 'POST')).toBe(true))

    // Open project settings and check context tab for archives
    fireEvent.click(await screen.findByTitle('Project settings'))
    fireEvent.click(await screen.findByText('context'))

    // Should show compact archives
    expect(await screen.findByText('Compact Archives')).toBeTruthy()
    expect(await screen.findByText('Archived conversation block')).toBeTruthy()
  })

  it('restores a compact archive and updates session messages', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    vi.stubGlobal('confirm', vi.fn(() => true))
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Session to restore', messages: [{ role: 'user', content: 'hi' }] }] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Session to restore', messages: [{ role: 'user', content: 'hi' }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      if (url === '/api/projects/proj-1/sessions/s1/context') {
        return Response.json({
          context: { estimated_tokens: 8000, max_tokens: 32000, percent: 0.25 },
          usage: { input_tokens: 7000, output_tokens: 1000, total_tokens: 8000 },
          summaries: [{ id: 'sum1', token_before: 50000, token_after: 8000, summary: 'Compacted conversation', created_at: Date.now() }],
          archives: [{ id: 'arch1', summary: { id: 'arch1-summary', summary: 'Archived conversation block', token_before: 50000, token_after: 8000, created_at: Date.now() }, token_before: 50000, token_after: 8000, created_at: new Date().toISOString() }]
        })
      }
      if (url === '/api/projects/proj-1/sessions/s1/archives/arch1/restore' && init?.method === 'POST') {
        return Response.json({
          session: { id: 's1', title: 'Session to restore', messages: [{ role: 'user', content: 'restored message' }] },
          context: { estimated_tokens: 50000, max_tokens: 32000, percent: 1.56 },
          usage: { input_tokens: 48000, output_tokens: 9000, total_tokens: 57000 },
          summaries: [],
          archives: [],
          restored: 1
        })
      }
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Session to restore'))
    await waitFor(() => expect(calls.some(c => c.url === '/api/projects/proj-1/sessions/s1')).toBe(true))

    fireEvent.click(await screen.findByTitle('Project settings'))
    fireEvent.click(await screen.findByText('context'))

    expect(await screen.findByText('Compact Archives')).toBeTruthy()
    expect(await screen.findByText('Archived conversation block')).toBeTruthy()

    const restoreButton = await screen.findByRole('button', { name: 'Restore' })
    expect(restoreButton).toBeTruthy()

    fireEvent.click(restoreButton)

    await waitFor(() => expect(calls.some(c => c.url === '/api/projects/proj-1/sessions/s1/archives/arch1/restore' && c.init?.method === 'POST')).toBe(true))
  })

  it('loads compact archives from context endpoint on session load without compacting', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Repo', workspace_path: 'C:/repo', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/projects/proj-1/sessions') return Response.json({ sessions: [{ id: 's1', title: 'Session with archives', messages: [{ role: 'user', content: 'hi' }] }] })
      if (url === '/api/projects/proj-1/sessions/s1') return Response.json({ id: 's1', title: 'Session with archives', messages: [{ role: 'user', content: 'hi' }] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/memory?project=proj-1') return Response.json({ memories: [] })
      if (url === '/api/projects/proj-1/open') return Response.json({ config_sources: [] })
      if (url === '/api/projects/proj-1/sessions/s1/context') {
        return Response.json({
          context: { estimated_tokens: 8000, max_tokens: 32000, percent: 0.25 },
          usage: { input_tokens: 7000, output_tokens: 1000, total_tokens: 8000 },
          summaries: [{ id: 'sum1', token_before: 50000, token_after: 8000, summary: 'Compacted conversation', created_at: Date.now() }],
          archives: [{ id: 'arch1', summary: { id: 'arch1-summary', summary: 'Previously archived block', token_before: 50000, token_after: 8000, created_at: Date.now() }, token_before: 50000, token_after: 8000, created_at: new Date().toISOString() }]
        })
      }
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    fireEvent.click(await screen.findByText('Session with archives'))
    await waitFor(() => expect(calls.some(c => c.url === '/api/projects/proj-1/sessions/s1')).toBe(true))

    fireEvent.click(await screen.findByTitle('Project settings'))
    fireEvent.click(await screen.findByText('context'))

    expect(await screen.findByText('Compact Archives')).toBeTruthy()
    expect(await screen.findByText('Previously archived block')).toBeTruthy()

    const compactCalls = calls.filter(c => c.url.includes('/compact') && c.init?.method === 'POST')
    expect(compactCalls.length).toBe(0)
  })

  it('trims project name and workspace path when creating a project', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') {
        if (init?.method === 'POST') {
          const body = JSON.parse(String(init.body))
          return Response.json({ id: 'proj-trimmed', name: body.name, workspace_path: body.workspace_path, temporary: false })
        }
        return Response.json({ projects: [] })
      }
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    const nameInput = await screen.findByPlaceholderText('Project name')
    const pathInput = await screen.findByPlaceholderText('Workspace path optional')

    fireEvent.change(nameInput, { target: { value: '  My Project  ' } })
    fireEvent.change(pathInput, { target: { value: '  C:/workspace/path  ' } })
    fireEvent.click(screen.getByText('Create'))

    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/projects' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      const body = JSON.parse(String(post?.init?.body))
      expect(body.name).toBe('My Project')
      expect(body.workspace_path).toBe('C:/workspace/path')
    })
  })

  it('shows create project error and preserves input fields on failure', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const fetchMock: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/projects' && init?.method === 'POST') {
        return new Response('workspace path is not a directory: C:/some/file.txt', { status: 400, statusText: 'Bad Request' })
      }
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    })
    globalThis.fetch = fetchMock

    render(<App />)
    const nameInput = await screen.findByPlaceholderText('Project name')
    const pathInput = await screen.findByPlaceholderText('Workspace path optional')

    fireEvent.change(nameInput, { target: { value: 'Test Project' } })
    fireEvent.change(pathInput, { target: { value: 'C:/some/file.txt' } })
    fireEvent.click(screen.getByText('Create'))

    // Should show error status in create project error div
    await waitFor(() => expect(document.querySelector('.createProjectError')?.textContent).toMatch(/workspace path is not a directory/i))

    // Input fields should still have their values
    expect((nameInput as HTMLInputElement).value).toBe('Test Project')
    expect((pathInput as HTMLInputElement).value).toBe('C:/some/file.txt')
  })

  it('creates a goal run and displays plan todos and activity', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Test Project', workspace_path: 'C:/test', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/projects/proj-1/goals' && init?.method === 'POST') {
        return Response.json({
          id: 'goal-1',
          project_id: 'proj-1',
          session_id: 's-1',
          agent_id: 'default',
          goal: 'Implement Goal Mode',
          status: 'running',
          plan: [
            { id: 'step-1', description: 'Understand goal and inspect context', subagent: 'planner' },
            { id: 'step-2', description: 'Explore relevant code', subagent: 'explorer' },
            { id: 'step-3', description: 'Implement focused changes', subagent: 'builder' },
            { id: 'step-4', description: 'Test and verify', subagent: 'tester' },
            { id: 'step-5', description: 'Review completion', subagent: 'reviewer' }
          ],
          todos: [
            { id: 'todo-1', step_id: 'step-1', description: 'Understand goal and inspect context', status: 'completed', result: 'Goal understood' },
            { id: 'todo-2', step_id: 'step-2', description: 'Explore relevant code', status: 'in_progress', result: '' }
          ],
          activities: [
            { id: 'act-1', type: 'goal_created', text: 'Goal created: Implement Goal Mode' },
            { id: 'act-2', type: 'plan_created', text: 'Plan created with 5 steps' },
            { id: 'act-3', type: 'todo_started', text: 'Started: Understand goal and inspect context' },
            { id: 'act-4', type: 'todo_completed', text: 'Completed: Understand goal and inspect context' }
          ]
        })
      }
      if (url === '/api/projects/proj-1/goals') return Response.json({ goals: [] })
      if (url === '/api/projects/proj-1/goals/goal-1') {
        return Response.json({
          id: 'goal-1',
          project_id: 'proj-1',
          session_id: 's-1',
          agent_id: 'default',
          goal: 'Implement Goal Mode',
          status: 'running',
          plan: [
            { id: 'step-1', description: 'Understand goal and inspect context', subagent: 'planner' },
            { id: 'step-2', description: 'Explore relevant code', subagent: 'explorer' },
            { id: 'step-3', description: 'Implement focused changes', subagent: 'builder' },
            { id: 'step-4', description: 'Test and verify', subagent: 'tester' },
            { id: 'step-5', description: 'Review completion', subagent: 'reviewer' }
          ],
          todos: [
            { id: 'todo-1', step_id: 'step-1', description: 'Understand goal and inspect context', status: 'completed', result: 'Goal understood' },
            { id: 'todo-2', step_id: 'step-2', description: 'Explore relevant code', status: 'in_progress', result: '' }
          ],
          activities: [
            { id: 'act-1', type: 'goal_created', text: 'Goal created: Implement Goal Mode' },
            { id: 'act-2', type: 'plan_created', text: 'Plan created with 5 steps' },
            { id: 'act-3', type: 'todo_started', text: 'Started: Understand goal and inspect context' },
            { id: 'act-4', type: 'todo_completed', text: 'Completed: Understand goal and inspect context' }
          ]
        })
      }
      return Response.json({})
    }) as any

    render(<App initialWorkspaceTab="chat" />)
    await waitFor(() => expect(screen.getAllByText('Test Project').length).toBeGreaterThan(0))

    const projectSelect = screen.queryByDisplayValue('None')
    if (projectSelect) {
      fireEvent.change(projectSelect, { target: { value: 'proj-1' } })
    }

    const goalModeButton = screen.queryByText('Goal mode')
    if (goalModeButton) {
      fireEvent.click(goalModeButton)
    }

    const goalInput = await screen.findByPlaceholderText('Enter your goal...')
    fireEvent.change(goalInput, { target: { value: 'Implement Goal Mode' } })
    fireEvent.click(await screen.findByText('Start Goal'))

    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/projects/proj-1/goals' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ goal: 'Implement Goal Mode', agent_id: 'default' })
    })

    expect(await screen.findByText('Plan')).toBeTruthy()
    expect(await screen.findByText('Todos')).toBeTruthy()
    expect(await screen.findByText('Activity')).toBeTruthy()

    expect(await screen.findByText('planner')).toBeTruthy()
    expect((await screen.findAllByText('explorer')).length).toBeGreaterThanOrEqual(1)
    expect(await screen.findByText('builder')).toBeTruthy()
    expect(await screen.findByText('tester')).toBeTruthy()
    expect(await screen.findByText('reviewer')).toBeTruthy()

    expect((await screen.findAllByText('Goal created: Implement Goal Mode')).length).toBeGreaterThanOrEqual(1)
    expect(await screen.findByText('Plan created with 5 steps')).toBeTruthy()
  })

  it('shows subagent delegate activities in the goal activity panel', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Test Project', workspace_path: 'C:/test', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/projects/proj-1/goals') return Response.json({
        goals: [{
          id: 'goal-1',
          project_id: 'proj-1',
          goal: 'Implement feature',
          status: 'running'
        }]
      })
      if (url === '/api/projects/proj-1/goals/goal-1') {
        return Response.json({
          id: 'goal-1',
          project_id: 'proj-1',
          session_id: 's-1',
          agent_id: 'default',
          goal: 'Implement feature',
          status: 'running',
          plan: [
            { id: 'step-1', description: 'Explore codebase', subagent: 'explorer' }
          ],
          todos: [
            { id: 'todo-1', step_id: 'step-1', description: 'Explore codebase', status: 'completed', result: 'Found 5 relevant files' }
          ],
          activities: [
            { id: 'act-1', type: 'delegate_started', text: 'explorer started: Explore codebase', subagent_id: 'explorer' },
            { id: 'act-2', type: 'delegate_completed', text: 'explorer completed: Found 5 relevant files', subagent_id: 'explorer', result: 'Found 5 relevant files' },
            { id: 'act-3', type: 'todo_completed', text: 'Completed: Explore codebase' }
          ]
        })
      }
      return Response.json({})
    }) as any

    render(<App initialWorkspaceTab="chat" />)
    await waitFor(() => expect(screen.getAllByText('Test Project').length).toBeGreaterThan(0))

    const projectSelect2 = screen.queryByDisplayValue('None')
    if (projectSelect2) {
      fireEvent.change(projectSelect2, { target: { value: 'proj-1' } })
    }

    const goalModeButton2 = screen.queryByText('Goal mode')
    if (goalModeButton2) {
      fireEvent.click(goalModeButton2)
    }

    fireEvent.click(await screen.findByText('Implement feature'))

    expect(await screen.findByText('Activity')).toBeTruthy()
    expect(await screen.findByText('explorer started: Explore codebase')).toBeTruthy()
    expect(await screen.findByText('explorer completed: Found 5 relevant files')).toBeTruthy()
    expect((await screen.findAllByText('explorer')).length).toBeGreaterThanOrEqual(1)
  })

  it('stops a running goal from the goal panel', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [{ id: 'proj-1', name: 'Test Project', workspace_path: 'C:/test', temporary: false }] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/projects/proj-1/goals') return Response.json({
        goals: [{
          id: 'goal-1',
          project_id: 'proj-1',
          goal: 'Long running task',
          status: 'running'
        }]
      })
      if (url === '/api/projects/proj-1/goals/goal-1') {
        return Response.json({
          id: 'goal-1',
          project_id: 'proj-1',
          session_id: 's-1',
          agent_id: 'default',
          goal: 'Long running task',
          status: 'running',
          plan: [{ id: 'step-1', description: 'Do work', subagent: 'builder' }],
          todos: [{ id: 'todo-1', step_id: 'step-1', description: 'Do work', status: 'in_progress', result: '' }],
          activities: [{ id: 'act-1', type: 'todo_started', text: 'Started: Do work' }]
        })
      }
      if (url === '/api/projects/proj-1/goals/goal-1/stop' && init?.method === 'POST') {
        return Response.json({ status: 'stopping' })
      }
      return Response.json({})
    }) as any

    render(<App initialWorkspaceTab="chat" />)
    await waitFor(() => expect(screen.getAllByText('Test Project').length).toBeGreaterThan(0))

    const projectSelect3 = screen.queryByDisplayValue('None')
    if (projectSelect3) {
      fireEvent.change(projectSelect3, { target: { value: 'proj-1' } })
    }

    const goalModeButton3 = screen.queryByText('Goal mode')
    if (goalModeButton3) {
      fireEvent.click(goalModeButton3)
    }

    fireEvent.click(await screen.findByText('Long running task'))

    expect(await screen.findByText('Stop goal')).toBeTruthy()

    fireEvent.click(await screen.findByText('Stop goal'))

    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/projects/proj-1/goals/goal-1/stop' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
    })

    expect(await screen.findByText('stopping')).toBeTruthy()
  })

  it('lists agents and saves enabled subagents from agent settings', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [
        { id: 'default', name: 'Default Agent', enabled_subagents: [] },
        { id: 'coder', name: 'Coding Agent', enabled_subagents: ['builder'] }
      ] })
      if (url === '/api/subagents') return Response.json({ subagents: [
        { id: 'planner', name: 'Planner' },
        { id: 'builder', name: 'Builder' }
      ] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: ['auto'] })
      if (url === '/api/skills') return Response.json({ skills: [], diagnostics: [] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/agents' && init?.method === 'POST') return Response.json({ id: 'coder', name: 'Coding Agent' })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByRole('button', { name: 'Agents' }))
    expect(await screen.findByText('Coding Agent')).toBeTruthy()
    fireEvent.click(await screen.findByText('Coding Agent'))
    fireEvent.click(await screen.findByText('Edit'))
    fireEvent.click(await screen.findByLabelText('Planner'))
    fireEvent.click(await screen.findByText('Save'))

    await waitFor(() => {
      const save = calls.find(c => c.url === '/api/agents' && c.init?.method === 'POST')
      expect(save).toBeTruthy()
      expect(JSON.parse(String(save?.init?.body)).enabled_subagents).toContain('planner')
    })
  })

  it('shows built-in CLIProxyAPI extension status and actions', async () => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/extensions') return Response.json({ extensions: [{ id: 'cliproxyapi', name: 'CLIProxyAPI', built_in: true, installed: false, status: 'missing', binary_path: 'C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\cli-proxy-api.exe', proxy_url: 'http://127.0.0.1:8317/v1' }] })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: [] })
      if (url === '/api/skills') return Response.json({ skills: [] })
      return Response.json({})
    }) as any
    render(<App />)
    fireEvent.click(await screen.findByText('Extensions'))
    expect(await screen.findByText('CLIProxyAPI')).toBeTruthy()
    expect(await screen.findByText('Missing')).toBeTruthy()
    fireEvent.click(await screen.findByText('CLIProxyAPI'))
    expect(await screen.findByText('Missing Binary')).toBeTruthy()
    expect(await screen.findByText(/Copy the Windows test binary to this path/i)).toBeTruthy()
    expect((await screen.findByRole('button', { name: 'Start' }) as HTMLButtonElement).disabled).toBe(true)
    const binaryPaths = await screen.findAllByText('C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\cli-proxy-api.exe')
    expect(binaryPaths.length).toBeGreaterThan(0)
  })

  it('enables CLIProxyAPI start when the binary is installed and stopped', async () => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/extensions') return Response.json({ extensions: [{ id: 'cliproxyapi', name: 'CLIProxyAPI', built_in: true, installed: true, status: 'stopped', binary_path: 'C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\cli-proxy-api.exe', proxy_url: 'http://127.0.0.1:8317/v1' }] })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: [] })
      if (url === '/api/skills') return Response.json({ skills: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Extensions'))
    fireEvent.click(await screen.findByText('CLIProxyAPI'))

    expect((await screen.findByRole('button', { name: 'Start' }) as HTMLButtonElement).disabled).toBe(false)
    const workspace = document.querySelector('.workspace')
    expect(workspace).toBeTruthy()
    expect(workspace?.querySelector('.extensionDetail')).toBeTruthy()
  })

  it('shows CLIProxyAPI management unavailable when running panel URL is absent', async () => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/extensions') return Response.json({ extensions: [{ id: 'cliproxyapi', name: 'CLIProxyAPI', built_in: true, installed: true, status: 'running', binary_path: 'C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\cli-proxy-api.exe', management_path: 'C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\management.html', management_installed: false, proxy_url: 'http://127.0.0.1:8317/v1', port: 8317 }] })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Extensions'))
    fireEvent.click(await screen.findByText('CLIProxyAPI'))

    expect(await screen.findByText('Packaged Management Panel Missing')).toBeTruthy()
    const managementPaths = await screen.findAllByText('C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\management.html')
    expect(managementPaths.length).toBeGreaterThan(0)
    expect(screen.queryByText('Open Management Panel')).toBeNull()
  })

  it('applies CLIProxyAPI proxy credentials to Models Settings', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/extensions') return Response.json({ extensions: [{ id: 'cliproxyapi', name: 'CLIProxyAPI', built_in: true, installed: true, status: 'running', binary_path: 'C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\cli-proxy-api.exe', proxy_url: 'http://127.0.0.1:8317/v1', proxy_api_token: 'sk-uuagent-from-extension', port: 8317 }] })
      if (url === '/api/models/settings' && init?.method === 'PUT') return Response.json({ proxy_url: 'http://127.0.0.1:8317/v1', proxy_api_key: 'sk-uuagent-from-extension', fallback_tier: 'strong', routing_tiers: { fast: ['fast-model'], strong: ['strong-model'] }, model_ids: ['fast-model', 'strong-model'] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', proxy_api_key: '', fallback_tier: 'strong', routing_tiers: { fast: ['fast-model'], strong: ['strong-model'] }, model_ids: ['fast-model', 'strong-model'] })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url === '/api/skills') return Response.json({ skills: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Extensions'))
    fireEvent.click(await screen.findByText('CLIProxyAPI'))
    fireEvent.click(await screen.findByRole('button', { name: 'Use for Models' }))

    await waitFor(() => {
      const put = calls.find(c => c.url === '/api/models/settings' && c.init?.method === 'PUT')
      expect(put).toBeTruthy()
      expect(JSON.parse(String(put?.init?.body))).toMatchObject({
        proxy_url: 'http://127.0.0.1:8317/v1',
        proxy_api_key: 'sk-uuagent-from-extension',
        fallback_tier: 'strong',
        routing_tiers: { fast: ['fast-model'], strong: ['strong-model'] },
      })
    })
  })

  it('opens CLIProxyAPI management panel through the service URL when available', async () => {
    const copied: string[] = []
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn(async (value: string) => { copied.push(value) }) },
    })
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/extensions') return Response.json({ extensions: [{ id: 'cliproxyapi', name: 'CLIProxyAPI', built_in: true, installed: true, status: 'running', binary_path: 'C:\\Users\\15171\\.uuagent\\plugins\\cliproxyapi\\cli-proxy-api.exe', proxy_url: 'http://127.0.0.1:8317/v1', port: 8317, management_url: 'http://127.0.0.1:8317/management.html', management_secret: 'mgmt-1234567890abcdef', proxy_api_token: 'sk-uuagent-abcdef1234567890' }] })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Extensions'))
    fireEvent.click(await screen.findByText('CLIProxyAPI'))

    const link = await screen.findByText('Open Management Panel') as HTMLAnchorElement
    expect(link.href).toBe('http://127.0.0.1:8317/management.html')
    expect(await screen.findByText('Credentials')).toBeTruthy()
    expect(await screen.findByText('Management Login Key')).toBeTruthy()
    expect(await screen.findByText('Proxy API Token')).toBeTruthy()
    expect(await screen.findByText('mgmt-1••••••cdef')).toBeTruthy()
    expect(await screen.findByText('sk-uua••••••7890')).toBeTruthy()
    expect(screen.queryByText('mgmt-1234567890abcdef')).toBeNull()
    fireEvent.click(await screen.findByRole('button', { name: 'Copy Management Login Key' }))
    await waitFor(() => expect(copied).toEqual(['mgmt-1234567890abcdef']))
    fireEvent.click(await screen.findByRole('button', { name: 'Copy Proxy API Token' }))
    await waitFor(() => expect(copied).toEqual(['mgmt-1234567890abcdef', 'sk-uuagent-abcdef1234567890']))
  })

  it('concrete selected model sends model_override in /api/chat body', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: ['gpt-4o', 'claude-sonnet-4'] })
      if (url === '/api/chat') {
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode('data: {"type":"content","text":"ok"}\n\n'))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    await waitFor(() => expect(screen.getAllByText('gpt-4o').length).toBeGreaterThan(0))

    const modelSelect = screen.getAllByRole('combobox').find(el => el.getAttribute('aria-label')?.toLowerCase().includes('model'))
    if (modelSelect) {
      fireEvent.change(modelSelect, { target: { value: 'gpt-4o' } })
    }

    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'test message' } })
    fireEvent.click(screen.getByText('Send'))

    await waitFor(() => {
      const chatCall = calls.find(c => c.url === '/api/chat')
      expect(chatCall).toBeTruthy()
      const body = JSON.parse(String(chatCall?.init?.body))
      expect(body.model_override).toBe('gpt-4o')
    })
  })

  it('auto selected model omits model_override from /api/chat body', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const encoder = new TextEncoder()
    const calls: Array<{ url: string; init?: RequestInit }> = []
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: ['gpt-4o', 'claude-sonnet-4'] })
      if (url === '/api/chat') {
        const stream = new ReadableStream<Uint8Array>({
          start(c) {
            c.enqueue(encoder.encode('data: {"type":"content","text":"ok"}\n\n'))
            c.close()
          },
        })
        return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
      }
      return Response.json({})
    }) as any

    render(<App />)
    await waitFor(() => expect(screen.getAllByText('Auto').length).toBeGreaterThan(0))

    const modelSelect = screen.getAllByRole('combobox').find(el => el.getAttribute('aria-label')?.toLowerCase().includes('model'))
    if (modelSelect) {
      fireEvent.change(modelSelect, { target: { value: 'auto' } })
    }

    const input = await screen.findByPlaceholderText('Ask UUAgent to inspect, edit or explain code... Ctrl+Enter to send')
    fireEvent.change(input, { target: { value: 'test message' } })
    fireEvent.click(screen.getByText('Send'))

    await waitFor(() => {
      const chatCall = calls.find(c => c.url === '/api/chat')
      expect(chatCall).toBeTruthy()
      const body = JSON.parse(String(chatCall?.init?.body))
      expect(body.model_override).toBeUndefined()
    })
  })

  it('models route preview displays selected model source and rule from mocked /api/route', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      if (url === '/api/models/settings') return Response.json({ proxy_url: 'http://localhost:18463/v1', fallback_tier: 'strong', routing_tiers: {}, model_ids: ['gpt-4o-mini', 'gpt-4o'] })
      if (url.startsWith('/api/route')) {
        return Response.json({
          selected_model: 'gpt-4o-mini',
          selected_tier: 'fast',
          source: 'rule',
          rule_name: 'fast-simple',
          reason: 'pattern matched'
        })
      }
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByText('Settings'))
    fireEvent.click(await screen.findByText('Models'))

    expect(await screen.findByDisplayValue('http://localhost:18463/v1')).toBeTruthy()

    const promptInput = await screen.findByPlaceholderText('Enter prompt to preview routing...')
    fireEvent.change(promptInput, { target: { value: 'format this code' } })
    fireEvent.click(await screen.findByText('Preview Route'))

    await waitFor(() => {
      expect(screen.getByText((content) => content.includes('gpt-4o-mini'))).toBeTruthy()
      expect(screen.getByText((content) => content.includes('Tier:'))).toBeTruthy()
      expect(screen.getByText((content) => content.includes('Source:'))).toBeTruthy()
      expect(screen.getByText((content) => content.includes('Rule:'))).toBeTruthy()
      expect(screen.getByText((content) => content.includes('Reason:'))).toBeTruthy()
    })
  })

  it('navigation icon buttons keep accessible names for Projects Chat Extensions Schedules Settings', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)

    expect(await screen.findByRole('button', { name: 'Projects' })).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'Chat' })).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'Extensions' })).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'Schedules' })).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'Settings' })).toBeTruthy()
  })

  it('shows a project selection empty state when Chat opens without an active project', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async (url: string) => {
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/sessions') return Response.json({ sessions: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/sessions/')) return Response.json({ summaries: [] })
      return Response.json({})
    }) as any

    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: 'Chat' }))

    expect(await screen.findByText('Choose or create a project')).toBeTruthy()
    expect(await screen.findByText('Select a project from Projects before starting Chat.')).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'Open Projects' })).toBeTruthy()
  })

})
