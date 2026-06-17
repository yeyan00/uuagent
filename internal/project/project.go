package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yeyan00/uuagent/internal/paths"
)

// Project describes a local UUAgent workspace.
type Project struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	WorkspacePath string `json:"workspace_path"`
	ConfigPath    string `json:"config_path"`
	Temporary     bool   `json:"temporary"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// Store is a lightweight persistent project registry.
type Store struct {
	root     string
	path     string
	projects map[string]Project
	mu       sync.RWMutex
}

// NewStore creates a project store. Empty root defaults to ~/.uuagent/projects.
func NewStore(root string) *Store {
	if root == "" {
		root = paths.ProjectsDir()
	}
	s := &Store{root: root, path: filepath.Join(filepath.Dir(root), "projects.json"), projects: map[string]Project{}}
	_ = s.Load()
	return s
}

// Load reads the project registry from disk if present.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []Project
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, p := range list {
		s.projects[p.ID] = p
	}
	return nil
}

// Save persists the project registry to disk.
func (s *Store) Save() error {
	list := make([]Project, 0, len(s.projects))
	for _, p := range s.projects {
		list = append(list, p)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// Create registers a project. If workspace is empty, a temporary workspace is
// created under ~/.uuagent/projects/<id>/workspace.
func (s *Store) Create(name, workspace string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(name) == "" {
		name = "Untitled Project"
	}
	id := slug(name)
	if id == "" {
		id = fmt.Sprintf("project-%d", time.Now().UnixNano())
	}
	if _, exists := s.projects[id]; exists {
		id = fmt.Sprintf("%s-%d", id, time.Now().Unix())
	}

	temporary := false
	if strings.TrimSpace(workspace) == "" {
		temporary = true
		workspace = filepath.Join(s.root, id, "workspace")
	}
	workspace = filepath.Clean(workspace)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return Project{}, err
	}

	configDir := filepath.Join(workspace, ".uuagent")
	if temporary {
		configDir = filepath.Join(s.root, id)
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return Project{}, err
	}

	now := time.Now().Unix()
	p := Project{
		ID:            id,
		Name:          name,
		WorkspacePath: workspace,
		ConfigPath:    filepath.Join(configDir, "project.yaml"),
		Temporary:     temporary,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.projects[id] = p
	if err := s.Save(); err != nil {
		return Project{}, err
	}
	return p, nil
}

// List returns all registered projects.
func (s *Store) List() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Project, 0, len(s.projects))
	for _, p := range s.projects {
		if p.WorkspacePath != "" {
			if info, err := os.Stat(p.WorkspacePath); err != nil || !info.IsDir() {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// Get returns one project.
func (s *Store) Get(id string) (Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	return p, ok
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
