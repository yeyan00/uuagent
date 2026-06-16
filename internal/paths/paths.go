package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// UserDir returns the UUAgent user directory. Tests can override it with
// UUAGENT_HOME so they do not pollute the real user profile.
func UserDir() string {
	if v := strings.TrimSpace(os.Getenv("UUAGENT_HOME")); v != "" {
		return filepath.Clean(v)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".uuagent"
	}
	return filepath.Join(home, ".uuagent")
}

func SessionsDir() string { return filepath.Join(UserDir(), "sessions") }
func ProjectsDir() string { return filepath.Join(UserDir(), "projects") }
