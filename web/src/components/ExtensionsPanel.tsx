import type { ExtensionStatus } from '../types'
import { ExtensionDetail } from './ExtensionDetail'

interface ExtensionsPanelProps {
  extensions: ExtensionStatus[]
  selectedExtensionId?: string
  mode?: 'full' | 'list' | 'detail'
  onSelectExtension?: (id: string) => void
  onStartExtension?: (id: string) => Promise<void>
  onStopExtension?: (id: string) => Promise<void>
  onRestartExtension?: (id: string) => Promise<void>
  onUseForModels?: (extension: ExtensionStatus) => Promise<void>
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
  mode = 'full',
  onSelectExtension,
  onStartExtension,
  onStopExtension,
  onRestartExtension,
  onUseForModels,
}: ExtensionsPanelProps) {
  const selectedExtension = extensions.find(extension => extension.id === selectedExtensionId)
  const showList = mode !== 'detail'
  const showDetail = mode !== 'list'

  return (
    <div className={`extensionsPanel extensionsPanel-${mode}`}>
      {showList && (
        <div className="extensionsList">
          {extensions.length === 0 && <div className="emptyPanel">No extensions available.</div>}
          {extensions.map(extension => (
            <button
              key={extension.id}
              className={`extensionListItem ${selectedExtensionId === extension.id ? 'selected' : ''}`}
              onClick={() => onSelectExtension?.(extension.id)}
            >
              <div className="extensionListHeader">
                <span className="extensionName">{extension.name}</span>
                {extension.built_in && <span className="builtInBadge">Built-in</span>}
              </div>
              <div className="extensionListMeta">
                <span className={statusBadgeClass(extension.status)}>{statusLabel(extension.status)}</span>
                {!extension.installed && <span className="notInstalledBadge">Not installed</span>}
              </div>
            </button>
          ))}
        </div>
      )}

      {showDetail && selectedExtension && (
        <ExtensionDetail
          extension={selectedExtension}
          onStart={onStartExtension}
          onStop={onStopExtension}
          onRestart={onRestartExtension}
          onUseForModels={onUseForModels}
        />
      )}
    </div>
  )
}
