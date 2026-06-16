package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/memory"
)

func TestMemoryPersistsToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	m := memory.NewManagerAt(path)
	entry := m.Add("remember this", "project-a", "project", "user", memory.StatusConfirmed)
	if entry.ID == "" {
		t.Fatal("expected id")
	}

	reloaded := memory.NewManagerAt(path)
	list := reloaded.List(memory.StatusConfirmed, "project-a")
	if len(list) != 1 || list[0].Content != "remember this" {
		t.Fatalf("memory did not reload: %+v", list)
	}

	if !reloaded.Edit(entry.ID, "updated") {
		t.Fatal("edit failed")
	}
	reloadedAgain := memory.NewManagerAt(path)
	list = reloadedAgain.List(memory.StatusConfirmed, "project-a")
	if len(list) != 1 || list[0].Content != "updated" {
		t.Fatalf("edit did not persist: %+v", list)
	}

	if !reloadedAgain.Delete(entry.ID) {
		t.Fatal("delete failed")
	}
	deleted := memory.NewManagerAt(path).List(memory.StatusDeleted, "project-a")
	if len(deleted) != 1 || deleted[0].Status != memory.StatusDeleted {
		t.Fatalf("delete did not persist: %+v", deleted)
	}
}

func TestMemoryBuildSystemPromptOnlyConfirmed(t *testing.T) {
	m := memory.NewManagerAt("")
	m.Add("confirmed memory", "p", "project", "user", memory.StatusConfirmed)
	m.Add("draft memory", "p", "project", "ai", memory.StatusDraft)
	prompt := m.BuildSystemPrompt("p")
	if prompt == "" || !strings.Contains(prompt, "confirmed memory") || strings.Contains(prompt, "draft memory") {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestMemoryListFiltersByScope(t *testing.T) {
	m := memory.NewManagerAt("")
	m.Add("project memory", "p", "project", "user", memory.StatusConfirmed)
	m.Add("session memory", "p", "session", "user", memory.StatusConfirmed)
	m.Add("other project session memory", "other", "session", "user", memory.StatusConfirmed)

	list := m.ListFiltered(memory.StatusConfirmed, "p", "session")
	if len(list) != 1 {
		t.Fatalf("expected one scoped memory, got %+v", list)
	}
	if list[0].Content != "session memory" || list[0].Scope != "session" || list[0].Project != "p" {
		t.Fatalf("unexpected scoped memory: %+v", list[0])
	}
}

func TestMarkdownMemoryFilesBuildPrompt(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(home), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".uuagent"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "user.md"), []byte("# User\n\n- Always answer in Chinese.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "memory.md"), []byte("# Global Memory\n\n- Prefer small code changes.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "memory.draft.md"), []byte("# Draft\n\n- This draft must not be injected.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".uuagent", "user.md"), []byte("# Project User\n\n- Use repository-local conventions.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".uuagent", "memory.md"), []byte("# Project Memory\n\n- This repo uses Gin.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m := memory.NewManagerAt(filepath.Join(home, "memory.json"))
	prompt := m.BuildScopedSystemPrompt(project, "", "")
	for _, want := range []string{"Always answer in Chinese", "Prefer small code changes", "Use repository-local conventions", "This repo uses Gin"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "draft must not be injected") {
		t.Fatalf("draft markdown should not be injected: %s", prompt)
	}
}

func TestDraftMemoryWritesMarkdownDraftFile(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	m := memory.NewManagerAt(filepath.Join(home, "memory.json"))

	m.AddDraft("User prefers markdown memory", project)

	draftPath := filepath.Join(project, ".uuagent", "memory.draft.md")
	data, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "User prefers markdown memory") {
		t.Fatalf("draft markdown did not contain memory: %s", string(data))
	}
	if strings.Contains(m.BuildScopedSystemPrompt(project, "", ""), "User prefers markdown memory") {
		t.Fatal("draft markdown should not be injected into prompt")
	}
}

func TestConfirmedProjectMemoryWritesMarkdownMemoryFile(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	m := memory.NewManagerAt(filepath.Join(home, "memory.json"))

	m.Add("User confirmed markdown memory", project, "project", "user", memory.StatusConfirmed)

	memoryPath := filepath.Join(project, ".uuagent", "memory.md")
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "User confirmed markdown memory") {
		t.Fatalf("confirmed markdown did not contain memory: %s", string(data))
	}
	if !strings.Contains(m.BuildScopedSystemPrompt(project, "", ""), "User confirmed markdown memory") {
		t.Fatal("confirmed markdown should be injected into prompt")
	}
}

func TestListIncludesMarkdownMemoryFiles(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".uuagent"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".uuagent", "memory.md"), []byte("# Project Memory\n\n- Markdown listed memory\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".uuagent", "memory.draft.md"), []byte("# Draft Memory\n\n- Markdown listed draft\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := memory.NewManagerAt(filepath.Join(home, "memory.json"))

	confirmed := m.ListFiltered(memory.StatusConfirmed, project, "project")
	if len(confirmed) != 1 || confirmed[0].Content != "Markdown listed memory" {
		t.Fatalf("expected confirmed markdown memory in list, got %+v", confirmed)
	}
	drafts := m.ListFiltered(memory.StatusDraft, project, "project")
	if len(drafts) != 1 || drafts[0].Content != "Markdown listed draft" {
		t.Fatalf("expected draft markdown memory in list, got %+v", drafts)
	}
}

func TestMarkdownProjectMemoryDoesNotDuplicateIntoJSON(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(home, "memory.json")
	m := memory.NewManagerAt(jsonPath)

	m.Add("Markdown primary memory", project, "project", "user", memory.StatusConfirmed)
	m.AddDraft("Markdown primary draft", project)

	reloaded := memory.NewManagerAt(jsonPath)
	confirmed := reloaded.ListFiltered(memory.StatusConfirmed, project, "project")
	if len(confirmed) != 1 || confirmed[0].Source != "markdown" || confirmed[0].Content != "Markdown primary memory" {
		t.Fatalf("expected only markdown confirmed memory after reload, got %+v", confirmed)
	}
	drafts := reloaded.ListFiltered(memory.StatusDraft, project, "project")
	if len(drafts) != 1 || drafts[0].Source != "markdown" || drafts[0].Content != "Markdown primary draft" {
		t.Fatalf("expected only markdown draft memory after reload, got %+v", drafts)
	}
}
