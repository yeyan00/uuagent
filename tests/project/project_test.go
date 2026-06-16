package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeyan00/uuagent/internal/project"
)

func TestCreateTemporaryProject(t *testing.T) {
	root := t.TempDir()
	store := project.NewStore(root)
	p, err := store.Create("My Temp Project", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.Temporary {
		t.Fatalf("expected temporary project")
	}
	if _, err := os.Stat(p.WorkspacePath); err != nil {
		t.Fatalf("workspace not created: %v", err)
	}
	if filepath.Base(p.ConfigPath) != "project.yaml" {
		t.Fatalf("unexpected config path: %s", p.ConfigPath)
	}
}

func TestCreateLocalProject(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace with spaces")
	store := project.NewStore(filepath.Join(root, "uuagent"))
	p, err := store.Create("Local", workspace)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Temporary {
		t.Fatalf("did not expect temporary project")
	}
	if p.WorkspacePath != filepath.Clean(workspace) {
		t.Fatalf("unexpected workspace: %s", p.WorkspacePath)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".uuagent")); err != nil {
		t.Fatalf("project config dir not created: %v", err)
	}
}
