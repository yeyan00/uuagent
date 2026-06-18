import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import App from './App'

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
    fireEvent.click(await screen.findByText('Agents'))
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
    expect(await screen.findByText('Chat')).toBeTruthy()
    expect(await screen.findByText('Stop')).toBeTruthy()
    controller.enqueue(encoder.encode('data: {"type":"content","text":" after settings"}\n\n'))
    fireEvent.click(await screen.findByText('Chat'))
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
    fireEvent.click(await screen.findByText('Agents'))
    expect(await screen.findByText('All skills')).toBeTruthy()
    fireEvent.click(await screen.findByLabelText('review'))
    fireEvent.click(await screen.findByText('Save Agent'))
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/agents' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ enabled_skills: ['review'] })
    })
  })

  it('manages subagent skill selection from Settings Subagents', async () => {
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
    fireEvent.click(await screen.findByText('Subagents'))
    fireEvent.click(await screen.findByText('Reviewer'))
    fireEvent.click(await screen.findByLabelText('review'))
    fireEvent.click(await screen.findByText('Save Subagent'))
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/subagents' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ id: 'reviewer', enabled_skills: ['review'] })
    })
  })

  it('forces a single selected skill when sending from composer', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const calls: string[] = []
    const encoder = new TextEncoder()
    globalThis.fetch = vi.fn(async (url: string) => {
      calls.push(url)
      if (url === '/api/projects') return Response.json({ projects: [] })
      if (url === '/api/agents') return Response.json({ agents: [{ id: 'default', name: 'Default Agent' }] })
      if (url === '/api/subagents') return Response.json({ subagents: [] })
      if (url === '/api/skills') return Response.json({ skills: [{ name: 'review', description: 'Review code', enabled: true, scope: 'global' }], diagnostics: [] })
      if (url === '/api/memory') return Response.json({ memories: [] })
      if (url.startsWith('/api/chat')) {
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
    await waitFor(() => expect(calls.some(url => url.startsWith('/api/chat') && decodeURIComponent(url).includes('/skill:review inspect code'))).toBe(true))
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
    fireEvent.click(await screen.findByText('Subagents'))
    fireEvent.click(await screen.findByText('New Subagent'))
    fireEvent.change(await screen.findByDisplayValue(/subagent-/), { target: { value: 'reviewer' } })
    fireEvent.change(await screen.findByPlaceholderText('Subagent name'), { target: { value: 'Reviewer' } })
    fireEvent.change(await screen.findByPlaceholderText('empty = route automatically'), { target: { value: 'sub-model' } })
    fireEvent.change(await screen.findByPlaceholderText('read, grep, shell'), { target: { value: 'read, grep' } })
    fireEvent.change(await screen.findByPlaceholderText('mock'), { target: { value: 'mock' } })
    fireEvent.change(await screen.findByPlaceholderText('ask'), { target: { value: 'ask' } })
    fireEvent.click(await screen.findByLabelText('review'))
    fireEvent.click(await screen.findByText('Save Subagent'))
    await waitFor(() => {
      const post = calls.find(c => c.url === '/api/subagents' && c.init?.method === 'POST')
      expect(post).toBeTruthy()
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ id: 'reviewer', name: 'Reviewer', model: 'sub-model', enabled_tools: ['read', 'grep'], enabled_mcp_servers: ['mock'], permission_mode: 'ask', enabled_skills: ['review'] })
    })
    fireEvent.click(await screen.findByText('Delete Subagent'))
    await waitFor(() => expect(calls.some(c => c.url === '/api/subagents/reviewer' && c.init?.method === 'DELETE')).toBe(true))
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
      expect(JSON.parse(String(post?.init?.body))).toMatchObject({ proxy_url: 'http://localhost:8080/v1' })
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
})
