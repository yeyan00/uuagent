package contextmgr

import (
	"fmt"
	"strings"
	"time"

	"github.com/yeyan00/uuagent/internal/types"
)

// Summary records a context compression event.
type Summary struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	FromIndex     int    `json:"from_index"`
	ToIndex       int    `json:"to_index"`
	Summary       string `json:"summary"`
	TokenBefore   int    `json:"token_before"`
	TokenAfter    int    `json:"token_after"`
	CreatedAt     int64  `json:"created_at"`
	CompressionBy string `json:"compression_by"`
}

// CompactResult contains active context plus archival data from compaction.
type CompactResult struct {
	Messages         []types.Message `json:"messages"`
	ArchivedMessages []types.Message `json:"archived_messages"`
	Summary          Summary         `json:"summary"`
	Compacted        bool            `json:"compacted"`
}

// EstimateTokens is a cheap deterministic approximation for P0.
func EstimateTokens(messages []types.Message) int {
	chars := 0
	for _, msg := range messages {
		chars += len([]rune(msg.Role)) + len([]rune(types.TextOf(msg.Content)))
	}
	if chars == 0 {
		return 0
	}
	return chars/4 + 1
}

// ShouldCompress returns true when estimated tokens exceed maxTokens*threshold.
func ShouldCompress(messages []types.Message, maxTokens int, threshold float64) bool {
	if maxTokens <= 0 || threshold <= 0 {
		return false
	}
	return float64(EstimateTokens(messages)) >= float64(maxTokens)*threshold
}

// CompressOldMessages summarizes older messages and keeps the latest keepLast
// messages verbatim. P0 uses deterministic local summarization; later versions
// can use a model-based compressor.
func CompressOldMessages(sessionID string, messages []types.Message, keepLast int) ([]types.Message, Summary, bool) {
	result := CompactOldMessages(sessionID, messages, keepLast)
	return result.Messages, result.Summary, result.Compacted
}

// CompactOldMessages summarizes older messages, keeps recent messages active,
// and returns the compacted messages as an archive.
func CompactOldMessages(sessionID string, messages []types.Message, keepLast int) CompactResult {
	if keepLast < 1 {
		keepLast = 1
	}
	if len(messages) <= keepLast+1 {
		return CompactResult{Messages: messages}
	}
	cut := len(messages) - keepLast
	old := append([]types.Message(nil), messages[:cut]...)
	kept := append([]types.Message(nil), messages[cut:]...)
	before := EstimateTokens(messages)

	counts := map[string]int{}
	var samples []string
	for _, msg := range old {
		counts[msg.Role]++
		content := strings.TrimSpace(types.TextOf(msg.Content))
		if content == "" {
			continue
		}
		if len([]rune(content)) > 80 {
			content = string([]rune(content)[:80]) + "..."
		}
		if len(samples) < 3 {
			samples = append(samples, fmt.Sprintf("%s: %s", msg.Role, content))
		}
	}
	summaryText := fmt.Sprintf("Previous conversation summary: compressed %d older messages", len(old))
	if len(counts) > 0 {
		summaryText += fmt.Sprintf(" (user=%d assistant=%d system=%d)", counts["user"], counts["assistant"], counts["system"])
	}
	if len(samples) > 0 {
		summaryText += ". Samples: " + strings.Join(samples, " | ")
	}
	compressed := append([]types.Message{{Role: "system", Content: summaryText}}, kept...)
	after := EstimateTokens(compressed)
	summary := Summary{
		ID:            fmt.Sprintf("sum-%d", time.Now().UnixNano()),
		SessionID:     sessionID,
		FromIndex:     0,
		ToIndex:       cut - 1,
		Summary:       summaryText,
		TokenBefore:   before,
		TokenAfter:    after,
		CreatedAt:     time.Now().Unix(),
		CompressionBy: "local-p0",
	}
	return CompactResult{Messages: compressed, ArchivedMessages: old, Summary: summary, Compacted: true}
}
