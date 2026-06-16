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
