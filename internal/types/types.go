package types

import "encoding/json"

// Event is an Agent event streamed over SSE.
type Event struct {
	Type     string `json:"type"` // route, status, content, tool_start, tool_result, error, done
	RunID    string `json:"run_id,omitempty"`
	Model    string `json:"model,omitempty"`
	Tier     string `json:"tier,omitempty"`
	Text     string `json:"text,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	ToolID   string `json:"tool_id,omitempty"`
	Args     string `json:"args,omitempty"`
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
	ID       string       `json:"id"`
	Type     string       `json:"type,omitempty"`
	Function ToolFunction `json:"function,omitempty"`
	Name     string       `json:"name,omitempty"`
	Args     string       `json:"args,omitempty"`
}

// ToolFunction is the OpenAI-compatible function-call payload persisted with
// assistant tool calls. Name and Args are kept for internal compatibility.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// MarshalJSON sends assistant tool calls in the OpenAI-compatible shape. The
// legacy Name/Args fields remain available in Go for older code paths, but are
// intentionally not emitted to providers because some reject non-standard
// tool-call payloads.
func (tc ToolCall) MarshalJSON() ([]byte, error) {
	name := tc.Function.Name
	if name == "" {
		name = tc.Name
	}
	args := tc.Function.Arguments
	if args == "" {
		args = tc.Args
	}
	typ := tc.Type
	if typ == "" {
		typ = "function"
	}
	return json.Marshal(struct {
		ID       string       `json:"id"`
		Type     string       `json:"type"`
		Function ToolFunction `json:"function"`
	}{ID: tc.ID, Type: typ, Function: ToolFunction{Name: name, Arguments: args}})
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
