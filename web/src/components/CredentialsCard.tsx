import { useState } from 'react'

interface CredentialsCardProps {
  managementSecret?: string
  proxyAPIToken?: string
  proxyURL?: string
}

const maskedCredential = (value: string): string => {
  if (value.length <= 12) return value
  return `${value.slice(0, 6)}••••••${value.slice(-4)}`
}

export function CredentialsCard({ managementSecret, proxyAPIToken, proxyURL }: CredentialsCardProps) {
  const [copiedCredential, setCopiedCredential] = useState('')

  const copyCredential = async (label: string, value?: string) => {
    if (!value) return
    await navigator.clipboard.writeText(value)
    setCopiedCredential(label)
  }

  return (
    <section className="extensionStatusCard">
      <h4>Credentials</h4>
      {managementSecret ? (
        <div className="credentialRow">
          <div className="credentialMeta">
            <span className="statusLabel">Management Login Key</span>
            <small>用于打开 CLIProxyAPI 管理面板</small>
          </div>
          <div className="statusValueWithAction credentialValue">
            <code className="statusValue">{maskedCredential(managementSecret)}</code>
            <button
              className="copyButton"
              aria-label="Copy Management Login Key"
              onClick={() => copyCredential('management', managementSecret)}
            >
              {copiedCredential === 'management' ? 'Copied' : 'Copy'}
            </button>
          </div>
        </div>
      ) : (
        <div className="credentialEmpty">Start CLIProxyAPI once to generate the management login key.</div>
      )}
      {proxyAPIToken ? (
        <div className="credentialRow">
          <div className="credentialMeta">
            <span className="statusLabel">Proxy API Token</span>
            <small>用于客户端访问 {proxyURL || 'CLIProxyAPI /v1'}</small>
          </div>
          <div className="statusValueWithAction credentialValue">
            <code className="statusValue">{maskedCredential(proxyAPIToken)}</code>
            <button
              className="copyButton"
              aria-label="Copy Proxy API Token"
              onClick={() => copyCredential('proxy', proxyAPIToken)}
            >
              {copiedCredential === 'proxy' ? 'Copied' : 'Copy'}
            </button>
          </div>
        </div>
      ) : (
        <div className="credentialEmpty">Start CLIProxyAPI once to generate the proxy API token.</div>
      )}
    </section>
  )
}
