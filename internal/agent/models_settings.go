package agent

import (
	"strings"

	"github.com/yeyan00/uuagent/internal/config"
)

// UpdateModelSettingsPersistent updates global model routing settings and writes user config.
func (a *Agent) UpdateModelSettingsPersistent(proxyURL string, proxyAPIKey string, tiers map[string][]string, fallback string) error {
	a.cfg.Agent.ProxyURL = strings.TrimRight(strings.TrimSpace(proxyURL), "/")
	a.cfg.Agent.ProxyAPIKey = strings.TrimSpace(proxyAPIKey)
	a.cfg.Agent.Routing.Tiers = cloneRoutingTiers(tiers)
	a.cfg.Agent.Routing.Fallback = strings.TrimSpace(fallback)
	if err := config.SaveUser(a.cfg); err != nil {
		return err
	}
	a.ReloadConfig(a.cfg)
	return nil
}

func cloneRoutingTiers(tiers map[string][]string) map[string][]string {
	out := make(map[string][]string, len(tiers))
	for tier, models := range tiers {
		cleanTier := strings.TrimSpace(tier)
		if cleanTier == "" {
			continue
		}
		cleanModels := make([]string, 0, len(models))
		for _, model := range models {
			cleanModel := strings.TrimSpace(model)
			if cleanModel != "" {
				cleanModels = append(cleanModels, cleanModel)
			}
		}
		out[cleanTier] = cleanModels
	}
	return out
}
