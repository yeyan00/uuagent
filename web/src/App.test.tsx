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
            c.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'tool_start', run_id: 'run-approval', tool_name: 'read', tool_id: 'call-1' })}\n\n`))
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
    expect(await screen.findByText('approved file contents')).toBeTruthy()
    expect(await screen.findByText('Running grep')).toBeTruthy()
    expect(await screen.findByText('matched important code')).toBeTruthy()
    expect(await screen.findByText('final streamed analysis')).toBeTruthy()
    await waitFor(() => expect(screen.getAllByText('Approved').length).toBeGreaterThanOrEqual(1))
    expect(await screen.findByRole('button', { name: 'Deny' })).toBeTruthy()
    expect(screen.queryByText('Done')).toBeNull()
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
        { role: 'tool', tool_name: 'list_dir', content: 'README.md\ninternal' },
        { role: 'tool', tool_name: 'read', content: 'module example' },
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
    expect(await screen.findByText('list_dir result')).toBeTruthy()
    expect(await screen.findByText('read result')).toBeTruthy()
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
})
