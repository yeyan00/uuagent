package project_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultStoreUsesUUAgentHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("UUAGENT_HOME", home)
	store := project.NewStore("")

	p, err := store.Create("Home Project", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(p.WorkspacePath, filepath.Join(home, "projects")) {
		t.Fatalf("workspace should be under UUAGENT_HOME projects dir, got %s", p.WorkspacePath)
	}
	if _, err := os.Stat(filepath.Join(home, "projects.json")); err != nil {
		t.Fatalf("projects registry should be under UUAGENT_HOME: %v", err)
	}
}

func TestListFiltersStaleWorkspaceEntries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	if err := os.MkdirAll(filepath.Dir(root), 0755); err != nil {
		t.Fatal(err)
	}
	liveWorkspace := filepath.Join(t.TempDir(), "live")
	if err := os.MkdirAll(liveWorkspace, 0755); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(filepath.Dir(root), "projects.json")
	data, err := json.Marshal([]project.Project{
		{ID: "live", Name: "Live", WorkspacePath: liveWorkspace},
		{ID: "stale", Name: "Stale", WorkspacePath: filepath.Join(t.TempDir(), "missing")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	list := project.NewStore(root).List()
	if len(list) != 1 || list[0].ID != "live" {
		t.Fatalf("expected only live project, got %+v", list)
	}
}

func TestCreateProjectWithFileAsWorkspacePath(t *testing.T) {
	root := t.TempDir()
	store := project.NewStore(root)

	// Create a temporary file
	tempFile := filepath.Join(root, "not-a-dir.txt")
	if err := os.WriteFile(tempFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Attempt to create project with file path as workspace
	_, err := store.Create("Test Project", tempFile)
	if err == nil {
		t.Fatal("expected error when workspace path is a file, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "workspace_path") && !strings.Contains(errMsg, "workspace path") {
		t.Fatalf("error message should mention 'workspace_path' or 'workspace path', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "not a directory") && !strings.Contains(errMsg, "not a dir") {
		t.Fatalf("error message should mention 'not a directory', got: %s", errMsg)
	}
}

func TestCreateProjectWithUnicodeWorkspacePath(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "项目 workspace")
	store := project.NewStore(filepath.Join(root, "uuagent"))
	p, err := store.Create("Unicode Project", workspace)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Temporary {
		t.Fatal("did not expect temporary project")
	}
	if p.WorkspacePath != filepath.Clean(workspace) {
		t.Fatalf("unexpected workspace: %s", p.WorkspacePath)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".uuagent")); err != nil {
		t.Fatalf("project config dir not created: %v", err)
	}
}
