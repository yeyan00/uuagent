package router

import (
	"fmt"
	"strings"

	"github.com/yeyan00/uuagent/internal/config"
)

// Tier is a model capability tier.
type Tier string

const (
	TierFast     Tier = "fast"      // low-cost and low-latency
	TierStrong   Tier = "strong"    // stronger reasoning
	TierLargeCtx Tier = "large_ctx" // large context
)

// Decision explains why a model was selected.
type Decision struct {
	Model    string `json:"selected_model"`
	Tier     Tier   `json:"selected_tier"`
	Source   string `json:"source"`
	RuleName string `json:"rule_name,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Router chooses a model based on rules and context size.
type Router struct {
	tiers    map[string][]string
	rules    []config.RouteRule
	fallback string
}

// New creates a Router.
func New(cfg config.RoutingConfig) *Router {
	return &Router{
		tiers:    cfg.Tiers,
		rules:    cfg.Rules,
		fallback: cfg.Fallback,
	}
}

// Decide chooses a model and returns route metadata for the prompt and token count.
func (r *Router) Decide(prompt string, tokenCount int) Decision {
	lower := strings.ToLower(prompt)
	for _, rule := range r.rules {
		if matched, reason := ruleMatches(rule, lower, tokenCount); matched {
			tier := Tier(rule.Tier)
			if model := r.pickModel(tier); model != "" {
				return Decision{Model: model, Tier: tier, Source: "rule", RuleName: rule.Name, Reason: reason}
			}
		}
	}
	if model := r.pickModel(Tier(r.fallback)); model != "" {
		return Decision{Model: model, Tier: Tier(r.fallback), Source: "fallback", Reason: "fallback tier"}
	}
	if model := r.pickModel(TierStrong); model != "" {
		return Decision{Model: model, Tier: TierStrong, Source: "fallback", Reason: "strong tier fallback"}
	}
	return Decision{Model: "gpt-4o-mini", Tier: TierFast, Source: "fallback", Reason: "hardcoded fallback"}
}

// Route chooses a model for the prompt and token count.
// It returns the model name and selected tier.
func (r *Router) Route(prompt string, tokenCount int) (string, Tier) {
	decision := r.Decide(prompt, tokenCount)
	return decision.Model, decision.Tier
}

func ruleMatches(rule config.RouteRule, lowerPrompt string, tokenCount int) (bool, string) {
	for _, pattern := range rule.Patterns {
		normalized := strings.ToLower(pattern)
		if strings.Contains(lowerPrompt, normalized) {
			return true, "matched pattern: " + normalized
		}
	}
	if strings.Contains(rule.Condition, "tokens >") {
		var threshold int
		if _, err := fmt.Sscanf(rule.Condition, "tokens > %d", &threshold); err == nil && tokenCount > threshold {
			return true, fmt.Sprintf("matched condition: tokens > %d", threshold)
		}
	}
	return false, ""
}

// pickModel selects the first configured model for a tier.
func (r *Router) pickModel(tier Tier) string {
	models, ok := r.tiers[string(tier)]
	if !ok || len(models) == 0 {
		return ""
	}
	return models[0]
}
