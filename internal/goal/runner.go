package goal

import (
	"context"
	"fmt"
	"sync"

	"github.com/yeyan00/uuagent/internal/config"
)

type Delegate interface {
	Delegate(ctx context.Context, profileID string, task string) (DelegateResult, error)
}

type DelegateFunc func(ctx context.Context, profileID string, task string) (DelegateResult, error)

func (f DelegateFunc) Delegate(ctx context.Context, profileID string, task string) (DelegateResult, error) {
	return f(ctx, profileID, task)
}

type DelegateResult struct {
	ProfileID string `json:"profile_id"`
	Task      string `json:"task"`
	Output    string `json:"output"`
}

type RunnerConfig struct {
	Subagents []config.SubagentProfile
	Delegate  Delegate
}

type Runner struct {
	store         *Store
	cfg           RunnerConfig
	mu            sync.Mutex
	stopAfterNext map[string]bool
}

func NewRunner(store *Store, cfg RunnerConfig) *Runner {
	return &Runner{store: store, cfg: cfg, stopAfterNext: map[string]bool{}}
}

func (r *Runner) Run(ctx context.Context, id string) (Goal, error) {
	if err := r.store.MarkRunning(ctx, id); err != nil {
		return Goal{}, err
	}
	goal, err := r.store.Get(ctx, id)
	if err != nil {
		return Goal{}, err
	}
	if len(goal.Plan.Todos) == 0 {
		goal.Plan = r.plan(goal)
	}
	for i := range goal.Plan.Todos {
		if goal.Status == StatusCancelled {
			return r.store.Get(ctx, id)
		}
		profileID := r.profileID(i)
		result, err := r.delegate(ctx, profileID, goal.Plan.Todos[i].Content)
		if err != nil {
			return Goal{}, fmt.Errorf("delegate %s: %w", profileID, err)
		}
		goal.Plan.Todos[i].Status = TodoDone
		goal.Activities = append(goal.Activities, Activity{Kind: ActivitySubagentCompleted, ProfileID: profileID, Task: result.Task, Output: result.Output})
		if r.consumeStopAfterNext(id) {
			goal.Status = StatusCancelled
			goal.Activities = append(goal.Activities, newActivity(ActivityGoalCancelled))
			if err := r.store.Save(ctx, goal); err != nil {
				return Goal{}, err
			}
			return goal, nil
		}
		if err := r.store.Save(ctx, goal); err != nil {
			return Goal{}, err
		}
	}
	goal.Status = StatusDone
	goal.Activities = append(goal.Activities, newActivity(ActivityGoalDone))
	if err := r.store.Save(ctx, goal); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

func (r *Runner) StopAfterNextActivityForTest(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopAfterNext[id] = true
}

func (r *Runner) plan(goal Goal) Plan {
	todos := make([]Todo, 0, len(r.cfg.Subagents))
	for _, profile := range r.cfg.Subagents {
		id := profile.ID
		if id == "" {
			id = profile.Name
		}
		todos = append(todos, Todo{ID: "todo-" + id, Content: goal.Prompt + " [" + id + "]", Status: TodoPending})
	}
	return Plan{Summary: goal.Prompt, Todos: todos}
}

func (r *Runner) profileID(index int) string {
	if index >= len(r.cfg.Subagents) {
		return ""
	}
	return r.cfg.Subagents[index].ID
}

func (r *Runner) delegate(ctx context.Context, profileID string, task string) (DelegateResult, error) {
	if r.cfg.Delegate == nil {
		return DelegateResult{ProfileID: profileID, Task: task, Output: ""}, nil
	}
	return r.cfg.Delegate.Delegate(ctx, profileID, task)
}

func (r *Runner) consumeStopAfterNext(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.stopAfterNext[id] {
		return false
	}
	delete(r.stopAfterNext, id)
	return true
}
