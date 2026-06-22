import { useState } from 'react';
import type { AgentProfile, SubagentProfile } from '../types';

interface SubagentsSettingsProps {
  subagents: SubagentProfile[];
  agents: AgentProfile[];
  onSave: (subagent: SubagentProfile) => Promise<void>;
  onDelete: (subagentId: string) => Promise<void>;
  onCreate: () => Promise<void>;
}

export function SubagentsSettings({ subagents, agents, onSave, onDelete, onCreate }: SubagentsSettingsProps) {
  const [selectedSubagentId, setSelectedSubagentId] = useState<string | null>(subagents.length > 0 ? subagents[0].id : null);
  const [isEditing, setIsEditing] = useState(false);
  const [editedSubagent, setEditedSubagent] = useState<SubagentProfile | null>(null);

  const selectedSubagent = subagents.find(s => s.id === selectedSubagentId) || null;

  const handleEdit = () => {
    if (selectedSubagent) {
      setEditedSubagent({ ...selectedSubagent });
      setIsEditing(true);
    }
  };

  const handleCancel = () => {
    setIsEditing(false);
    setEditedSubagent(null);
  };

  const handleSave = async () => {
    if (editedSubagent) {
      await onSave(editedSubagent);
      setIsEditing(false);
      setEditedSubagent(null);
    }
  };

  const handleCreate = async () => {
    await onCreate();
  };

  const handleDelete = async () => {
    if (selectedSubagent && confirm(`Delete subagent "${selectedSubagent.name}"?`)) {
      try {
        await onDelete(selectedSubagent.id);
        if (subagents.length > 1) {
          const remaining = subagents.filter(s => s.id !== selectedSubagent.id);
          setSelectedSubagentId(remaining[0]?.id || null);
        } else {
          setSelectedSubagentId(null);
        }
      } catch {
        // Keep current selection if delete fails
      }
    }
  };

  const getUsedByAgents = (subagentId: string): AgentProfile[] => {
    return agents.filter(agent => 
      agent.enabled_subagents?.includes(subagentId)
    );
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Never';
    const date = new Date(dateString);
    return date.toLocaleString();
  };

  return (
    <div className="subagents-settings">
      <div className="settings-header">
        <h2>Subagents</h2>
        <button className="btn-primary" onClick={handleCreate}>+ New Subagent</button>
      </div>
      
      <div className="settings-content">
        <div className="settings-list">
          {subagents.map(subagent => (
            <div
              key={subagent.id}
              className={`settings-list-item ${selectedSubagentId === subagent.id ? 'selected' : ''}`}
              onClick={() => {
                setSelectedSubagentId(subagent.id);
                setIsEditing(false);
                setEditedSubagent(null);
              }}
            >
              <div className="item-name">{subagent.name}</div>
              <div className="item-description">{subagent.description || 'No description'}</div>
            </div>
          ))}
        </div>

        <div className="settings-detail">
          {selectedSubagent ? (
            isEditing && editedSubagent ? (
              <div className="detail-form">
                <div className="form-group">
                  <label>Name</label>
                  <input
                    type="text"
                    value={editedSubagent.name}
                    onChange={e => setEditedSubagent({ ...editedSubagent, name: e.target.value })}
                  />
                </div>
                <div className="form-group">
                  <label>Description</label>
                  <textarea
                    value={editedSubagent.description || ''}
                    onChange={e => setEditedSubagent({ ...editedSubagent, description: e.target.value })}
                    rows={3}
                  />
                </div>
                <div className="form-group">
                  <label>System Prompt</label>
                  <textarea
                    value={editedSubagent.system_prompt || ''}
                    onChange={e => setEditedSubagent({ ...editedSubagent, system_prompt: e.target.value })}
                    rows={5}
                  />
                </div>
                <div className="form-row">
                  <div className="form-group">
                    <label>Model</label>
                    <input
                      type="text"
                      value={editedSubagent.model || ''}
                      onChange={e => setEditedSubagent({ ...editedSubagent, model: e.target.value })}
                    />
                  </div>
                  <div className="form-group">
                    <label>Temperature</label>
                    <input
                      type="number"
                      step="0.1"
                      min="0"
                      max="2"
                      value={editedSubagent.temperature ?? ''}
                      onChange={e => setEditedSubagent({ ...editedSubagent, temperature: e.target.value ? parseFloat(e.target.value) : undefined })}
                    />
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
                  <h3>{selectedSubagent.name}</h3>
                  <div className="detail-actions">
                    <button className="btn-secondary" onClick={handleEdit}>Edit</button>
                    <button className="btn-danger" onClick={handleDelete}>Delete</button>
                  </div>
                </div>
                <div className="detail-section">
                  <label>Description</label>
                  <p>{selectedSubagent.description || 'No description'}</p>
                </div>
                <div className="detail-section">
                  <label>System Prompt</label>
                  <pre>{selectedSubagent.system_prompt || 'No system prompt'}</pre>
                </div>
                <div className="detail-row">
                  <div className="detail-section">
                    <label>Model</label>
                    <p>{selectedSubagent.model || 'Default'}</p>
                  </div>
                  <div className="detail-section">
                    <label>Temperature</label>
                    <p>{selectedSubagent.temperature ?? 'Default'}</p>
                  </div>
                </div>
                
                {/* Usage Information Section */}
                <div className="detail-section usage-section">
                  <label>Usage Information</label>
                  <div className="usage-stats">
                    <div className="usage-stat">
                      <span className="stat-label">Usage Count:</span>
                      <span className="stat-value">{selectedSubagent.usage_count || 0}</span>
                    </div>
                    <div className="usage-stat">
                      <span className="stat-label">Last Used:</span>
                      <span className="stat-value">{formatDate(selectedSubagent.last_used)}</span>
                    </div>
                  </div>
                  
                  <div className="usage-by-agents">
                    <span className="stat-label">Used by Agents:</span>
                    {(() => {
                      const usedByAgents = getUsedByAgents(selectedSubagent.id);
                      if (usedByAgents.length === 0) {
                        return <span className="stat-value">Not used by any agents</span>;
                      }
                      return (
                        <ul className="used-by-list">
                          {usedByAgents.map(agent => (
                            <li key={agent.id}>{agent.name}</li>
                          ))}
                        </ul>
                      );
                    })()}
                  </div>
                </div>
              </div>
            )
          ) : (
            <div className="empty-state">
              <p>No subagent selected</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
