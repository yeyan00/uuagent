package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeyan00/uuagent/internal/session"
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
	for i := 0; i < 12; i++ {
		s.Append("user", "this older message should persist inside compact archive")
	}

	// When
	archive, ok := s.CompactArchive(80, 0.5, 4)

	// Then
	if !ok {
		t.Fatalf("expected compact archive")
	}
	reloaded := session.NewStoreAt(root)
	loaded, ok := reloaded.Get("archive-me")
	if !ok {
		t.Fatalf("expected reloaded session")
	}
	archives := loaded.ListArchives()
	if len(archives) != 1 {
		t.Fatalf("expected one archive, got %d", len(archives))
	}
	if archives[0].ID != archive.ID {
		t.Fatalf("expected archive ID %s, got %s", archive.ID, archives[0].ID)
	}
	if len(archives[0].Messages) != len(archive.Messages) {
		t.Fatalf("expected %d archived messages, got %d", len(archive.Messages), len(archives[0].Messages))
	}
	if archives[0].Messages[0].Content != archive.Messages[0].Content {
		t.Fatalf("expected first archive message to persist")
	}
	snap := loaded.Snapshot()
	if len(snap.Archives) != 1 {
		t.Fatalf("expected snapshot archive copy, got %d", len(snap.Archives))
	}
	snap.Archives[0].Messages[0].Content = "mutated"
	if loaded.ListArchives()[0].Messages[0].Content != archive.Messages[0].Content {
		t.Fatalf("expected snapshot archive mutation not to affect session")
	}
}
