package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

const (
	OpenAIBaseURL = "https://api.openai.com/v1"
	GrokBaseURL   = "https://api.x.ai/v1"
	GeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
)

const (
	OpenAIModelGM     Model = "gpt-5.1"
	OpenAIModelHelper Model = "gpt-5.1-mini"
	GrokModelGM       Model = "grok-4.3"
	GrokModelHelper   Model = "grok-4.3-fast"
	GeminiModelGM     Model = "gemini-2.5-pro"
	GeminiModelHelper Model = "gemini-2.5-flash"
)

// OpenAIChat adapts the official OpenAI Go SDK to Whisperer's existing LLM
// interface. Grok and Gemini both expose OpenAI-compatible Chat Completions
// endpoints, so they share this transport with provider-specific base URLs.
type OpenAIChat struct {
	client openai.Client
}

func NewOpenAIChat(cfg ClientConfig) *OpenAIChat {
	var opts []option.RequestOption
	key := firstNonEmpty(cfg.AuthToken, cfg.APIKey)
	if key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, option.WithMaxRetries(cfg.MaxRetries))
	}
	if cfg.RequestTimeout > 0 {
		opts = append(opts, option.WithRequestTimeout(cfg.RequestTimeout))
	}
	return &OpenAIChat{client: openai.NewClient(opts...)}
}

func (c *OpenAIChat) NewMessage(ctx context.Context, params MessageRequest) (*Message, error) {
	req, err := toOpenAIChatParams(params)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Chat.Completions.New(ctx, req)
	if err != nil {
		return nil, err
	}
	return openAIChatCompletionToAnthropic(resp, string(params.Model))
}

func toOpenAIChatParams(params MessageRequest) (openai.ChatCompletionNewParams, error) {
	req := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(params.Model),
	}
	if params.MaxTokens > 0 {
		req.MaxCompletionTokens = openai.Int(params.MaxTokens)
	}
	system := strings.TrimSpace(joinTextBlocks(params.System))
	if system != "" {
		req.Messages = append(req.Messages, openai.SystemMessage(system))
	}
	for _, msg := range params.Messages {
		req.Messages = append(req.Messages, messageParamToOpenAI(msg)...)
	}
	for _, tool := range params.Tools {
		converted, ok, err := toolParamToOpenAI(tool)
		if err != nil {
			return req, err
		}
		if ok {
			req.Tools = append(req.Tools, converted)
		}
	}
	if len(req.Tools) > 0 {
		req.ToolChoice.OfAuto = param.NewOpt("auto")
	}
	return req, nil
}

func messageParamToOpenAI(msg MessageParam) []openai.ChatCompletionMessageParamUnion {
	var textParts []string
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	var toolMessages []openai.ChatCompletionMessageParamUnion

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			args := "{}"
			if len(block.Input) > 0 {
				args = string(block.Input)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: block.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      block.Name,
						Arguments: args,
					},
				},
			})
		case "tool_result":
			toolMessages = append(toolMessages, openai.ToolMessage(
				block.Text,
				block.ToolUseID,
			))
		}
	}

	if len(toolMessages) > 0 && len(textParts) == 0 && len(toolCalls) == 0 {
		return toolMessages
	}

	role := string(msg.Role)
	if role == "assistant" {
		assistant := openai.ChatCompletionAssistantMessageParam{}
		text := strings.Join(textParts, "\n")
		if text != "" {
			assistant.Content.OfString = openai.String(text)
		}
		assistant.ToolCalls = toolCalls
		return append([]openai.ChatCompletionMessageParamUnion{{OfAssistant: &assistant}}, toolMessages...)
	}
	return append([]openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(strings.Join(textParts, "\n")),
	}, toolMessages...)
}

func toolParamToOpenAI(t ToolDefinition) (openai.ChatCompletionToolUnionParam, bool, error) {
	schema := map[string]any{}
	schemaBytes, err := json.Marshal(t.InputSchema)
	if err != nil {
		return openai.ChatCompletionToolUnionParam{}, false, fmt.Errorf("openai chat: marshal tool %q schema: %w", t.Name, err)
	}
	if len(schemaBytes) > 0 && string(schemaBytes) != "null" {
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			return openai.ChatCompletionToolUnionParam{}, false, fmt.Errorf("openai chat: unmarshal tool %q schema: %w", t.Name, err)
		}
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        t.Name,
		Description: openai.String(t.Description),
		Parameters:  shared.FunctionParameters(schema),
	}), true, nil
}

func openAIChatCompletionToAnthropic(resp *openai.ChatCompletion, fallbackModel string) (*Message, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, errors.New("openai chat: response has no choices")
	}
	choice := resp.Choices[0]
	content := make([]ContentBlock, 0, 1+len(choice.Message.ToolCalls))
	if strings.TrimSpace(choice.Message.Content) != "" {
		content = append(content, ContentBlock{Type: "text", Text: choice.Message.Content})
	}
	for i, tc := range choice.Message.ToolCalls {
		fn := tc.Function
		input := json.RawMessage([]byte("{}"))
		if strings.TrimSpace(fn.Arguments) != "" {
			if json.Valid([]byte(fn.Arguments)) {
				input = json.RawMessage(fn.Arguments)
			} else {
				input, _ = json.Marshal(map[string]any{"_raw": fn.Arguments})
			}
		}
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		content = append(content, ContentBlock{Type: "tool_use", ID: id, Name: fn.Name, Input: input})
	}
	if len(content) == 0 {
		content = append(content, ContentBlock{Type: "text", Text: ""})
	}

	stopReason := StopReasonEndTurn
	if len(choice.Message.ToolCalls) > 0 || choice.FinishReason == "tool_calls" {
		stopReason = StopReasonToolUse
	} else if choice.FinishReason == "length" {
		stopReason = StopReasonMaxTokens
	}

	return &Message{
		ID:         firstNonEmpty(resp.ID, "msg_openai_chat"),
		Role:       RoleAssistant,
		Model:      Model(firstNonEmpty(resp.Model, fallbackModel)),
		Content:    content,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func joinTextBlocks(blocks []SystemBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
