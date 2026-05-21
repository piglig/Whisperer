package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
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
	OpenAIModelGM     anthropic.Model = "gpt-5.1"
	OpenAIModelHelper anthropic.Model = "gpt-5.1-mini"
	GrokModelGM       anthropic.Model = "grok-4.3"
	GrokModelHelper   anthropic.Model = "grok-4.3-fast"
	GeminiModelGM     anthropic.Model = "gemini-2.5-pro"
	GeminiModelHelper anthropic.Model = "gemini-2.5-flash"
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

func (c *OpenAIChat) NewMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
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

func toOpenAIChatParams(params anthropic.MessageNewParams) (openai.ChatCompletionNewParams, error) {
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

func messageParamToOpenAI(msg anthropic.MessageParam) []openai.ChatCompletionMessageParamUnion {
	var textParts []string
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	var toolMessages []openai.ChatCompletionMessageParamUnion

	for _, block := range msg.Content {
		switch {
		case block.OfText != nil:
			textParts = append(textParts, block.OfText.Text)
		case block.OfToolUse != nil:
			args := "{}"
			if block.OfToolUse.Input != nil {
				if b, err := json.Marshal(block.OfToolUse.Input); err == nil && len(b) > 0 {
					args = string(b)
				}
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: block.OfToolUse.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      block.OfToolUse.Name,
						Arguments: args,
					},
				},
			})
		case block.OfToolResult != nil:
			toolMessages = append(toolMessages, openai.ToolMessage(
				toolResultText(block.OfToolResult),
				block.OfToolResult.ToolUseID,
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

func toolParamToOpenAI(tool anthropic.ToolUnionParam) (openai.ChatCompletionToolUnionParam, bool, error) {
	if tool.OfTool == nil {
		return openai.ChatCompletionToolUnionParam{}, false, nil
	}
	t := tool.OfTool
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
	desc := ""
	if t.Description.Valid() {
		desc = t.Description.Value
	}
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        t.Name,
		Description: openai.String(desc),
		Parameters:  shared.FunctionParameters(schema),
	}), true, nil
}

func toolResultText(block *anthropic.ToolResultBlockParam) string {
	if block == nil {
		return ""
	}
	var parts []string
	for _, item := range block.Content {
		if item.OfText != nil {
			parts = append(parts, item.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func openAIChatCompletionToAnthropic(resp *openai.ChatCompletion, fallbackModel string) (*anthropic.Message, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, errors.New("openai chat: response has no choices")
	}
	choice := resp.Choices[0]
	content := make([]map[string]any, 0, 1+len(choice.Message.ToolCalls))
	if strings.TrimSpace(choice.Message.Content) != "" {
		content = append(content, map[string]any{
			"type": "text",
			"text": choice.Message.Content,
		})
	}
	for i, tc := range choice.Message.ToolCalls {
		fn := tc.Function
		input := map[string]any{}
		if strings.TrimSpace(fn.Arguments) != "" {
			if err := json.Unmarshal([]byte(fn.Arguments), &input); err != nil {
				input = map[string]any{"_raw": fn.Arguments}
			}
		}
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  fn.Name,
			"input": input,
		})
	}
	if len(content) == 0 {
		content = append(content, map[string]any{"type": "text", "text": ""})
	}

	stopReason := "end_turn"
	if len(choice.Message.ToolCalls) > 0 || choice.FinishReason == "tool_calls" {
		stopReason = "tool_use"
	} else if choice.FinishReason == "length" {
		stopReason = "max_tokens"
	}

	msgJSON, err := json.Marshal(map[string]any{
		"id":          firstNonEmpty(resp.ID, "msg_openai_chat"),
		"type":        "message",
		"role":        "assistant",
		"model":       firstNonEmpty(resp.Model, fallbackModel),
		"content":     content,
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
		},
	})
	if err != nil {
		return nil, err
	}
	var msg anthropic.Message
	if err := json.Unmarshal(msgJSON, &msg); err != nil {
		return nil, fmt.Errorf("openai chat: convert response: %w", err)
	}
	return &msg, nil
}

func joinTextBlocks(blocks []anthropic.TextBlockParam) string {
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
