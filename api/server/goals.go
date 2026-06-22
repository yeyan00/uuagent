package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/goal"
	"github.com/yeyan00/uuagent/internal/paths"
	"github.com/yeyan00/uuagent/internal/subagent"
)

type goalCreateRequest struct {
	Prompt string `json:"prompt"`
}

func handleCreateGoal(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		var req goalCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		store := goal.NewStore(filepath.Join(paths.ProjectsDir(), p.ID, "goals"))
		created, err := store.Create(c.Request.Context(), goal.CreateRequest{ProjectID: p.ID, ProjectPath: p.WorkspacePath, Prompt: req.Prompt, Plan: defaultGoalPlan(req.Prompt, agt.Config().Agent.Subagent.Profiles)})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, created)
	}
}

func handleListGoals(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		goals, err := goal.NewStore(filepath.Join(paths.ProjectsDir(), p.ID, "goals")).List(c.Request.Context(), p.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"goals": goals})
	}
}

func handleGetGoal(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		got, err := goal.NewStore(filepath.Join(paths.ProjectsDir(), p.ID, "goals")).Get(c.Request.Context(), c.Param("goal_id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, got)
	}
}

func handleRunGoal(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		store := goal.NewStore(filepath.Join(paths.ProjectsDir(), p.ID, "goals"))
		runner := goal.NewRunner(store, goal.RunnerConfig{
			Subagents: agt.Config().Agent.Subagent.Profiles,
			Delegate:  goalDelegate{agt: agt},
		})
		run, err := runner.Run(c.Request.Context(), c.Param("goal_id"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, run)
	}
}

func handleStopGoal(agt *agent.Agent) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, p, ok := agt.ProjectSessions(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		stopped, err := goal.NewStore(filepath.Join(paths.ProjectsDir(), p.ID, "goals")).Stop(c.Request.Context(), c.Param("goal_id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, stopped)
	}
}

type goalDelegate struct {
	agt *agent.Agent
}

func (d goalDelegate) Delegate(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
	manager := subagent.NewManager(d.agt.Config().Agent.Subagent, nil)
	results := manager.DelegateProfile(ctx, d.agt, "goal_mode", profileID, []string{task})
	if len(results) != 1 {
		return goal.DelegateResult{}, fmt.Errorf("delegate %s returned no result", profileID)
	}
	result := results[0]
	if result.Error != "" {
		return goal.DelegateResult{}, fmt.Errorf("delegate %s: %s", profileID, result.Error)
	}
	return goal.DelegateResult{ProfileID: profileID, Task: result.Goal, Output: result.Output}, nil
}

func defaultGoalPlan(prompt string, profiles []config.SubagentProfile) goal.Plan {
	todos := make([]goal.Todo, 0, len(profiles))
	for _, profile := range profiles {
		todos = append(todos, goal.Todo{ID: "todo-" + profile.ID, Content: prompt + " [" + profile.ID + "]", Status: goal.TodoPending})
	}
	return goal.Plan{Summary: prompt, Todos: todos}
}
