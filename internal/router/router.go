package router

import (
	"fmt"
	"strings"

	"github.com/uuagent/uuagent/internal/config"
)

// Tier 模型等级
type Tier string

const (
	TierFast      Tier = "fast"       // 便宜快速
	TierStrong    Tier = "strong"     // 强力推理
	TierLargeCtx  Tier = "large_ctx"  // 大上下文
)

// Router 智能模型路由器 — UUAgent 的杀手特性
type Router struct {
	tiers    map[string][]string
	rules    []config.RouteRule
	fallback string
}

// New 创建路由器
func New(cfg config.RoutingConfig) *Router {
	return &Router{
		tiers:    cfg.Tiers,
		rules:    cfg.Rules,
		fallback: cfg.Fallback,
	}
}

// Route 根据输入内容路由到合适的模型
// 返回: (模型名, 模型等级)
func (r *Router) Route(prompt string, tokenCount int) (string, Tier) {
	// 1. 遍历规则，匹配关键词
	for _, rule := range r.rules {
		if r.matchRule(prompt, tokenCount, rule) {
			tier := Tier(rule.Tier)
			if model := r.pickModel(tier); model != "" {
				return model, tier
			}
		}
	}

	// 2. 规则匹配不到，用 fallback
	if model := r.pickModel(Tier(r.fallback)); model != "" {
		return model, Tier(r.fallback)
	}

	// 3. 都没有，用 strong 的第一个
	if model := r.pickModel(TierStrong); model != "" {
		return model, TierStrong
	}

	return "gpt-4o-mini", TierFast // 终极 fallback
}

// matchRule 检查是否匹配规则
func (r *Router) matchRule(prompt string, tokenCount int, rule config.RouteRule) bool {
	lower := strings.ToLower(prompt)

	// 关键词匹配
	for _, pattern := range rule.Patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}

	// 条件匹配 (简单解析 "tokens > N")
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

// pickModel 从 tier 中选第一个可用模型
func (r *Router) pickModel(tier Tier) string {
	models, ok := r.tiers[string(tier)]
	if !ok || len(models) == 0 {
		return ""
	}
	return models[0]
}
