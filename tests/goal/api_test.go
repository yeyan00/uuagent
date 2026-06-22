package goal_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func Test_GoalAPI_Create_returns_goal_with_plan_todos_and_activity(t *testing.T) {
	// Given
	r, projectID := newGoalAPIServer(t)
	body := strings.NewReader(`{"prompt":"Ship goal mode backend"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/goals", body)
	req.Header.Set("Content-Type", "application/json")

	// When
	r.ServeHTTP(rec, req)

	// Then
	if rec.Code != http.StatusOK {
		t.Fatalf("create goal status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got goalAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode create goal response: %v", err)
	}
	if got.ID == "" || got.ProjectID != projectID || got.Prompt != "Ship goal mode backend" {
		t.Fatalf("unexpected goal response: %+v", got)
	}
	if len(got.Plan.Todos) == 0 {
		t.Fatalf("expected plan todos in create response: %+v", got.Plan)
	}
	if len(got.Activities) == 0 || got.Activities[0].Kind != "goal_created" {
		t.Fatalf("expected create activity, got %+v", got.Activities)
	}
}

func Test_GoalAPI_List_and_Get_return_project_goal(t *testing.T) {
	// Given
	r, projectID := newGoalAPIServer(t)
	created := createGoalViaAPI(t, r, projectID, "Listable goal")

	// When
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/goals", nil))
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/goals/"+created.ID, nil))

	// Then
	if listRec.Code != http.StatusOK {
		t.Fatalf("list goals status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	if getRec.Code != http.StatusOK {
		t.Fatalf("get goal status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), created.ID) || !strings.Contains(getRec.Body.String(), "Listable goal") {
		t.Fatalf("list/get responses did not include goal: list=%s get=%s", listRec.Body.String(), getRec.Body.String())
	}
}

func Test_GoalAPI_Stop_cancels_running_goal(t *testing.T) {
	// Given
	r, projectID := newGoalAPIServer(t)
	created := createGoalViaAPI(t, r, projectID, "Stoppable goal")

	// When
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/goals/"+created.ID+"/stop", nil))

	// Then
	if rec.Code != http.StatusOK {
		t.Fatalf("stop goal status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stopped goalAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stopped); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if stopped.Status != "cancelled" {
		t.Fatalf("expected cancelled goal, got %+v", stopped)
	}
}

func Test_GoalAPI_Run_creates_persisted_delegated_activity(t *testing.T) {
	// Given
	r, projectID := newGoalAPIServer(t)
	created := createGoalViaAPI(t, r, projectID, "Run delegated goal")

	// When
	runRec := httptest.NewRecorder()
	r.ServeHTTP(runRec, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/goals/"+created.ID+"/run", nil))
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/goals/"+created.ID, nil))

	// Then
	if runRec.Code != http.StatusOK {
		t.Fatalf("run goal status=%d body=%s", runRec.Code, runRec.Body.String())
	}
	if getRec.Code != http.StatusOK {
		t.Fatalf("get goal status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var reloaded goalAPIResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &reloaded); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if reloaded.Status != "done" {
		t.Fatalf("expected done goal after run, got %+v", reloaded)
	}
	for _, kind := range []string{"goal_started", "todo_started", "delegate_started", "delegate_completed", "todo_completed", "goal_completed"} {
		if !hasAPIActivityKind(reloaded.Activities, kind) {
			t.Fatalf("expected %s activity after run, got %+v", kind, reloaded.Activities)
		}
	}
	if !hasAPIActivity(reloaded.Activities, "delegate_completed", "reviewer") {
		t.Fatalf("expected reviewer delegated completion activity, got %+v", reloaded.Activities)
	}
}

type goalAPIResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Prompt    string `json:"prompt"`
	Status    string `json:"status"`
	Plan      struct {
		Todos []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	} `json:"plan"`
	Activities []struct {
		Kind      string `json:"kind"`
		ProfileID string `json:"profile_id"`
	} `json:"activities"`
}

func hasAPIActivityKind(activities []struct {
	Kind      string `json:"kind"`
	ProfileID string `json:"profile_id"`
}, kind string) bool {
	for _, activity := range activities {
		if activity.Kind == kind {
			return true
		}
	}
	return false
}

func hasAPIActivity(activities []struct {
	Kind      string `json:"kind"`
	ProfileID string `json:"profile_id"`
}, kind string, profileID string) bool {
	for _, activity := range activities {
		if activity.Kind == kind && activity.ProfileID == profileID {
			return true
		}
	}
	return false
}

func newGoalAPIServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"goal delegate completed"}}]}`))
	}))
	t.Cleanup(llm.Close)
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	a := agent.New(cfg)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)
	return r, createGoalProject(t, r, workspace)
}

func createGoalProject(t *testing.T, r http.Handler, workspace string) string {
	t.Helper()
	payload := `{"name":"Goal Test","workspace_path":"` + strings.ReplaceAll(workspace, `\`, `\\`) + `"}`
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("create project status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got.ID
}

func createGoalViaAPI(t *testing.T, r http.Handler, projectID string, prompt string) goalAPIResponse {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/goals", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create goal status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got goalAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}
