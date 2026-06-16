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

// Route chooses a model for the prompt and token count.
// It returns the model name and selected tier.
func (r *Router) Route(prompt string, tokenCount int) (string, Tier) {
	// 1. Match explicit routing rules.
	for _, rule := range r.rules {
		if r.matchRule(prompt, tokenCount, rule) {
			tier := Tier(rule.Tier)
			if model := r.pickModel(tier); model != "" {
				return model, tier
			}
		}
	}

	// 2. Use fallback tier when no rule matches.
	if model := r.pickModel(Tier(r.fallback)); model != "" {
		return model, Tier(r.fallback)
	}

	// 3. Use the first strong model if fallback is unavailable.
	if model := r.pickModel(TierStrong); model != "" {
		return model, TierStrong
	}

	return "gpt-4o-mini", TierFast // final fallback
}

// matchRule checks whether a rule matches.
func (r *Router) matchRule(prompt string, tokenCount int, rule config.RouteRule) bool {
	lower := strings.ToLower(prompt)

	// Keyword match.
	for _, pattern := range rule.Patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}

	// Simple condition parsing for "tokens > N".
	if rule.Condition != "" {
		if strings.Contains(rule.Condition, "tokens >") {
			var threshold int
			fmt.Sscanf(rule.Condition, "tokens > %d", &threshold)
			if tokenCount > threshold {
				return true
			}
		}
	}

	return false
}

// pickModel selects the first configured model for a tier.
func (r *Router) pickModel(tier Tier) string {
	models, ok := r.tiers[string(tier)]
	if !ok || len(models) == 0 {
		return ""
	}
	return models[0]
}
