package goal_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yeyan00/uuagent/internal/goal"
)

func Test_Store_Create_persists_goal_with_plan_todos_and_activities(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	request := goal.CreateRequest{
		ProjectID:   "project-a",
		ProjectPath: t.TempDir(),
		Prompt:      "Ship goal mode",
		Plan: goal.Plan{
			Summary: "Implement goal mode backend",
			Todos: []goal.Todo{
				{ID: "todo-1", Content: "Plan backend", Status: goal.TodoPending},
				{ID: "todo-2", Content: "Run subagents", Status: goal.TodoPending},
			},
		},
	}

	// When
	created, err := store.Create(ctx, request)

	// Then
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	loaded, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.ProjectID != "project-a" || loaded.Prompt != "Ship goal mode" {
		t.Fatalf("goal metadata was not persisted: %+v", loaded)
	}
	if len(loaded.Plan.Todos) != 2 || loaded.Plan.Todos[0].Status != goal.TodoPending {
		t.Fatalf("plan todos were not persisted: %+v", loaded.Plan.Todos)
	}
	if len(loaded.Activities) == 0 || loaded.Activities[0].Kind != goal.ActivityGoalCreated {
		t.Fatalf("create activity was not recorded: %+v", loaded.Activities)
	}
}

func Test_Store_Stop_marks_running_goal_cancelled(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	created, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "Stop me"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.MarkRunning(ctx, created.ID); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}

	// When
	stopped, err := store.Stop(ctx, created.ID)

	// Then
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped.Status != goal.StatusCancelled {
		t.Fatalf("expected cancelled goal, got %+v", stopped)
	}
	if len(stopped.Activities) == 0 || stopped.Activities[len(stopped.Activities)-1].Kind != goal.ActivityGoalCancelled {
		t.Fatalf("cancel activity was not recorded: %+v", stopped.Activities)
	}
}

func Test_Store_List_returns_project_goals_only(t *testing.T) {
	// Given
	ctx := context.Background()
	store := goal.NewStore(filepath.Join(t.TempDir(), "goals"))
	if _, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-a", ProjectPath: t.TempDir(), Prompt: "A"}); err != nil {
		t.Fatalf("Create project A: %v", err)
	}
	if _, err := store.Create(ctx, goal.CreateRequest{ProjectID: "project-b", ProjectPath: t.TempDir(), Prompt: "B"}); err != nil {
		t.Fatalf("Create project B: %v", err)
	}

	// When
	goals, err := store.List(ctx, "project-a")

	// Then
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(goals) != 1 || goals[0].ProjectID != "project-a" || goals[0].Prompt != "A" {
		t.Fatalf("expected only project-a goals, got %+v", goals)
	}
}
