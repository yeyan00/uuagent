import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
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
})
