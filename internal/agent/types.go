package agent

import (
	"context"
	"encoding/json"
)

type Model string

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
)

type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type MessageRequest struct {
	Model     Model
	MaxTokens int64
	System    []SystemBlock
	Messages  []MessageParam
	Tools     []ToolDefinition
}

type SystemBlock struct {
	Text           string
	CacheEphemeral bool
}

type MessageParam struct {
	Role    Role                `json:"role"`
	Content []ContentBlockParam `json:"content"`
}

type ContentBlockParam struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type Message struct {
	ID         string         `json:"id"`
	Role       Role           `json:"role"`
	Model      Model          `json:"model"`
	StopReason StopReason     `json:"stop_reason"`
	Content    []ContentBlock `json:"content"`
	Usage      Usage          `json:"usage"`
}

type ContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema ToolInputSchema
}

type ToolInputSchema struct {
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}

// LLM 是 Whisperer 内部使用的 provider-neutral 子集。
type LLM interface {
	NewMessage(ctx context.Context, params MessageRequest) (*Message, error)
}

func NewTextBlock(text string) ContentBlockParam {
	return ContentBlockParam{Type: "text", Text: text}
}

func NewToolResultBlock(toolUseID string, content string, isError bool) ContentBlockParam {
	return ContentBlockParam{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		Text:      content,
		IsError:   isError,
	}
}

func NewUserMessage(blocks ...ContentBlockParam) MessageParam {
	return MessageParam{Role: RoleUser, Content: blocks}
}

func NewAssistantMessage(blocks ...ContentBlockParam) MessageParam {
	return MessageParam{Role: RoleAssistant, Content: blocks}
}

func (m Message) ToParam() MessageParam {
	out := MessageParam{Role: RoleAssistant, Content: make([]ContentBlockParam, 0, len(m.Content))}
	for _, block := range m.Content {
		switch block.Type {
		case "text":
			out.Content = append(out.Content, NewTextBlock(block.Text))
		case "tool_use":
			out.Content = append(out.Content, ContentBlockParam{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: append(json.RawMessage(nil), block.Input...),
			})
		}
	}
	return out
}

func (m Message) Text() string {
	out := ""
	for _, block := range m.Content {
		if block.Type != "text" {
			continue
		}
		if out != "" {
			out += "\n"
		}
		out += block.Text
	}
	return out
}
