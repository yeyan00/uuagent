import type { ModelsSettings, ModelsTestResult, RoutePreviewResult } from '../types'

interface ModelsSettingsPanelProps {
  modelsDraft: ModelsSettings | null
  modelsTestResult: ModelsTestResult | null
  routePreviewPrompt: string
  routePreviewResult: RoutePreviewResult | null
  onUpdateDraft: (patch: Partial<ModelsSettings>) => void
  onUpdateRoutingTier: (tier: string, value: string) => void
  onSave: () => Promise<void>
  onTestConnection: () => Promise<void>
  onRoutePreviewPromptChange: (value: string) => void
  onPreviewRoute: () => Promise<void>
}

const joinList = (value?: string[]) => (value || []).join(', ')

const tierDescription = (tier: string): string => {
  switch (tier) {
    case 'fast':
      return 'Low-latency work such as formatting, renames, short explanations, and simple edits.'
    case 'strong':
      return 'Default coding and debugging tier for implementation, tests, and multi-step reasoning.'
    case 'large_ctx':
      return 'Long-context tier used when prompts or sessions are too large for normal models.'
    default:
      return 'Custom routing tier. Models are tried in the order listed.'
  }
}

export function ModelsSettingsPanel({
  modelsDraft,
  modelsTestResult,
  routePreviewPrompt,
  routePreviewResult,
  onUpdateDraft,
  onUpdateRoutingTier,
  onSave,
  onTestConnection,
  onRoutePreviewPromptChange,
  onPreviewRoute,
}: ModelsSettingsPanelProps) {
  if (!modelsDraft) {
    return <div className="emptyPanel">Loading models settings...</div>
  }

  return (
    <div className="modelsSettingsPanel">
      <div className="settingsPageHeader">
        <div>
          <strong>Models</strong>
          <p>Connect UUAgent to an OpenAI-compatible proxy, choose the fallback tier, and tune deterministic routing.</p>
        </div>
      </div>

      <section className="settingsCard modelsConnectionCard">
        <div className="settingsCardHeader">
          <div>
            <h3>Proxy Connection</h3>
            <p>Use CLIProxyAPI or another OpenAI-compatible endpoint for all model requests.</p>
          </div>
        </div>
        <div className="settingsGrid">
          <label className="wide">
            Proxy URL
            <input value={modelsDraft.proxy_url} onChange={event => onUpdateDraft({ proxy_url: event.target.value })} placeholder="http://127.0.0.1:8317/v1" />
          </label>
          <label className="wide">
            Proxy API Token
            <input value={modelsDraft.proxy_api_key || ''} onChange={event => onUpdateDraft({ proxy_api_key: event.target.value })} placeholder="sk-uuagent-..." />
          </label>
          <label>
            Fallback Tier
            <input value={modelsDraft.fallback_tier} onChange={event => onUpdateDraft({ fallback_tier: event.target.value })} placeholder="strong" />
          </label>
          <div className="settingsActions wide">
            <button className="primaryButton" onClick={onSave}>Save Settings</button>
            <button onClick={onTestConnection}>Test Connection</button>
          </div>
        </div>
        {modelsTestResult && <div className={`testResult ${modelsTestResult.success ? 'success' : 'error'}`}>
          {modelsTestResult.success ? (
            <><strong>Connection successful</strong><p>Available models: {joinList(modelsTestResult.model_ids)}</p></>
          ) : (
            <><strong>Connection failed</strong><p>{modelsTestResult.error || 'Unknown error'}</p></>
          )}
        </div>}
      </section>

      <section className="settingsCard modelsRoutingCard">
        <div className="settingsCardHeader">
          <div>
            <h3>Routing Tiers</h3>
            <p>UUAgent routes by deterministic token and keyword rules. The first available model in the chosen tier is used.</p>
          </div>
        </div>
        <div className="tierGrid">
          {Object.entries(modelsDraft.routing_tiers || {}).map(([tier, models]) => (
            <label key={tier} className="tierCard">
              <span>{tier}</span>
              <small>{tierDescription(tier)}</small>
              <textarea value={joinList(models)} onChange={event => onUpdateRoutingTier(tier, event.target.value)} placeholder="model-1, model-2" />
            </label>
          ))}
        </div>
      </section>

      <section className="settingsCard routePreviewCard">
        <div className="settingsCardHeader">
          <div>
            <h3>Route Preview</h3>
            <p>Preview which tier/model the current routing rules choose for a prompt.</p>
          </div>
        </div>
        <div className="settingsGrid">
          <label className="wide">
            Prompt
            <input value={routePreviewPrompt} onChange={event => onRoutePreviewPromptChange(event.target.value)} placeholder="Enter prompt to preview routing..." />
          </label>
          <div className="settingsActions wide">
            <button onClick={onPreviewRoute}>Preview Route</button>
          </div>
        </div>
        {routePreviewResult && <div className="testResult">
          <strong>Selected: {routePreviewResult.selected_model || 'N/A'}</strong>
          <p>Tier: {routePreviewResult.selected_tier || 'N/A'}</p>
          <p>Source: {routePreviewResult.source || 'N/A'}</p>
          {routePreviewResult.rule_name && <p>Rule: {routePreviewResult.rule_name}</p>}
          {routePreviewResult.reason && <p>Reason: {routePreviewResult.reason}</p>}
        </div>}
      </section>
    </div>
  )
}
