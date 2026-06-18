package session_test

import (
	"testing"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/contextmgr"
	"github.com/yeyan00/uuagent/internal/session"
	"github.com/yeyan00/uuagent/internal/types"
)

func TestSessionCompressionSummaries(t *testing.T) {
	store := session.NewStoreAt(t.TempDir())
	s := store.GetOrCreate("s1")
	for i := 0; i < 20; i++ {
		s.Append("user", "this is a long message that should contribute to token estimation and trigger local compression")
	}
	summary, ok := s.MaybeCompress(50, 0.5, 4)
	if !ok {
		t.Fatalf("expected compression")
	}
	if summary.TokenAfter >= summary.TokenBefore {
		t.Fatalf("expected token reduction: before=%d after=%d", summary.TokenBefore, summary.TokenAfter)
	}
	if len(s.ListSummaries()) != 1 {
		t.Fatalf("expected one summary")
	}
}

func TestDefaultCompressionThresholdIsSeventyFivePercent(t *testing.T) {
	cfg := config.Default()
	if cfg.Agent.Context.CompressThreshold != 0.75 {
		t.Fatalf("expected default compression threshold 0.75, got %v", cfg.Agent.Context.CompressThreshold)
	}
}

func TestSessionCompactArchivePreservesCompactedMessages(t *testing.T) {
	// Given
	messages := []types.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "first request"},
		{Role: "assistant", Content: "first response"},
		{Role: "user", Content: "latest request"},
		{Role: "assistant", Content: "latest response"},
	}

	// When
	result, ok := contextmgr.CompactOldMessages("archive-session", messages, 2)

	// Then
	if !ok {
		t.Fatalf("expected compact result")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected summary plus 2 kept messages, got %d", len(result.Messages))
	}
	if len(result.Archive.Messages) != 3 {
		t.Fatalf("expected 3 archived messages, got %d", len(result.Archive.Messages))
	}
	if result.Archive.Messages[0].Content != "system prompt" {
		t.Fatalf("expected archive to preserve first compacted message, got %+v", result.Archive.Messages[0])
	}
	if result.Summary.ToIndex != 2 {
		t.Fatalf("expected summary to cover compacted messages through index 2, got %d", result.Summary.ToIndex)
	}
}
