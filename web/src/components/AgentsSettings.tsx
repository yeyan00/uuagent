import { useState } from 'react';
import type { AgentProfile, SubagentProfile } from '../types';

interface AgentsSettingsProps {
  agents: AgentProfile[];
  subagents: SubagentProfile[];
  onSave: (agent: AgentProfile) => Promise<void>;
  onDelete: (agentId: string) => Promise<void>;
  onCreate: () => Promise<void>;
}

export function AgentsSettings({ agents, subagents, onSave, onDelete, onCreate }: AgentsSettingsProps) {
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(agents.length > 0 ? agents[0].id : null);
  const [isEditing, setIsEditing] = useState(false);
  const [editedAgent, setEditedAgent] = useState<AgentProfile | null>(null);

  const selectedAgent = agents.find(a => a.id === selectedAgentId) || null;

  const handleEdit = () => {
    if (selectedAgent) {
      setEditedAgent({ ...selectedAgent });
      setIsEditing(true);
    }
  };

  const handleCancel = () => {
    setIsEditing(false);
    setEditedAgent(null);
  };

  const handleSave = async () => {
    if (editedAgent) {
      await onSave(editedAgent);
      setIsEditing(false);
      setEditedAgent(null);
    }
  };

  const handleToggleSubagent = (subagentId: string) => {
    if (!editedAgent) return;
    const currentEnabled = editedAgent.enabled_subagents || [];
    const newEnabled = currentEnabled.includes(subagentId)
      ? currentEnabled.filter(id => id !== subagentId)
      : [...currentEnabled, subagentId];
    setEditedAgent({ ...editedAgent, enabled_subagents: newEnabled });
  };

  const handleCreate = async () => {
    await onCreate();
  };

  const handleDelete = async () => {
    if (selectedAgent && confirm(`Delete agent "${selectedAgent.name}"?`)) {
      await onDelete(selectedAgent.id);
      if (agents.length > 1) {
        const remaining = agents.filter(a => a.id !== selectedAgent.id);
        setSelectedAgentId(remaining[0]?.id || null);
      } else {
        setSelectedAgentId(null);
      }
    }
  };

  return (
    <div className="agents-settings">
      <div className="settings-header">
        <h2>Agents</h2>
        <button className="btn-primary" onClick={handleCreate}>+ New Agent</button>
      </div>
      
      <div className="settings-content">
        <div className="settings-list">
          {agents.map(agent => (
            <div
              key={agent.id}
              className={`settings-list-item ${selectedAgentId === agent.id ? 'selected' : ''}`}
              onClick={() => {
                setSelectedAgentId(agent.id);
                setIsEditing(false);
                setEditedAgent(null);
              }}
            >
              <div className="item-name">{agent.name}</div>
              <div className="item-description">{agent.description || 'No description'}</div>
            </div>
          ))}
        </div>

        <div className="settings-detail">
          {selectedAgent ? (
            isEditing && editedAgent ? (
              <div className="detail-form">
                <div className="form-group">
                  <label>Name</label>
                  <input
                    type="text"
                    value={editedAgent.name}
                    onChange={e => setEditedAgent({ ...editedAgent, name: e.target.value })}
                  />
                </div>
                <div className="form-group">
                  <label>Description</label>
                  <textarea
                    value={editedAgent.description || ''}
                    onChange={e => setEditedAgent({ ...editedAgent, description: e.target.value })}
                    rows={3}
                  />
                </div>
                <div className="form-group">
                  <label>System Prompt</label>
                  <textarea
                    value={editedAgent.system_prompt || ''}
                    onChange={e => setEditedAgent({ ...editedAgent, system_prompt: e.target.value })}
                    rows={5}
                  />
                </div>
                <div className="form-row">
                  <div className="form-group">
                    <label>Model</label>
                    <input
                      type="text"
                      value={editedAgent.model || ''}
                      onChange={e => setEditedAgent({ ...editedAgent, model: e.target.value })}
                    />
                  </div>
                  <div className="form-group">
                    <label>Temperature</label>
                    <input
                      type="number"
                      step="0.1"
                      min="0"
                      max="2"
                      value={editedAgent.temperature ?? ''}
                      onChange={e => setEditedAgent({ ...editedAgent, temperature: parseFloat(e.target.value) })}
                    />
                  </div>
                </div>
                
                <div className="form-group">
                  <label>Enabled Subagents</label>
                  <div className="subagent-checkboxes">
                    {subagents.length === 0 ? (
                      <p className="no-subagents">No subagents available</p>
                    ) : (
                      subagents.map(subagent => (
                        <div key={subagent.id} className="checkbox-item">
                          <input
                            type="checkbox"
                            id={`subagent-${subagent.id}`}
                            checked={(editedAgent.enabled_subagents || []).includes(subagent.id)}
                            onChange={() => handleToggleSubagent(subagent.id)}
                          />
                          <label htmlFor={`subagent-${subagent.id}`}>{subagent.name}</label>
                        </div>
                      ))
                    )}
                  </div>
                </div>

                <div className="form-actions">
                  <button className="btn-secondary" onClick={handleCancel}>Cancel</button>
                  <button className="btn-primary" onClick={handleSave}>Save</button>
                </div>
              </div>
            ) : (
              <div className="detail-view">
                <div className="detail-header">
                  <h3>{selectedAgent.name}</h3>
                  <div className="detail-actions">
                    <button className="btn-secondary" onClick={handleEdit}>Edit</button>
                    <button className="btn-danger" onClick={handleDelete}>Delete</button>
                  </div>
                </div>
                <div className="detail-section">
                  <label>Description</label>
                  <p>{selectedAgent.description || 'No description'}</p>
                </div>
                <div className="detail-section">
                  <label>System Prompt</label>
                  <pre>{selectedAgent.system_prompt || 'No system prompt'}</pre>
                </div>
                <div className="detail-row">
                  <div className="detail-section">
                    <label>Model</label>
                    <p>{selectedAgent.model || 'Default'}</p>
                  </div>
                  <div className="detail-section">
                    <label>Temperature</label>
                    <p>{selectedAgent.temperature ?? 'Default'}</p>
                  </div>
                </div>
                <div className="detail-section">
                  <label>Enabled Subagents</label>
                  {selectedAgent.enabled_subagents && selectedAgent.enabled_subagents.length > 0 ? (
                    <ul className="enabled-subagents-list">
                      {selectedAgent.enabled_subagents.map(subagentId => {
                        const subagent = subagents.find(s => s.id === subagentId);
                        return (
                          <li key={subagentId}>{subagent?.name || subagentId}</li>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="no-subagents">No subagents enabled</p>
                  )}
                </div>
              </div>
            )
          ) : (
            <div className="empty-state">
              <p>No agent selected</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
