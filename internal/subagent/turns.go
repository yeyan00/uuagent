package subagent

import "github.com/yeyan00/uuagent/internal/config"

func (m *Manager) resolveMaxTurns(profile config.SubagentProfile) int {
	if profile.MaxTurns > 0 {
		return profile.MaxTurns
	}
	if m.cfg.MaxTurns > 0 {
		return m.cfg.MaxTurns
	}
	return 45
}
