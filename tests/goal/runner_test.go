package goal_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
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

func Test_Runner_Run_records_lifecycle_activities_in_order(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	runner := goal.NewRunner(store, goal.RunnerConfig{
		Subagents: []config.SubagentProfile{{ID: "planner"}},
		Delegate: goal.DelegateFunc(func(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
			return goal.DelegateResult{ProfileID: profileID, Task: task, Output: "planned"}, nil
		}),
	})
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Order lifecycle"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// When
	completed, err := runner.Run(ctx, created.ID)

	// Then
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"goal_created", "goal_started", "goal_running", "todo_started", "delegate_started", "delegate_completed", "subagent_completed", "todo_completed", "goal_completed", "goal_done"}
	got := activityKinds(completed.Activities)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("activity order mismatch\nwant: %+v\n got: %+v", want, got)
	}
}

func Test_Runner_Run_returns_terminal_goal_without_delegating_or_duplicating_activities(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	delegateCalls := 0
	runner := goal.NewRunner(store, goal.RunnerConfig{
		Subagents: []config.SubagentProfile{{ID: "planner"}},
		Delegate: goal.DelegateFunc(func(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
			delegateCalls++
			return goal.DelegateResult{ProfileID: profileID, Task: task, Output: "planned"}, nil
		}),
	})
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Run once"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	completed, err := runner.Run(ctx, created.ID)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	beforeActivities := activityKinds(completed.Activities)

	// When
	again, err := runner.Run(ctx, created.ID)

	// Then
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if again.Status != goal.StatusDone {
		t.Fatalf("expected done goal, got %+v", again)
	}
	if delegateCalls != 1 {
		t.Fatalf("expected no second delegation for terminal goal, got %d calls", delegateCalls)
	}
	if !reflect.DeepEqual(activityKinds(again.Activities), beforeActivities) {
		t.Fatalf("terminal rerun duplicated activities\nbefore: %+v\n after: %+v", beforeActivities, activityKinds(again.Activities))
	}
}

func Test_Runner_Run_returns_cancelled_goal_without_delegating(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	delegateCalls := 0
	runner := goal.NewRunner(store, goal.RunnerConfig{
		Subagents: []config.SubagentProfile{{ID: "planner"}},
		Delegate: goal.DelegateFunc(func(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
			delegateCalls++
			return goal.DelegateResult{ProfileID: profileID, Task: task, Output: "planned"}, nil
		}),
	})
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Do not run cancelled"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cancelled, err := store.Stop(ctx, created.ID)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	beforeActivities := activityKinds(cancelled.Activities)

	// When
	after, err := runner.Run(ctx, created.ID)

	// Then
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if after.Status != goal.StatusCancelled {
		t.Fatalf("expected cancelled goal to stay cancelled, got %+v", after)
	}
	if delegateCalls != 0 {
		t.Fatalf("expected no delegation for cancelled goal, got %d", delegateCalls)
	}
	if !reflect.DeepEqual(activityKinds(after.Activities), beforeActivities) {
		t.Fatalf("cancelled rerun changed activities\nbefore: %+v\n after: %+v", beforeActivities, activityKinds(after.Activities))
	}
}

func Test_Runner_Run_persists_cancelled_status_when_delegate_context_cancelled(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	runner := goal.NewRunner(store, goal.RunnerConfig{
		Subagents: []config.SubagentProfile{{ID: "planner"}},
		Delegate: goal.DelegateFunc(func(ctx context.Context, profileID string, task string) (goal.DelegateResult, error) {
			return goal.DelegateResult{}, context.Canceled
		}),
	})
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Cancel during delegate"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// When
	_, err = runner.Run(ctx, created.ID)

	// Then
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	reloaded, err := store.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("reload cancelled goal: %v", err)
	}
	if reloaded.Status != goal.StatusCancelled {
		t.Fatalf("expected persisted cancelled status, got %+v", reloaded)
	}
	if !hasActivityKind(reloaded.Activities, "goal_cancelled") {
		t.Fatalf("expected cancellation activity, got %+v", reloaded.Activities)
	}
}

func activityKinds(activities []goal.Activity) []string {
	kinds := make([]string, 0, len(activities))
	for _, activity := range activities {
		kinds = append(kinds, string(activity.Kind))
	}
	return kinds
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
