package types

// Event Agent 事件 (SSE 推送)
type Event struct {
	Type     string `json:"type"` // route, status, content, tool_start, tool_result, error, done
	Model    string `json:"model,omitempty"`
	Tier     string `json:"tier,omitempty"`
	Text     string `json:"text,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	ToolID   string `json:"tool_id,omitempty"`
}

// Message OpenAI-compatible chat message. Content may be a plain string or a
// multimodal []ContentPart array for vision-capable models.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
}

// ContentPart is an OpenAI-compatible multimodal content part.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL references an image by URL or data URL.
type ImageURL struct {
	URL string `json:"url"`
}

// ToolCall is the internal flattened tool-call representation.
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

func TextOf(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []ContentPart:
		out := ""
		for _, p := range v {
			if p.Type == "text" {
				out += p.Text
			}
		}
		return out
	case []any:
		out := ""
		for _, item := range v {
			if m, ok := item.(map[string]any); ok && m["type"] == "text" {
				if s, ok := m["text"].(string); ok {
					out += s
				}
			}
		}
		return out
	default:
		return ""
	}
}
