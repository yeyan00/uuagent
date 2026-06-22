package goal

import "time"

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusDone      Status = "done"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

type TodoStatus string

const (
	TodoPending TodoStatus = "pending"
	TodoRunning TodoStatus = "running"
	TodoDone    TodoStatus = "done"
)

type ActivityKind string

const (
	ActivityGoalCreated       ActivityKind = "goal_created"
	ActivityGoalStarted       ActivityKind = "goal_started"
	ActivityGoalRunning       ActivityKind = "goal_running"
	ActivityTodoStarted       ActivityKind = "todo_started"
	ActivityDelegateStarted   ActivityKind = "delegate_started"
	ActivityDelegateCompleted ActivityKind = "delegate_completed"
	ActivityTodoCompleted     ActivityKind = "todo_completed"
	ActivityGoalCompleted     ActivityKind = "goal_completed"
	ActivityGoalDone          ActivityKind = "goal_done"
	ActivityGoalFailed        ActivityKind = "goal_failed"
	ActivityGoalStopped       ActivityKind = "goal_stopped"
	ActivityGoalCancelled     ActivityKind = "goal_cancelled"
	ActivitySubagentCompleted ActivityKind = "subagent_completed"
)

type Goal struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	ProjectPath string     `json:"project_path"`
	Prompt      string     `json:"prompt"`
	Status      Status     `json:"status"`
	Plan        Plan       `json:"plan"`
	Activities  []Activity `json:"activities"`
	CreatedAt   int64      `json:"created_at"`
	UpdatedAt   int64      `json:"updated_at"`
}

type Plan struct {
	Summary string `json:"summary"`
	Todos   []Todo `json:"todos"`
}

type Todo struct {
	ID      string     `json:"id"`
	Content string     `json:"content"`
	Status  TodoStatus `json:"status"`
}

type Activity struct {
	Kind      ActivityKind `json:"kind"`
	TodoID    string       `json:"todo_id,omitempty"`
	ProfileID string       `json:"profile_id,omitempty"`
	Task      string       `json:"task,omitempty"`
	Output    string       `json:"output,omitempty"`
	Error     string       `json:"error,omitempty"`
	CreatedAt int64        `json:"created_at"`
}

type CreateRequest struct {
	ProjectID   string `json:"project_id"`
	ProjectPath string `json:"project_path"`
	Prompt      string `json:"prompt"`
	Plan        Plan   `json:"plan"`
}

func newActivity(kind ActivityKind) Activity {
	return Activity{Kind: kind, CreatedAt: time.Now().Unix()}
}

func HasActivity(activities []Activity, kind ActivityKind, profileID string) bool {
	for _, activity := range activities {
		if activity.Kind == kind && activity.ProfileID == profileID {
			return true
		}
	}
	return false
}
