package session_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestSessionAPIGetPatchDelete(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	gin.SetMode(gin.TestMode)
	a := agent.New(config.Default())
	s := a.Sessions().GetOrCreate("api-session")
	s.Append("user", "hello")

	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/api-session", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "api-session" {
		t.Fatalf("unexpected session: %+v", got)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/sessions/api-session", bytes.NewBufferString(`{"title":"Readable Title"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("Readable Title")) {
		t.Fatalf("patch status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/sessions/api-session", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}
	if _, ok := a.Sessions().Get("api-session"); ok {
		t.Fatal("session still exists after delete")
	}
}
