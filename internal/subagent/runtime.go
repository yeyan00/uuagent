package subagent

import (
	"context"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/session"
	"github.com/yeyan00/uuagent/internal/types"
)

// ParentAgent is the parent runtime capability needed to spawn isolated child subagents.
type ParentAgent interface {
	NewSubagentChildWithSession(model string, blockedTools map[string]bool, store *session.Store) ChildAgent
}

// ChildAgent is the child runtime capability used by delegated subagents.
type ChildAgent interface {
	RunOnce(ctx context.Context, prompt string) (string, error)
	RunWithProfileParts(ctx context.Context, sessionID, projectID string, profile config.AgentProfile, parts []types.ContentPart) (<-chan types.Event, error)
}
