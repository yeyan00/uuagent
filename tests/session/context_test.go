package session_test

import (
	"testing"

	"github.com/uuagent/uuagent/internal/session"
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
