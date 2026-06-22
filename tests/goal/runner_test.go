package goal_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/goal"
)

func Test_Runner_Run_completes_goal_with_plan_todos_and_subagent_activities(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	runner := goal.NewRunner(store, goal.RunnerConfig{
		Subagents: []config.SubagentProfile{
			{ID: "planner", Name: "Planner"},
			{ID: "explorer", Name: "Explorer"},
			{ID: "builder", Name: "Builder"},
			{ID: "tester", Name: "Tester"},
			{ID: "reviewer", Name: "Reviewer"},
		},
		Delegate: goal.DelegateFunc(func(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
			return goal.DelegateResult{ProfileID: profileID, Task: task, Output: profileID + " completed"}, nil
		}),
	})
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Build goal mode"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// When
	completed, err := runner.Run(ctx, created.ID)

	// Then
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if completed.Status != goal.StatusDone {
		t.Fatalf("expected done goal, got %+v", completed)
	}
	if len(completed.Plan.Todos) != 5 {
		t.Fatalf("expected one todo per default subagent, got %+v", completed.Plan.Todos)
	}
	for _, todo := range completed.Plan.Todos {
		if todo.Status != goal.TodoDone {
			t.Fatalf("expected completed todo, got %+v", todo)
		}
	}
	reloaded, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload completed goal: %v", err)
	}
	if reloaded.Status != goal.StatusDone {
		t.Fatalf("expected persisted done goal, got %+v", reloaded)
	}
	wantActivities := []string{"goal_started", "todo_started", "delegate_started", "delegate_completed", "todo_completed", "goal_completed"}
	for _, kind := range wantActivities {
		if !hasActivityKind(reloaded.Activities, kind) {
			t.Fatalf("expected persisted %s activity, got %+v", kind, reloaded.Activities)
		}
	}
	if !goal.HasActivity(reloaded.Activities, goal.ActivityKind("delegate_completed"), "reviewer") {
		t.Fatalf("reviewer delegate completion activity missing: %+v", reloaded.Activities)
	}
}

func hasActivityKind(activities []goal.Activity, kind string) bool {
	for _, activity := range activities {
		if string(activity.Kind) == kind {
			return true
		}
	}
	return false
}

func Test_Runner_Stop_cancels_goal_before_next_todo(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	delegateCalls := 0
	runner := goal.NewRunner(store, goal.RunnerConfig{
		Subagents: []config.SubagentProfile{{ID: "planner"}, {ID: "builder"}},
		Delegate: goal.DelegateFunc(func(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
			delegateCalls++
			if delegateCalls == 1 {
				return goal.DelegateResult{ProfileID: profileID, Task: task, Output: "planned"}, nil
			}
			t.Fatalf("runner delegated after cancellation to %q", profileID)
			return goal.DelegateResult{}, nil
		}),
	})
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Cancel after first todo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	runner.StopAfterNextActivityForTest(created.ID)

	// When
	stopped, err := runner.Run(ctx, created.ID)

	// Then
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stopped.Status != goal.StatusCancelled {
		t.Fatalf("expected cancelled goal, got %+v", stopped)
	}
	if delegateCalls != 1 {
		t.Fatalf("expected exactly one delegate call before cancellation, got %d", delegateCalls)
	}
}
