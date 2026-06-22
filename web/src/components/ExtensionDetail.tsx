import type { ExtensionStatus } from '../types'
import { CredentialsCard } from './CredentialsCard'

interface ExtensionDetailProps {
  extension: ExtensionStatus
  onStart?: (id: string) => Promise<void>
  onStop?: (id: string) => Promise<void>
  onRestart?: (id: string) => Promise<void>
  onUseForModels?: (extension: ExtensionStatus) => Promise<void>
}

const statusBadgeClass = (status: string): string => {
  switch (status) {
    case 'running':
      return 'statusBadge running'
    case 'starting':
      return 'statusBadge starting'
    case 'stopped':
      return 'statusBadge stopped'
    case 'missing':
      return 'statusBadge missing'
    case 'error':
      return 'statusBadge error'
    default:
      return 'statusBadge'
  }
}

const statusLabel = (status: string): string => {
  switch (status) {
    case 'running':
      return 'Running'
    case 'starting':
      return 'Starting'
    case 'stopped':
      return 'Stopped'
    case 'missing':
      return 'Missing'
    case 'error':
      return 'Error'
    default:
      return status
  }
}

export function ExtensionDetail({ extension, onStart, onStop, onRestart, onUseForModels }: ExtensionDetailProps) {
  const isMissing = extension.status === 'missing' || extension.installed === false
  const isRunning = extension.status === 'running'
  const isStarting = extension.status === 'starting'
  const isStoppedLike = extension.status === 'stopped' || extension.status === 'error'
  const canStart = isStoppedLike && !isMissing
  const canStop = isRunning || isStarting
  const canRestart = isRunning

  return (
    <div className="extensionDetail">
      <div className="extensionDetailHeader">
        <div>
          <h3>{extension.name}</h3>
          {extension.description && <p className="extensionDescription">{extension.description}</p>}
        </div>
        <div className="extensionActions">
          <button className="btn-primary" onClick={() => onStart?.(extension.id)} disabled={!canStart}>Start</button>
          {canStop && <button className="btn-secondary" onClick={() => onStop?.(extension.id)}>Stop</button>}
          {canRestart && <button className="btn-secondary" onClick={() => onRestart?.(extension.id)}>Restart</button>}
          {extension.proxy_url && extension.proxy_api_token && (
            <button className="btn-secondary" onClick={() => onUseForModels?.(extension)}>Use for Models</button>
          )}
        </div>
      </div>

      <div className="extensionCards">
        <section className="extensionStatusCard extensionSummaryCard">
          <h4>Service Status</h4>
          <div className="statusRow">
            <span className="statusLabel">Status</span>
            <span className={statusBadgeClass(extension.status)}>{statusLabel(extension.status)}</span>
          </div>
          {extension.last_error && (
            <div className="statusRow errorRow">
              <span className="statusLabel">Last Error</span>
              <pre className="errorValue">{extension.last_error}</pre>
            </div>
          )}
        </section>

        <section className="extensionStatusCard">
          <h4>Connection</h4>
          {extension.port && <div className="statusRow"><span className="statusLabel">Port</span><span className="statusValue">{extension.port}</span></div>}
          {extension.proxy_url && (
            <div className="statusRow">
              <span className="statusLabel">Proxy URL</span>
              <div className="statusValueWithAction">
                <code className="statusValue">{extension.proxy_url}</code>
                <button className="copyButton" onClick={() => navigator.clipboard.writeText(extension.proxy_url || '')} title="Copy to clipboard">Copy</button>
              </div>
            </div>
          )}
          {extension.status === 'running' && extension.management_url && (
            <div className="statusRow">
              <span className="statusLabel">Management</span>
              <div className="statusValueWithAction"><a href={extension.management_url} target="_blank" rel="noopener noreferrer" className="managementLink">Open Management Panel</a></div>
            </div>
          )}
          {extension.status === 'running' && !extension.management_url && extension.management_path && extension.management_installed === false && (
            <div className="extensionGuidance compactGuidance"><h4>Packaged Management Panel Missing</h4><p>Place management.html beside cli-proxy-api.exe to enable the offline management panel. UUAgent disables CLIProxyAPI runtime panel downloads.</p><code>{extension.management_path}</code></div>
          )}
          {extension.status === 'running' && !extension.management_url && (!extension.management_path || extension.management_installed !== false) && (
            <div className="extensionGuidance compactGuidance"><h4>Management Panel Unavailable</h4><p>CLIProxyAPI is running, but its management panel endpoint is not reachable. Use the Proxy URL above for model routing.</p></div>
          )}
        </section>

        <CredentialsCard managementSecret={extension.management_secret} proxyAPIToken={extension.proxy_api_token} proxyURL={extension.proxy_url} />

        <section className="extensionStatusCard">
          <h4>Files</h4>
          {extension.binary_path && <div className="statusRow"><span className="statusLabel">Binary Path</span><code className="statusValue">{extension.binary_path}</code></div>}
          {extension.config_path && <div className="statusRow"><span className="statusLabel">Config Path</span><code className="statusValue">{extension.config_path}</code></div>}
          {extension.management_path && <div className="statusRow"><span className="statusLabel">Management Panel Path</span><code className="statusValue">{extension.management_path}</code></div>}
        </section>

        {extension.status === 'missing' && extension.binary_path && (
          <section className="extensionGuidance">
            <h4>Missing Binary</h4>
            <p>CLIProxyAPI is managed by UUAgent, but the executable is not present yet. Copy the Windows test binary to this path, then refresh Extensions and click Start.</p>
            <code>{extension.binary_path}</code>
          </section>
        )}
      </div>
    </div>
  )
}
