package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/uuagent/uuagent/internal/agent"
)

// RegisterRoutes 注册 UUAgent API 路由
func RegisterRoutes(r *gin.RouterGroup, agt *agent.Agent) {
	r.GET("/chat", handleChatSSE(agt))
	r.GET("/route", handleRouteInfo(agt))
	r.POST("/memory", handleMemory(agt))
	r.GET("/config", handleConfig(agt))
	r.GET("/health", handleHealth)
}

// handleChatSSE SSE 流式聊天
func handleChatSSE(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		prompt := c.Query("prompt")
		sessionID := c.Query("session_id")
		if sessionID == "" {
			sessionID = "default"
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		events, err := agt.Run(c.Request.Context(), sessionID, prompt)
		if err != nil {
			fmt.Fprintf(c.Writer, "data: {\"type\":\"error\",\"text\":\"%s\"}\n\n", err.Error())
			c.Writer.Flush()
			return
		}

		for evt := range events {
			fmt.Fprintf(c.Writer, "data: {\"type\":\"%s\",\"model\":\"%s\",\"tier\":\"%s\",\"text\":\"%s\"}\n\n",
				evt.Type, evt.Model, evt.Tier, escapeJSON(evt.Text))
			c.Writer.Flush()
		}
	}
}

// handleRouteInfo 查看路由决策 (不执行)
func handleRouteInfo(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		prompt := c.Query("prompt")
		model, tier := agt.Route(prompt, 0)
		c.JSON(http.StatusOK, gin.H{
			"model": model,
			"tier":  tier,
			"prompt": prompt,
		})
	}
}

// handleMemory Memory CRUD
func handleMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 实现 Memory CRUD
		c.JSON(http.StatusOK, gin.H{"status": "not implemented"})
	}
}

// handleConfig 配置查看
func handleConfig(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 返回当前配置
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// handleHealth 健康检查
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.1.0"})
}

// escapeJSON 简单 JSON 转义
func escapeJSON(s string) string {
	s = fmt.Sprintf("%q", s)
	return s[1 : len(s)-1] // 去掉两端的引号
}
