package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestAgentWithNoEnabledSkillsReceivesAllSkillMetadata(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var systemPrompt string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				systemPrompt = msg.Content
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}}})
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Skills = []config.SkillConfig{
		{Name: "review", Description: "Review code", Prompt: "full review", Enabled: true, Scope: "global"},
		{Name: "docx", Description: "Work with docx", Prompt: "full docx", Enabled: true, Scope: "global"},
	}
	cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", EnabledSkills: nil}}

	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "skill-all", "default", "hello")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(systemPrompt, `<skill name="review"`) || !strings.Contains(systemPrompt, `<skill name="docx"`) {
		t.Fatalf("expected all skill metadata, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "full review") || strings.Contains(systemPrompt, "full docx") {
		t.Fatalf("default metadata prompt should not include full skill bodies: %q", systemPrompt)
	}
}

func TestAgentEnabledSkillsWhitelistLimitsSkillMetadata(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var systemPrompt string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				systemPrompt = msg.Content
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}}})
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Skills = []config.SkillConfig{
		{Name: "review", Description: "Review code", Prompt: "full review", Enabled: true, Scope: "global"},
		{Name: "docx", Description: "Work with docx", Prompt: "full docx", Enabled: true, Scope: "global"},
	}
	cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", EnabledSkills: []string{"review"}}}

	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "skill-one", "default", "hello")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(systemPrompt, `<skill name="review"`) {
		t.Fatalf("expected review skill metadata, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, `<skill name="docx"`) {
		t.Fatalf("did not expect docx metadata for whitelisted agent, got %q", systemPrompt)
	}
}

func TestExplicitSkillCommandInjectsFullSkillContent(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var systemPrompt string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				systemPrompt = msg.Content
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}}})
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Skills = []config.SkillConfig{{Name: "review", Description: "Review code", Prompt: "FULL REVIEW BODY", Enabled: true, Scope: "global"}}
	cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", EnabledSkills: []string{"review"}}}

	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "skill-force", "default", "/skill:review inspect this")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(systemPrompt, "[Skill review]") || !strings.Contains(systemPrompt, "FULL REVIEW BODY") {
		t.Fatalf("expected full skill content from explicit command, got %q", systemPrompt)
	}
}

func TestSubagentProfilePersistsEnabledSkills(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	cfg := config.Default()
	a := agent.New(cfg)

	profile, err := a.UpsertSubagentProfile(config.SubagentProfile{
		ID:            "reviewer",
		Name:          "Reviewer",
		EnabledSkills: []string{"review"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.EnabledSkills) != 1 || profile.EnabledSkills[0] != "review" {
		t.Fatalf("expected enabled skill to persist, got %+v", profile)
	}
	profiles := a.SubagentProfiles()
	if len(profiles) != 1 || len(profiles[0].EnabledSkills) != 1 || profiles[0].EnabledSkills[0] != "review" {
		t.Fatalf("expected stored subagent profile enabled skills, got %+v", profiles)
	}
}
