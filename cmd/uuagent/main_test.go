package main

import (
	"path/filepath"
	"testing"
)

func TestResolveWebUIDirFindsRepoWebDistFromDistExecutable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	webDist := filepath.Join(root, "web", "dist")
	exePath := filepath.Join(root, "dist", "uuagent.exe")
	cwd := filepath.Join(t.TempDir(), "outside")

	got := resolveWebUIDir(cwd, exePath, func(path string) bool {
		return filepath.Clean(path) == filepath.Clean(webDist)
	})

	if got != filepath.Clean(webDist) {
		t.Fatalf("expected %q, got %q", filepath.Clean(webDist), got)
	}
}
