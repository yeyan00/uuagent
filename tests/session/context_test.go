package session_test

import (
	"testing"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/session"
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
	store := session.NewStoreAt(t.TempDir())
	s := store.GetOrCreate("archive-session")
	for i := 0; i < 12; i++ {
		s.Append("user", "this older message should contribute to archive token pressure")
	}

	// When
	archive, ok := s.CompactArchive(80, 0.5, 4)

	// Then
	if !ok {
		t.Fatalf("expected compact archive")
	}
	if len(archive.Messages) != 8 {
		t.Fatalf("expected 8 archived messages, got %d", len(archive.Messages))
	}
	if archive.FromIndex != 0 || archive.ToIndex != 7 {
		t.Fatalf("expected archive indexes 0..7, got %d..%d", archive.FromIndex, archive.ToIndex)
	}
	if archive.Summary.ID == "" || archive.Summary.SessionID != "archive-session" {
		t.Fatalf("unexpected archive summary: %+v", archive.Summary)
	}
	if archive.TokenAfter >= archive.TokenBefore {
		t.Fatalf("expected token reduction: before=%d after=%d", archive.TokenBefore, archive.TokenAfter)
	}
	if len(s.ListSummaries()) != 1 {
		t.Fatalf("expected one summary")
	}
	if len(s.ListArchives()) != 1 {
		t.Fatalf("expected one archive")
	}
	snap := s.Snapshot()
	if len(snap.Messages) != 5 {
		t.Fatalf("expected active context summary plus 4 kept messages, got %d", len(snap.Messages))
	}
}
