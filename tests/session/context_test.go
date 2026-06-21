package session_test

import (
	"fmt"
	"strings"
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

func TestSessionCompactArchiveRestoreReturnsOriginalMessagesAndRemovesSummary(t *testing.T) {
	// Given: a session with messages that gets compacted
	store := session.NewStoreAt(t.TempDir())
	s := store.GetOrCreate("restore-session")
	for i := 0; i < 12; i++ {
		s.Append("user", fmt.Sprintf("original message %d", i))
	}
	// Add tail messages that should be kept after compact
	s.Append("user", "tail message 1")
	s.Append("assistant", "tail response 1")

	archive, ok := s.CompactArchive(80, 0.5, 2)
	if !ok {
		t.Fatalf("expected compact archive")
	}

	// Verify compact worked
	snap := s.Snapshot()
	if len(snap.Messages) != 3 { // summary + 2 tail messages
		t.Fatalf("expected 3 messages after compact (summary + 2 tail), got %d", len(snap.Messages))
	}
	if len(snap.Summaries) != 1 {
		t.Fatalf("expected 1 summary after compact, got %d", len(snap.Summaries))
	}
	if len(snap.Archives) != 1 {
		t.Fatalf("expected 1 archive after compact, got %d", len(snap.Archives))
	}

	// When: restore the archive
	restoredMessages, err := s.RestoreCompactArchive(archive.ID)

	// Then: restore succeeded
	if err != nil {
		t.Fatalf("expected restore to succeed, got error: %v", err)
	}

	// Verify restored messages = archive messages + tail messages
	expectedLen := len(archive.Messages) + 2 // 2 tail messages
	if len(restoredMessages) != expectedLen {
		t.Fatalf("expected %d restored messages (archive + tail), got %d", expectedLen, len(restoredMessages))
	}

	// Verify first restored message matches first archived message
	if restoredMessages[0].Content != "original message 0" {
		t.Fatalf("expected first restored message to be 'original message 0', got %s", restoredMessages[0].Content)
	}

	// Verify tail messages are preserved
	if restoredMessages[len(restoredMessages)-2].Content != "tail message 1" {
		t.Fatalf("expected second-to-last message to be 'tail message 1', got %s", restoredMessages[len(restoredMessages)-2].Content)
	}
	if restoredMessages[len(restoredMessages)-1].Content != "tail response 1" {
		t.Fatalf("expected last message to be 'tail response 1', got %s", restoredMessages[len(restoredMessages)-1].Content)
	}

	// Verify the summary system message was removed
	snap = s.Snapshot()
	for _, msg := range snap.Messages {
		if msg.Role == "system" && msg.Content == archive.Summary.Summary {
			t.Fatalf("expected summary system message to be removed")
		}
	}

	// Verify the summary was removed from summaries list
	if len(s.ListSummaries()) != 0 {
		t.Fatalf("expected 0 summaries after restore, got %d", len(s.ListSummaries()))
	}

	// Verify the archive was removed from archives list
	if len(s.ListArchives()) != 0 {
		t.Fatalf("expected 0 archives after restore, got %d", len(s.ListArchives()))
	}
}

func TestSessionRestoreCompactArchiveReturnsConflictOnMismatch(t *testing.T) {
	// Given: a session with messages that gets compacted
	store := session.NewStoreAt(t.TempDir())
	s := store.GetOrCreate("mismatch-session")
	for i := 0; i < 12; i++ {
		s.Append("user", fmt.Sprintf("original message %d", i))
	}

	archive, ok := s.CompactArchive(80, 0.5, 2)
	if !ok {
		t.Fatalf("expected compact archive")
	}

	s.Append("user", "new message after compact")

	_, err := s.RestoreCompactArchive(archive.ID)

	if err == nil {
		t.Fatalf("expected conflict error when summary mismatch")
	}
	if !strings.Contains(err.Error(), "conflict") && !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected error to mention conflict or mismatch, got: %v", err)
	}
}

func TestSessionRestoreCompactArchiveReturnsNotFoundForMissingArchive(t *testing.T) {
	// Given: a session
	store := session.NewStoreAt(t.TempDir())
	s := store.GetOrCreate("notfound-session")
	s.Append("user", "some message")

	// When: try to restore non-existent archive
	_, err := s.RestoreCompactArchive("non-existent-archive-id")

	// Then: should return not found error
	if err == nil {
		t.Fatalf("expected not found error for missing archive")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected error to mention not found, got: %v", err)
	}
}
