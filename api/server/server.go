package server

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/memory"
	"github.com/yeyan00/uuagent/internal/paths"
	"github.com/yeyan00/uuagent/internal/subagent"
	"github.com/yeyan00/uuagent/internal/types"
)

// RegisterRoutes registers UUAgent API routes.
func RegisterRoutes(r *gin.RouterGroup, agt *agent.Agent) {
	r.GET("/projects", handleListProjects(agt))
	r.POST("/projects", handleCreateProject(agt))
	r.GET("/projects/:id", handleGetProject(agt))
	r.POST("/projects/:id/open", handleOpenProject(agt))
	r.GET("/projects/:id/sessions", handleListProjectSessions(agt))
	r.POST("/projects/:id/sessions", handleCreateProjectSession(agt))
	r.GET("/projects/:id/sessions/:session_id", handleGetProjectSession(agt))
	r.GET("/projects/:id/sessions/:session_id/context", handleGetProjectSessionContext(agt))
	r.POST("/projects/:id/sessions/:session_id/compact", handleCompactProjectSession(agt))
	r.POST("/projects/:id/sessions/:session_id/archives/:archive_id/restore", handleRestoreProjectSessionArchive(agt))
	r.PATCH("/projects/:id/sessions/:session_id", handlePatchProjectSession(agt))
	r.DELETE("/projects/:id/sessions/:session_id", handleDeleteProjectSession(agt))
	r.POST("/projects/:id/sessions/:session_id/fork", handleForkProjectSession(agt))
	r.GET("/agents", handleListAgents(agt))
	r.POST("/agents", handleUpsertAgent(agt))
	r.GET("/agents/:id", handleGetAgent(agt))
	r.PATCH("/agents/:id", handlePatchAgent(agt))
	r.DELETE("/agents/:id", handleDeleteAgent(agt))
	r.POST("/agents/:id/clone", handleCloneAgent(agt))
	r.GET("/subagents", handleListSubagents(agt))
	r.POST("/subagents", handleUpsertSubagent(agt))
	r.PATCH("/subagents/:id", handlePatchSubagent(agt))
	r.DELETE("/subagents/:id", handleDeleteSubagent(agt))
	r.GET("/subagent/tasks", handleListSubagentTasks(agt))
	r.GET("/skills", handleListSkills(agt))
	r.POST("/skills", handleCreateSkill(agt))
	r.POST("/skills/upload", handleUploadSkill(agt))
	r.GET("/skills/:name/content", handleGetSkillContent(agt))
	r.DELETE("/skills/:name", handleDeleteSkill(agt))
	r.GET("/mcp/servers", handleListMCPServers(agt))
	r.GET("/tools", handleListTools(agt))
	r.GET("/models/settings", handleGetModelsSettings(agt))
	r.PUT("/models/settings", handlePutModelsSettings(agt))
	r.POST("/models/test", handleTestModels())
	r.GET("/chat", handleChatSSE(agt))
	r.POST("/chat", handleChatSSE(agt))
	r.GET("/route", handleRouteInfo(agt))
	r.GET("/sessions", handleListSessions(agt))
	r.GET("/sessions/:id", handleGetSession(agt))
	r.PATCH("/sessions/:id", handlePatchSession(agt))
	r.DELETE("/sessions/:id", handleDeleteSession(agt))
	r.GET("/sessions/:id/summaries", handleSessionSummaries(agt))
	r.POST("/sessions/:id/fork", handleForkSession(agt))
	r.POST("/sessions/:id/memory/refresh", handleRefreshSessionMemory(agt))
	r.GET("/memory", handleListMemory(agt))
	r.POST("/memory", handleCreateMemory(agt))
	r.PATCH("/memory/:id", handleEditMemory(agt))
	r.POST("/memory/:id/confirm", handleConfirmMemory(agt))
	r.DELETE("/memory/:id", handleDeleteMemory(agt))
	r.POST("/runs/:id/stop", handleStopRun(agt))
	r.POST("/runs/:id/approve", handleApproveRun(agt))
	r.POST("/runs/:id/approve/stream", handleApproveRunStream(agt))
	r.POST("/runs/:id/deny", handleDenyRun(agt))
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
		agt.ReloadProjectSkills(p.WorkspacePath)
		c.JSON(http.StatusOK, gin.H{"project": p, "config_sources": sources, "config": cfg.Safe()})
	}
}

type projectSessionRequest struct {
	ID string `json:"id"`
}

func handleListProjectSessions(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"sessions": store.List(), "path": store.Root()})
	}
}

func handleCreateProjectSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		var req projectSessionRequest
		_ = c.ShouldBindJSON(&req)
		if req.ID == "" {
			req.ID = fmt.Sprintf("s-%d", time.Now().UnixMilli())
		}
		sess := store.GetOrCreate(req.ID)
		if err := sess.BindProject(p.ID, p.WorkspacePath); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sess.Snapshot())
	}
}

func handleGetProjectSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		sess, ok := store.Get(c.Param("session_id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusOK, sess.Snapshot())
	}
}

func handleGetProjectSessionContext(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		sess, ok := store.Get(c.Param("session_id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		snap := sess.Snapshot()
		c.JSON(http.StatusOK, gin.H{
			"context":   sess.ContextStats(agt.Config().Agent.Context.MaxTokens),
			"usage":     snap.Usage,
			"summaries": snap.Summaries,
			"archives":  snap.Archives,
		})
	}
}

type compactSessionRequest struct {
	KeepLastMessages int     `json:"keep_last_messages"`
	Threshold        float64 `json:"threshold"`
}

func handleCompactProjectSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		sess, ok := store.Get(c.Param("session_id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		var req compactSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg := agt.Config().Agent.Context
		keepLast := cfg.KeepLastMessages
		if req.KeepLastMessages > 0 {
			keepLast = req.KeepLastMessages
		}
		threshold := cfg.CompressThreshold
		if req.Threshold > 0 {
			threshold = req.Threshold
		}
		if _, ok := sess.CompactArchive(cfg.MaxTokens, threshold, keepLast); !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session context is below compact threshold"})
			return
		}
		snap := sess.Snapshot()
		c.JSON(http.StatusOK, gin.H{
			"context":   sess.ContextStats(cfg.MaxTokens),
			"usage":     snap.Usage,
			"summaries": snap.Summaries,
			"archives":  snap.Archives,
		})
	}
}

func handleRestoreProjectSessionArchive(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		sess, ok := store.Get(c.Param("session_id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		archiveID := c.Param("archive_id")
		restored, err := sess.RestoreCompactArchive(archiveID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			if strings.Contains(err.Error(), "conflict") {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		snap := sess.Snapshot()
		c.JSON(http.StatusOK, gin.H{
			"session":   snap,
			"context":   sess.ContextStats(agt.Config().Agent.Context.MaxTokens),
			"usage":     snap.Usage,
			"summaries": snap.Summaries,
			"archives":  snap.Archives,
			"restored":  len(restored),
		})
	}
}

func handlePatchProjectSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		sess, ok := store.Get(c.Param("session_id"))
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

func handleDeleteProjectSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, _, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		if !store.Delete(c.Param("session_id")) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleForkProjectSession(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		store, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		parentID := c.Param("session_id")
		newID := c.Query("new_id")
		if newID == "" {
			newID = fmt.Sprintf("%s-fork-%d", parentID, time.Now().Unix())
		}
		child, err := store.Fork(parentID, newID, -1)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		_ = child.BindProject(p.ID, p.WorkspacePath)
		c.JSON(http.StatusOK, child.Snapshot())
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

func handleListSubagents(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"subagents": agt.SubagentProfiles()})
	}
}

func handleUpsertSubagent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var profile config.SubagentProfile
		if err := c.ShouldBindJSON(&profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		profile, err := agt.UpsertSubagentProfile(profile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, profile)
	}
}

func handlePatchSubagent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var profile config.SubagentProfile
		if err := c.ShouldBindJSON(&profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		profile.ID = c.Param("id")
		profile, err := agt.UpsertSubagentProfile(profile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, profile)
	}
}

func handleDeleteSubagent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := agt.DeleteSubagentProfile(c.Param("id")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleListSubagentTasks(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		m := subagent.NewManager(agt.Config().Agent.Subagent, nil)
		c.JSON(http.StatusOK, gin.H{"tasks": m.Tasks()})
	}
}

type chatRequest struct {
	Prompt    string   `json:"prompt"`
	SessionID string   `json:"session_id"`
	AgentID   string   `json:"agent_id"`
	ProjectID string   `json:"project_id"`
	ImageURL  []string `json:"image_url"`
}

const (
	maxImagesPerRequest = 4
	maxImageSizeBytes   = 4 * 1024 * 1024 // 4MB
)

// handleChatSSE streams chat events over SSE.
func handleChatSSE(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req chatRequest
		if c.Request.Method == http.MethodPost {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else {
			req.Prompt = c.Query("prompt")
			req.SessionID = c.Query("session_id")
			req.AgentID = c.Query("agent_id")
			req.ProjectID = c.Query("project_id")
			req.ImageURL = c.QueryArray("image_url")
		}

		if req.Prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
			return
		}
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		// Image attachment guard: max 4 images, ~4MB each
		if len(req.ImageURL) > maxImagesPerRequest {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("maximum %d images allowed per request", maxImagesPerRequest)})
			return
		}
		for _, imageURL := range req.ImageURL {
			if imageURL != "" {
				// Check image size for data URLs
				if strings.HasPrefix(imageURL, "data:") {
					// Extract base64 data
					parts := strings.SplitN(imageURL, ",", 2)
					if len(parts) == 2 {
						decoded, err := base64.StdEncoding.DecodeString(parts[1])
						if err == nil && len(decoded) > maxImageSizeBytes {
							c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("image exceeds maximum size of %dMB", maxImageSizeBytes/(1024*1024))})
							return
						}
					}
				}
			}
		}

		parts := []types.ContentPart{{Type: "text", Text: req.Prompt}}
		for _, imageURL := range req.ImageURL {
			if imageURL != "" {
				parts = append(parts, types.ContentPart{Type: "image_url", ImageURL: &types.ImageURL{URL: imageURL}})
			}
		}
		events, err := agt.RunWithAgentProjectParts(c.Request.Context(), req.SessionID, req.AgentID, req.ProjectID, parts)
		if err != nil {
			if errors.Is(err, agent.ErrSessionProjectConflict) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		for evt := range events {
			writeSSE(c, evt)
		}
	}
}

func handleListSkills(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"skills": agt.Skills().List(), "diagnostics": agt.Skills().Diagnostics()})
	}
}

func handleGetSkillContent(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		content, ok := agt.Skills().Content(c.Param("name"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"name": c.Param("name"), "content": content})
	}
}

type createSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
}

func handleCreateSkill(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createSkillRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.URL) != "" {
			resp, err := http.Get(req.URL)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			defer resp.Body.Close()
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
			name, description := skillMetaFromMarkdown(string(data), req.Name, req.Description)
			if err := writeUserSkill(name, description, string(data)); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			agt.ReloadConfig(agt.Config())
			c.JSON(http.StatusOK, gin.H{"name": name, "description": description})
			return
		}
		if err := writeUserSkill(req.Name, req.Description, req.Content); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		agt.ReloadConfig(agt.Config())
		c.JSON(http.StatusOK, gin.H{"name": req.Name, "description": req.Description})
	}
}

func handleUploadSkill(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer src.Close()
		tmp, err := os.CreateTemp("", "uuagent-skill-*.zip")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer os.Remove(tmp.Name())
		_, _ = io.Copy(tmp, src)
		_ = tmp.Close()
		zr, err := zip.OpenReader(tmp.Name())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer zr.Close()
		var created []string
		for _, f := range zr.File {
			if filepath.Base(f.Name) != "SKILL.md" && !strings.HasSuffix(strings.ToLower(f.Name), ".md") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(io.LimitReader(rc, 1024*1024))
			_ = rc.Close()
			fallback := strings.TrimSuffix(filepath.Base(filepath.Dir(f.Name)), filepath.Ext(filepath.Base(filepath.Dir(f.Name))))
			if fallback == "." || fallback == "" {
				fallback = strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(filepath.Base(f.Name)))
			}
			name, description := skillMetaFromMarkdown(string(data), fallback, "")
			if err := writeUserSkill(name, description, string(data)); err == nil {
				created = append(created, name)
			}
		}
		if len(created) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no skill markdown found"})
			return
		}
		agt.ReloadConfig(agt.Config())
		c.JSON(http.StatusOK, gin.H{"skills": created})
	}
}

func handleDeleteSkill(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := safeSkillName(c.Param("name"))
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid skill name"})
			return
		}
		workspace := ""
		removedFromConfig := false
		removePath := filepath.Join(paths.UserDir(), "skills", name)
		if skill, ok := agt.Skills().Get(name); ok && strings.TrimSpace(skill.Path) != "" {
			removePath = deletableSkillPath(skill.Path)
			workspace = skillWorkspaceRoot(skill.Path)
		} else if removeSkillFromUserConfig(name) {
			removedFromConfig = true
			removePath = ""
		}
		if removePath != "" {
			if err := os.RemoveAll(removePath); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else if !removedFromConfig {
			c.JSON(http.StatusBadRequest, gin.H{"error": "skill cannot be deleted from its configured source"})
			return
		}
		if removedFromConfig {
			if cfg, err := config.Load(config.UserConfigPath()); err == nil {
				agt.ReloadConfig(cfg)
			}
		} else {
			agt.ReloadConfig(agt.Config())
		}
		if workspace != "" {
			agt.ReloadProjectSkills(workspace)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func removeSkillFromUserConfig(name string) bool {
	cfg, err := config.Load(config.UserConfigPath())
	if err != nil {
		return false
	}
	kept := cfg.Skills[:0]
	removed := false
	for _, skill := range cfg.Skills {
		if skill.Name == name {
			removed = true
			continue
		}
		kept = append(kept, skill)
	}
	if !removed {
		return false
	}
	cfg.Skills = kept
	for i := range cfg.Agents {
		cfg.Agents[i].EnabledSkills = removeString(cfg.Agents[i].EnabledSkills, name)
	}
	for i := range cfg.Agent.Subagent.Profiles {
		cfg.Agent.Subagent.Profiles[i].EnabledSkills = removeString(cfg.Agent.Subagent.Profiles[i].EnabledSkills, name)
	}
	return config.SaveUser(cfg) == nil
}

func removeString(items []string, value string) []string {
	kept := items[:0]
	for _, item := range items {
		if item != value {
			kept = append(kept, item)
		}
	}
	return kept
}

func deletableSkillPath(skillPath string) string {
	clean := filepath.Clean(skillPath)
	base := filepath.Base(clean)
	dir := filepath.Dir(clean)
	if base == "SKILL.md" {
		return dir
	}
	if strings.HasSuffix(strings.ToLower(base), ".md") {
		return clean
	}
	return ""
}

func skillWorkspaceRoot(skillPath string) string {
	clean := filepath.Clean(skillPath)
	markers := []string{filepath.Join(".uuagent", "skills"), filepath.Join(".agents", "skills")}
	for _, marker := range markers {
		idx := strings.Index(clean, marker)
		if idx > 0 {
			return strings.TrimRight(clean[:idx], string(filepath.Separator))
		}
	}
	return ""
}

func writeUserSkill(name, description, content string) error {
	name = safeSkillName(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(description) == "" {
		description = name
	}
	body := strings.TrimSpace(content)
	if body == "" {
		body = description
	}
	if !strings.HasPrefix(body, "---") {
		body = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s", name, description, body)
	}
	dir := filepath.Join(paths.UserDir(), "skills", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644)
}

func skillMetaFromMarkdown(raw, fallbackName, fallbackDescription string) (string, string) {
	name := fallbackName
	description := fallbackDescription
	if strings.HasPrefix(raw, "---") {
		lines := strings.Split(raw, "\n")
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if line == "---" {
				break
			}
			if v, ok := strings.CutPrefix(line, "name:"); ok {
				name = strings.Trim(strings.TrimSpace(v), `"'`)
			}
			if v, ok := strings.CutPrefix(line, "description:"); ok {
				description = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	return safeSkillName(name), description
}

func safeSkillName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, `/\\`)
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\\`) {
		return ""
	}
	return name
}

func handleListMCPServers(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		servers := []gin.H{}
		for _, srv := range agt.Config().MCPServers {
			status := "disabled"
			if srv.Enabled {
				status = "connected"
			}
			servers = append(servers, gin.H{"id": srv.ID, "name": srv.Name, "transport": srv.Transport, "enabled": srv.Enabled, "status": status})
		}
		c.JSON(http.StatusOK, gin.H{"servers": servers})
	}
}

func handleListTools(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		tools := []string{}
		tools = append(tools, agt.ToolNames()...)
		for _, tool := range agt.MCPTools(c.Request.Context()) {
			tools = append(tools, tool.Name)
		}
		c.JSON(http.StatusOK, gin.H{"tools": tools})
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

func handleRefreshSessionMemory(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := agt.RefreshSessionMemorySnapshot(c.Param("id"), resolveProjectMemoryKey(agt, c.Query("project_id")), c.Query("agent_id"))
		c.JSON(http.StatusOK, gin.H{"memory_snapshot": snapshot})
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
		project := resolveProjectMemoryKey(agt, c.Query("project"))
		scope := c.Query("scope")
		c.JSON(http.StatusOK, gin.H{"memories": agt.Memories().ListFiltered(status, project, scope), "path": agt.Memories().Path()})
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
		req.Project = resolveProjectMemoryKey(agt, req.Project)
		entry := agt.Memories().Add(req.Content, req.Project, req.Scope, req.Source, req.Status)
		c.JSON(http.StatusOK, entry)
	}
}

func resolveProjectMemoryKey(agt *agent.Agent, project string) string {
	if p, ok := agt.Projects().Get(project); ok && p.WorkspacePath != "" {
		return p.WorkspacePath
	}
	return project
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

func handleStopRun(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !agt.StopRun(c.Param("id")) {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "stopping"})
	}
}

func handleApproveRun(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		content, err := agt.ApproveRun(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "approved", "content": content})
	}
}

func handleDenyRun(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := agt.DenyRun(c.Param("id")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "denied"})
	}
}

func handleApproveRunStream(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, err := agt.ApproveRunEvents(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		flusher, _ := c.Writer.(http.Flusher)
		for evt := range events {
			data, _ := json.Marshal(evt)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
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
