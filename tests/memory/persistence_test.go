package memory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/uuagent/uuagent/internal/memory"
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
