import { useState, useEffect, useRef } from 'react'

interface ChatEvent {
  type: string
  model?: string
  tier?: string
  text?: string
  tool_name?: string
  tool_id?: string
}

interface Message {
  role: 'user' | 'assistant' | 'system'
  content: string
  model?: string
  tier?: string
}

function App() {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [routeInfo, setRouteInfo] = useState<{ model: string; tier: string } | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const sendMessage = async () => {
    if (!input.trim() || isStreaming) return

    const userMsg: Message = { role: 'user', content: input }
    setMessages(prev => [...prev, userMsg])
    setInput('')
    setIsStreaming(true)
    setRouteInfo(null)

    try {
      const response = await fetch(`/api/chat?prompt=${encodeURIComponent(input)}&session_id=default`)
      const reader = response.body?.getReader()
      const decoder = new TextDecoder()

      if (!reader) return

      let assistantContent = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        const chunk = decoder.decode(value)
        const lines = chunk.split('\n')

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          try {
            const evt: ChatEvent = JSON.parse(line.slice(6))

            if (evt.type === 'route') {
              setRouteInfo({ model: evt.model || '', tier: evt.tier || '' })
            } else if (evt.type === 'content') {
              assistantContent += evt.text || ''
              setMessages(prev => {
                const updated = [...prev]
                const last = updated[updated.length - 1]
                if (last?.role === 'assistant') {
                  updated[updated.length - 1] = { ...last, content: assistantContent }
                } else {
                  updated.push({
                    role: 'assistant',
                    content: assistantContent,
                    model: routeInfo?.model,
                    tier: routeInfo?.tier,
                  })
                }
                return updated
              })
            }
          } catch { /* skip invalid JSON */ }
        }
      }
    } catch (err) {
      setMessages(prev => [...prev, { role: 'system', content: `Error: ${err}` }])
    } finally {
      setIsStreaming(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', maxWidth: 900, margin: '0 auto' }}>
      {/* Header */}
      <header style={{ padding: '12px 16px', borderBottom: '1px solid #333', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h1 style={{ margin: 0, fontSize: 18 }}>🤖 UUAgent</h1>
        <div style={{ display: 'flex', gap: 12, fontSize: 13 }}>
          <a href="/ui/" style={{ color: '#6af' }}>Chat</a>
          <a href="/management.html" style={{ color: '#6af' }}>⚙ Models</a>
        </div>
      </header>

      {/* Route Info */}
      {routeInfo && (
        <div style={{ padding: '6px 16px', background: '#1a1a2e', fontSize: 12, color: '#aaa' }}>
          ⚡ Smart Route: <b style={{ color: '#6f6' }}>{routeInfo.model}</b> (tier: {routeInfo.tier})
        </div>
      )}

      {/* Messages */}
      <div style={{ flex: 1, overflow: 'auto', padding: 16 }}>
        {messages.map((msg, i) => (
          <div key={i} style={{
            marginBottom: 12,
            padding: '8px 12px',
            borderRadius: 8,
            background: msg.role === 'user' ? '#1a3a5c' : '#1a1a2e',
            alignSelf: msg.role === 'user' ? 'flex-end' : 'flex-start',
          }}>
            <div style={{ fontSize: 11, color: '#888', marginBottom: 4 }}>
              {msg.role === 'user' ? '👤 You' : `🤖 ${msg.model || 'Assistant'}`}
            </div>
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontFamily: 'inherit' }}>{msg.content}</pre>
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>

      {/* Input — 始终可操作，不会锁定 */}
      <div style={{ padding: 12, borderTop: '1px solid #333', display: 'flex', gap: 8 }}>
        <input
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() } }}
          placeholder="Type a message... (Enter to send)"
          disabled={isStreaming}
          style={{
            flex: 1, padding: '10px 14px', borderRadius: 8,
            background: '#1a1a2e', color: '#eee', border: '1px solid #444',
            fontSize: 14, outline: 'none',
          }}
        />
        <button
          onClick={sendMessage}
          disabled={isStreaming || !input.trim()}
          style={{
            padding: '10px 20px', borderRadius: 8,
            background: isStreaming ? '#333' : '#2563eb', color: '#fff',
            border: 'none', cursor: isStreaming ? 'not-allowed' : 'pointer',
            fontSize: 14,
          }}
        >
          {isStreaming ? '...' : 'Send'}
        </button>
      </div>
    </div>
  )
}

export default App
