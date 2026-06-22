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
		if goal.Plan.Todos[i].Status == TodoDone {
			continue
		}
		profileID := r.profileID(i)
		goal.Plan.Todos[i].Status = TodoRunning
		goal.Activities = append(goal.Activities,
			activityForTodo(ActivityTodoStarted, goal.Plan.Todos[i], profileID, ""),
			activityForTodo(ActivityDelegateStarted, goal.Plan.Todos[i], profileID, ""),
		)
		if err := r.store.Save(ctx, goal); err != nil {
			return Goal{}, err
		}
		result, err := r.delegate(ctx, profileID, goal.Plan.Todos[i].Content)
		if err != nil {
			goal.Status = StatusFailed
			goal.Activities = append(goal.Activities, activityForTodo(ActivityGoalFailed, goal.Plan.Todos[i], profileID, err.Error()))
			if saveErr := r.store.Save(ctx, goal); saveErr != nil {
				return Goal{}, saveErr
			}
			return Goal{}, fmt.Errorf("delegate %s: %w", profileID, err)
		}
		goal.Plan.Todos[i].Status = TodoDone
		goal.Activities = append(goal.Activities,
			activityForResult(ActivityDelegateCompleted, goal.Plan.Todos[i], profileID, result),
			activityForResult(ActivitySubagentCompleted, goal.Plan.Todos[i], profileID, result),
			activityForResult(ActivityTodoCompleted, goal.Plan.Todos[i], profileID, result),
		)
		if r.consumeStopAfterNext(id) {
			goal.Status = StatusCancelled
			goal.Activities = append(goal.Activities, newActivity(ActivityGoalStopped), newActivity(ActivityGoalCancelled))
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
	goal.Activities = append(goal.Activities, newActivity(ActivityGoalCompleted), newActivity(ActivityGoalDone))
	if err := r.store.Save(ctx, goal); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

func activityForTodo(kind ActivityKind, todo Todo, profileID string, message string) Activity {
	activity := newActivity(kind)
	activity.TodoID = todo.ID
	activity.ProfileID = profileID
	activity.Task = todo.Content
	activity.Error = message
	return activity
}

func activityForResult(kind ActivityKind, todo Todo, profileID string, result DelegateResult) Activity {
	activity := activityForTodo(kind, todo, profileID, "")
	activity.Task = result.Task
	activity.Output = result.Output
	return activity
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
