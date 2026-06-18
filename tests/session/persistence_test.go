package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeyan00/uuagent/internal/contextmgr"
	"github.com/yeyan00/uuagent/internal/session"
	"github.com/yeyan00/uuagent/internal/types"
)

func TestSessionPersistsToJSON(t *testing.T) {
	root := t.TempDir()
	store := session.NewStoreAt(root)
	s := store.GetOrCreate("persist-me")
	s.Append("user", "hello")
	s.Append("assistant", "hi")

	path := filepath.Join(root, "persist-me.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected session file %s: %v", path, err)
	}

	reloaded := session.NewStoreAt(root)
	loaded, ok := reloaded.Get("persist-me")
	if !ok {
		t.Fatalf("expected reloaded session")
	}
	snap := loaded.Snapshot()
	if len(snap.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(snap.Messages))
	}
	if snap.Messages[0].Content != "hello" || snap.Messages[1].Content != "hi" {
		t.Fatalf("unexpected messages: %+v", snap.Messages)
	}
}

func TestForkPersistsChildSession(t *testing.T) {
	root := t.TempDir()
	store := session.NewStoreAt(root)
	parent := store.GetOrCreate("parent")
	parent.Append("user", "one")
	child, err := store.Fork("parent", "child", -1)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ParentID != "parent" {
		t.Fatalf("unexpected parent id: %s", child.ParentID)
	}
	if _, err := os.Stat(filepath.Join(root, "child.json")); err != nil {
		t.Fatalf("expected child session file: %v", err)
	}
}

func TestCompactArchivesPersistToJSON(t *testing.T) {
	// Given
	root := t.TempDir()
	store := session.NewStoreAt(root)
	s := store.GetOrCreate("archive-me")
	archive := session.CompactArchive{
		ID:        "archive-1",
		SummaryID: "summary-1",
		Messages: []types.Message{
			{Role: "user", Content: "old prompt"},
			{Role: "assistant", Content: "old answer"},
		},
		CreatedAt: 123,
	}
	summary := contextmgr.Summary{ID: "summary-1", SessionID: "archive-me", FromIndex: 0, ToIndex: 1, Summary: "archived", CreatedAt: 123}

	// When
	s.CompactArchive(summary, archive)

	// Then
	reloaded := session.NewStoreAt(root)
	loaded, ok := reloaded.Get("archive-me")
	if !ok {
		t.Fatalf("expected reloaded session")
	}
	archives := loaded.ListArchives()
	if len(archives) != 1 {
		t.Fatalf("expected one archive, got %d", len(archives))
	}
	if archives[0].Messages[1].Content != "old answer" {
		t.Fatalf("expected archive message to persist, got %+v", archives[0].Messages[1])
	}
	snap := loaded.Snapshot()
	if len(snap.Archives) != 1 {
		t.Fatalf("expected snapshot archive copy, got %d", len(snap.Archives))
	}
	snap.Archives[0].Messages[0].Content = "mutated"
	if loaded.ListArchives()[0].Messages[0].Content != "old prompt" {
		t.Fatalf("expected snapshot archive mutation not to affect session")
	}
}
