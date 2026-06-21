package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/internal/agent"
)

type modelsSettingsPayload struct {
	ProxyURL     string              `json:"proxy_url"`
	RoutingTiers map[string][]string `json:"routing_tiers"`
	FallbackTier string              `json:"fallback_tier"`
	ModelIDs     []string            `json:"model_ids,omitempty"`
}

type modelsTestRequest struct {
	ProxyURL string `json:"proxy_url"`
}

type modelsTestResponse struct {
	Success  bool     `json:"success"`
	ModelIDs []string `json:"model_ids"`
	Error    string   `json:"error,omitempty"`
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func handleGetModelsSettings(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := agt.Config().Agent
		c.JSON(http.StatusOK, modelsSettingsPayload{
			ProxyURL:     cfg.ProxyURL,
			RoutingTiers: cloneTiers(cfg.Routing.Tiers),
			FallbackTier: cfg.Routing.Fallback,
			ModelIDs:     flattenModelIDs(cfg.Routing.Tiers),
		})
	}
}

func handlePutModelsSettings(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req modelsSettingsPayload
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := agt.UpdateModelSettingsPersistent(req.ProxyURL, req.RoutingTiers, req.FallbackTier); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		cfg := agt.Config().Agent
		c.JSON(http.StatusOK, modelsSettingsPayload{
			ProxyURL:     cfg.ProxyURL,
			RoutingTiers: cloneTiers(cfg.Routing.Tiers),
			FallbackTier: cfg.Routing.Fallback,
			ModelIDs:     flattenModelIDs(cfg.Routing.Tiers),
		})
	}
}

func handleTestModels() gin.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(c *gin.Context) {
		var req modelsTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, modelsTestResponse{Success: false, Error: err.Error()})
			return
		}
		modelIDs, status, err := fetchModels(c.Request.Context(), client, req.ProxyURL)
		if err != nil {
			c.JSON(status, modelsTestResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, modelsTestResponse{Success: true, ModelIDs: modelIDs})
	}
}

func validateProxyURL(proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL: scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("invalid URL: host is required")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil {
		// Allow loopback addresses (localhost, 127.0.0.1, [::1]) for testing
		// Reject private non-loopback IPs (192.168.x.x, 10.x.x.x, etc.)
		if ip.IsPrivate() && !ip.IsLoopback() {
			return fmt.Errorf("private IP addresses are not allowed")
		}
		if ip.IsUnspecified() {
			return fmt.Errorf("unspecified IP addresses are not allowed")
		}
	}
	return nil
}

func redactSensitiveData(body string) string {
	// Redact API keys and tokens
	patterns := []struct {
		pattern *regexp.Regexp
		repl    string
	}{
		{regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|apikey|key|token|secret|password)["']?\s*[:=]\s*["'])[^"']{8,}(["']?)`), "${1}[REDACTED]${2}"},
		{regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`), "[REDACTED]"},
		{regexp.MustCompile(`(?i)(Bearer\s+)[a-zA-Z0-9_\-\.]{20,}`), "${1}[REDACTED]"},
	}
	result := body
	for _, p := range patterns {
		result = p.pattern.ReplaceAllString(result, p.repl)
	}
	return result
}

func fetchModels(ctx context.Context, client *http.Client, proxyURL string) ([]string, int, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(proxyURL), "/")
	if baseURL == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("proxy_url is required")
	}
	if err := validateProxyURL(baseURL); err != nil {
		return nil, http.StatusBadRequest, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("create models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("models request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if readErr != nil {
			return nil, http.StatusBadGateway, fmt.Errorf("models request failed: status=%d read body: %w", resp.StatusCode, readErr)
		}
		_ = redactSensitiveData(string(data))
		return nil, resp.StatusCode, fmt.Errorf("models request failed: status=%d [upstream error redacted]", resp.StatusCode)
	}
	modelIDs, err := parseModelsResponse(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	return modelIDs, http.StatusOK, nil
}

func cloneTiers(tiers map[string][]string) map[string][]string {
	out := make(map[string][]string, len(tiers))
	for tier, models := range tiers {
		out[tier] = append([]string(nil), models...)
	}
	return out
}

func flattenModelIDs(tiers map[string][]string) []string {
	seen := map[string]bool{}
	for _, models := range tiers {
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model != "" {
				seen[model] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func parseModelsResponse(body io.Reader) ([]string, error) {
	var payload openAIModelsResponse
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	modelIDs := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			modelIDs = append(modelIDs, model.ID)
		}
	}
	return modelIDs, nil
}
