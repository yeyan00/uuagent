import type { ExtensionStatus } from '../types'

interface ExtensionsPanelProps {
  extensions: ExtensionStatus[]
  selectedExtensionId?: string
  onSelectExtension?: (id: string) => void
  onStartExtension?: (id: string) => Promise<void>
  onStopExtension?: (id: string) => Promise<void>
  onRestartExtension?: (id: string) => Promise<void>
  onFetchLogs?: (id: string) => Promise<string[]>
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

export function ExtensionsPanel({
  extensions,
  selectedExtensionId,
  onSelectExtension,
  onStartExtension,
  onStopExtension,
  onRestartExtension,
}: ExtensionsPanelProps) {
  const selectedExtension = extensions.find(e => e.id === selectedExtensionId)

  const handleStart = async () => {
    if (selectedExtensionId && onStartExtension) {
      await onStartExtension(selectedExtensionId)
    }
  }

  const handleStop = async () => {
    if (selectedExtensionId && onStopExtension) {
      await onStopExtension(selectedExtensionId)
    }
  }

  const handleRestart = async () => {
    if (selectedExtensionId && onRestartExtension) {
      await onRestartExtension(selectedExtensionId)
    }
  }

  const isMissing = selectedExtension?.status === 'missing' || selectedExtension?.installed === false
  const isRunning = selectedExtension?.status === 'running'
  const isStarting = selectedExtension?.status === 'starting'
  const isStoppedLike = selectedExtension?.status === 'stopped' || selectedExtension?.status === 'error'
  const canStart = Boolean(selectedExtension && isStoppedLike && !isMissing)
  const canStop = Boolean(selectedExtension && (isRunning || isStarting))
  const canRestart = Boolean(selectedExtension && isRunning)

  return (
    <div className="extensionsPanel">
      <div className="extensionsList">
        {extensions.length === 0 && (
          <div className="emptyPanel">No extensions available.</div>
        )}
        {extensions.map(ext => (
          <button
            key={ext.id}
            className={`extensionListItem ${selectedExtensionId === ext.id ? 'selected' : ''}`}
            onClick={() => onSelectExtension?.(ext.id)}
          >
            <div className="extensionListHeader">
              <span className="extensionName">{ext.name}</span>
              {ext.built_in && <span className="builtInBadge">Built-in</span>}
            </div>
            <div className="extensionListMeta">
              <span className={statusBadgeClass(ext.status)}>
                {statusLabel(ext.status)}
              </span>
              {!ext.installed && <span className="notInstalledBadge">Not installed</span>}
            </div>
          </button>
        ))}
      </div>

      {selectedExtension && (
        <div className="extensionDetail">
          <div className="extensionDetailHeader">
            <div>
              <h3>{selectedExtension.name}</h3>
              {selectedExtension.description && (
                <p className="extensionDescription">{selectedExtension.description}</p>
              )}
            </div>
            <div className="extensionActions">
              <button className="btn-primary" onClick={handleStart} disabled={!canStart}>
                Start
              </button>
              {canStop && (
                <button className="btn-secondary" onClick={handleStop}>
                  Stop
                </button>
              )}
              {canRestart && (
                <button className="btn-secondary" onClick={handleRestart}>
                  Restart
                </button>
              )}
            </div>
          </div>

          <div className="extensionStatusCard">
            <div className="statusRow">
              <span className="statusLabel">Status</span>
              <span className={statusBadgeClass(selectedExtension.status)}>
                {statusLabel(selectedExtension.status)}
              </span>
            </div>

            {selectedExtension.binary_path && (
              <div className="statusRow">
                <span className="statusLabel">Binary Path</span>
                <code className="statusValue">{selectedExtension.binary_path}</code>
              </div>
            )}

            {selectedExtension.config_path && (
              <div className="statusRow">
                <span className="statusLabel">Config Path</span>
                <code className="statusValue">{selectedExtension.config_path}</code>
              </div>
            )}

            {selectedExtension.port && (
              <div className="statusRow">
                <span className="statusLabel">Port</span>
                <span className="statusValue">{selectedExtension.port}</span>
              </div>
            )}

            {selectedExtension.proxy_url && (
              <div className="statusRow">
                <span className="statusLabel">Proxy URL</span>
                <div className="statusValueWithAction">
                  <code className="statusValue">{selectedExtension.proxy_url}</code>
                  <button
                    className="copyButton"
                    onClick={() => navigator.clipboard.writeText(selectedExtension.proxy_url || '')}
                    title="Copy to clipboard"
                  >
                    Copy
                  </button>
                </div>
              </div>
            )}

            {selectedExtension.management_url && selectedExtension.status === 'running' && (
              <div className="statusRow">
                <span className="statusLabel">Management</span>
                <div className="statusValueWithAction">
                  <a
                    href={selectedExtension.management_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="managementLink"
                  >
                    Open Management Panel
                  </a>
                </div>
              </div>
            )}

            {selectedExtension.last_error && (
              <div className="statusRow errorRow">
                <span className="statusLabel">Last Error</span>
                <pre className="errorValue">{selectedExtension.last_error}</pre>
              </div>
            )}
          </div>

          {selectedExtension.status === 'missing' && selectedExtension.binary_path && (
            <div className="extensionGuidance">
              <h4>Missing Binary</h4>
              <p>
                CLIProxyAPI is managed by UUAgent, but the executable is not present yet.
                Copy the Windows test binary to this path, then refresh Extensions and click Start.
              </p>
              <code>{selectedExtension.binary_path}</code>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
