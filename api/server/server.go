package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/memory"
	"github.com/yeyan00/uuagent/internal/types"
)

// RegisterRoutes registers UUAgent API routes.
func RegisterRoutes(r *gin.RouterGroup, agt *agent.Agent) {
	r.GET("/projects", handleListProjects(agt))
	r.POST("/projects", handleCreateProject(agt))
	r.GET("/projects/:id", handleGetProject(agt))
	r.POST("/projects/:id/open", handleOpenProject(agt))
	r.GET("/agents", handleListAgents(agt))
	r.POST("/agents", handleUpsertAgent(agt))
	r.GET("/agents/:id", handleGetAgent(agt))
	r.PATCH("/agents/:id", handlePatchAgent(agt))
	r.DELETE("/agents/:id", handleDeleteAgent(agt))
	r.POST("/agents/:id/clone", handleCloneAgent(agt))
	r.GET("/chat", handleChatSSE(agt))
	r.GET("/route", handleRouteInfo(agt))
	r.GET("/sessions", handleListSessions(agt))
	r.GET("/sessions/:id", handleGetSession(agt))
	r.PATCH("/sessions/:id", handlePatchSession(agt))
	r.DELETE("/sessions/:id", handleDeleteSession(agt))
	r.GET("/sessions/:id/summaries", handleSessionSummaries(agt))
	r.POST("/sessions/:id/fork", handleForkSession(agt))
	r.GET("/memory", handleListMemory(agt))
	r.POST("/memory", handleCreateMemory(agt))
	r.PATCH("/memory/:id", handleEditMemory(agt))
	r.POST("/memory/:id/confirm", handleConfirmMemory(agt))
	r.DELETE("/memory/:id", handleDeleteMemory(agt))
	r.GET("/config", handleConfig(agt))
	r.GET("/health", handleHealth)
}

type projectRequest struct {
	Name          string `json:"name"`
	WorkspacePath string `json:"workspace_path"`
}

func handleListProjects(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"projects": agt.Projects().List()})
	}
}

func handleCreateProject(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req projectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		p, err := agt.Projects().Create(req.Name, req.WorkspacePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, p)
	}
}

func handleGetProject(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := agt.Projects().Get(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		c.JSON(http.StatusOK, p)
	}
}

func handleOpenProject(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := agt.Projects().Get(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		cfg, sources, err := config.LoadAuto(config.CandidatePaths(p.WorkspacePath)...)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "sources": sources})
			return
		}
		agt.ReloadConfig(cfg)
		c.JSON(http.StatusOK, gin.H{"project": p, "config_sources": sources, "config": cfg.Safe()})
	}
}

func handleListAgents(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"agents": agt.Profiles()})
	}
}

func handleGetAgent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		profile, ok := agt.GetProfile(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		c.JSON(http.StatusOK, profile)
	}
}

func handleUpsertAgent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var profile config.AgentProfile
		if err := c.ShouldBindJSON(&profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		profile, err := agt.UpsertProfilePersistent(profile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, profile)
	}
}

func handlePatchAgent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var profile config.AgentProfile
		if err := c.ShouldBindJSON(&profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		profile.ID = c.Param("id")
		profile, err := agt.UpsertProfilePersistent(profile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, profile)
	}
}

func handleDeleteAgent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := agt.DeleteProfilePersistent(c.Param("id")); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

type cloneAgentRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func handleCloneAgent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req cloneAgentRequest
		_ = c.ShouldBindJSON(&req)
		profile, err := agt.CloneProfilePersistent(c.Param("id"), req.ID, req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, profile)
	}
}

// handleChatSSE streams chat events over SSE.
func handleChatSSE(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		prompt := c.Query("prompt")
		if prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
			return
		}
		sessionID := c.Query("session_id")
		if sessionID == "" {
			sessionID = "default"
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		agentID := c.Query("agent_id")
		parts := []types.ContentPart{{Type: "text", Text: prompt}}
		for _, imageURL := range c.QueryArray("image_url") {
			if imageURL != "" {
				parts = append(parts, types.ContentPart{Type: "image_url", ImageURL: &types.ImageURL{URL: imageURL}})
			}
		}
		events, err := agt.RunWithAgentParts(c.Request.Context(), sessionID, agentID, parts)
		if err != nil {
			writeSSE(c, agent.Event{Type: "error", Text: err.Error()})
			return
		}

		for evt := range events {
			writeSSE(c, evt)
		}
	}
}

func writeSSE(c *gin.Context, evt agent.Event) {
	encoder := json.NewEncoder(c.Writer)
	fmt.Fprint(c.Writer, "data: ")
	_ = encoder.Encode(evt)
	fmt.Fprint(c.Writer, "\n")
	c.Writer.Flush()
}

// handleRouteInfo returns the routing decision without executing a chat turn.
func handleRouteInfo(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		prompt := c.Query("prompt")
		model, tier := agt.Route(prompt, 0)
		c.JSON(http.StatusOK, gin.H{
			"model":  model,
			"tier":   tier,
			"prompt": prompt,
		})
	}
}

func handleListSessions(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"sessions": agt.Sessions().List(), "path": agt.Sessions().Root()})
	}
}

func handleGetSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := agt.Sessions().Get(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusOK, sess.Snapshot())
	}
}

type sessionPatchRequest struct {
	Title string `json:"title"`
}

func handlePatchSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := agt.Sessions().Get(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		var req sessionPatchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		sess.UpdateTitle(req.Title)
		c.JSON(http.StatusOK, sess.Snapshot())
	}
}

func handleDeleteSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !agt.Sessions().Delete(c.Param("id")) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleSessionSummaries(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := agt.Sessions().GetOrCreate(c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"summaries": sess.ListSummaries()})
	}
}

func handleForkSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		parentID := c.Param("id")
		newID := c.Query("new_id")
		if newID == "" {
			newID = fmt.Sprintf("%s-fork-%d", parentID, time.Now().Unix())
		}
		child, err := agt.Sessions().Fork(parentID, newID, -1)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, child)
	}
}

type memoryRequest struct {
	Content string        `json:"content"`
	Project string        `json:"project"`
	Scope   string        `json:"scope"`
	Source  string        `json:"source"`
	Status  memory.Status `json:"status"`
}

func handleListMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := memory.Status(c.Query("status"))
		project := c.Query("project")
		c.JSON(http.StatusOK, gin.H{"memories": agt.Memories().List(status, project), "path": agt.Memories().Path()})
	}
}

func handleCreateMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req memoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
			return
		}
		if req.Status == "" {
			req.Status = memory.StatusConfirmed
		}
		entry := agt.Memories().Add(req.Content, req.Project, req.Scope, req.Source, req.Status)
		c.JSON(http.StatusOK, entry)
	}
}

func handleEditMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req memoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !agt.Memories().Edit(c.Param("id"), req.Content) {
			c.JSON(http.StatusNotFound, gin.H{"error": "memory not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleConfirmMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !agt.Memories().Confirm(c.Param("id")) {
			c.JSON(http.StatusNotFound, gin.H{"error": "draft memory not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleDeleteMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !agt.Memories().Delete(c.Param("id")) {
			c.JSON(http.StatusNotFound, gin.H{"error": "memory not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// handleConfig returns the redacted active configuration.
func handleConfig(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "config": agt.Config().Safe()})
	}
}

// handleHealth returns the service health status.
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.1.0"})
}
