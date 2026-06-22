package goal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	root string
	mu   sync.RWMutex
}

func NewStore(root string) *Store {
	return &Store{root: filepath.Clean(root)}
}

func (s *Store) Create(ctx context.Context, req CreateRequest) (Goal, error) {
	if err := ctx.Err(); err != nil {
		return Goal{}, err
	}
	now := time.Now().Unix()
	goal := Goal{
		ID:          fmt.Sprintf("goal-%d", time.Now().UnixNano()),
		ProjectID:   strings.TrimSpace(req.ProjectID),
		ProjectPath: filepath.Clean(req.ProjectPath),
		Prompt:      req.Prompt,
		Status:      StatusPending,
		Plan:        req.Plan,
		CreatedAt:   now,
		UpdatedAt:   now,
		Activities:  []Activity{newActivity(ActivityGoalCreated)},
	}
	if err := s.save(goal); err != nil {
		return Goal{}, fmt.Errorf("create goal: %w", err)
	}
	return goal, nil
}

func (s *Store) Get(ctx context.Context, id string) (Goal, error) {
	if err := ctx.Err(); err != nil {
		return Goal{}, err
	}
	return s.load(id)
}

func (s *Store) List(ctx context.Context, projectID string) ([]Goal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Goal{}, nil
		}
		return nil, fmt.Errorf("list goals: %w", err)
	}
	goals := make([]Goal, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		goal, err := s.load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if goal.ProjectID == projectID {
			goals = append(goals, goal)
		}
	}
	sort.Slice(goals, func(i, j int) bool { return goals[i].CreatedAt < goals[j].CreatedAt })
	return goals, nil
}

func (s *Store) MarkRunning(ctx context.Context, id string) error {
	return s.update(ctx, id, func(goal *Goal) {
		goal.Status = StatusRunning
		goal.Activities = append(goal.Activities, newActivity(ActivityGoalRunning))
	})
}

func (s *Store) Stop(ctx context.Context, id string) (Goal, error) {
	if err := s.update(ctx, id, func(goal *Goal) {
		goal.Status = StatusCancelled
		goal.Activities = append(goal.Activities, newActivity(ActivityGoalCancelled))
	}); err != nil {
		return Goal{}, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Save(ctx context.Context, goal Goal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	goal.UpdatedAt = time.Now().Unix()
	return s.save(goal)
}

func (s *Store) update(ctx context.Context, id string, mutate func(*Goal)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	goal, err := s.load(id)
	if err != nil {
		return err
	}
	mutate(&goal)
	goal.UpdatedAt = time.Now().Unix()
	return s.save(goal)
}

func (s *Store) load(id string) (Goal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Goal{}, fmt.Errorf("read goal %s: %w", id, err)
	}
	var goal Goal
	if err := json.Unmarshal(data, &goal); err != nil {
		return Goal{}, fmt.Errorf("decode goal %s: %w", id, err)
	}
	return goal, nil
}

func (s *Store) save(goal Goal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return fmt.Errorf("ensure goal store: %w", err)
	}
	data, err := json.MarshalIndent(goal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode goal %s: %w", goal.ID, err)
	}
	return os.WriteFile(s.path(goal.ID), data, 0600)
}

func (s *Store) path(id string) string {
	return filepath.Join(s.root, filepath.Clean(id)+".json")
}
