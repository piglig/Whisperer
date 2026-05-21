package agent

import (
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	ProviderAnthropic  = "anthropic"
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
	ProviderGrok       = "grok"
	ProviderGemini     = "gemini"
)

type ProviderSpec struct {
	Name        string
	EnvKey      string
	ModelGM     anthropic.Model
	ModelHelper anthropic.Model
}

var providerSpecs = map[string]ProviderSpec{
	ProviderAnthropic: {
		Name:        ProviderAnthropic,
		EnvKey:      "ANTHROPIC_API_KEY",
		ModelGM:     ModelGM,
		ModelHelper: ModelHelper,
	},
	ProviderOpenRouter: {
		Name:        ProviderOpenRouter,
		EnvKey:      "OPENROUTER_API_KEY",
		ModelGM:     OpenRouterModelGM,
		ModelHelper: OpenRouterModelHelper,
	},
	ProviderOpenAI: {
		Name:        ProviderOpenAI,
		EnvKey:      "OPENAI_API_KEY",
		ModelGM:     OpenAIModelGM,
		ModelHelper: OpenAIModelHelper,
	},
	ProviderGrok: {
		Name:        ProviderGrok,
		EnvKey:      "XAI_API_KEY",
		ModelGM:     GrokModelGM,
		ModelHelper: GrokModelHelper,
	},
	ProviderGemini: {
		Name:        ProviderGemini,
		EnvKey:      "GEMINI_API_KEY",
		ModelGM:     GeminiModelGM,
		ModelHelper: GeminiModelHelper,
	},
}

func NormalizeProvider(p string) string {
	if spec, ok := ParseProvider(p); ok {
		return spec.Name
	}
	return ProviderAnthropic
}

func ParseProvider(p string) (ProviderSpec, bool) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", ProviderAnthropic:
		return providerSpecs[ProviderAnthropic], true
	case ProviderOpenRouter, "or":
		return providerSpecs[ProviderOpenRouter], true
	case ProviderOpenAI, "gpt":
		return providerSpecs[ProviderOpenAI], true
	case ProviderGrok, "xai", "x.ai":
		return providerSpecs[ProviderGrok], true
	case ProviderGemini, "google":
		return providerSpecs[ProviderGemini], true
	default:
		return ProviderSpec{}, false
	}
}

func ProviderInfo(provider string) ProviderSpec {
	return providerSpecs[NormalizeProvider(provider)]
}

func AutoProvider() string {
	var found string
	for _, provider := range []string{
		ProviderAnthropic,
		ProviderOpenRouter,
		ProviderOpenAI,
		ProviderGrok,
		ProviderGemini,
	} {
		spec := providerSpecs[provider]
		if os.Getenv(spec.EnvKey) == "" {
			continue
		}
		if found != "" {
			return ProviderAnthropic
		}
		found = provider
	}
	if found != "" {
		return found
	}
	return ProviderAnthropic
}

func BuildLLM(provider, key string, maxRetries int, timeout time.Duration) (LLM, anthropic.Model, anthropic.Model) {
	spec := ProviderInfo(provider)
	switch spec.Name {
	case ProviderOpenRouter:
		return NewAnthropic(ClientConfig{
			AuthToken:      key,
			BaseURL:        OpenRouterBaseURL,
			MaxRetries:     maxRetries,
			RequestTimeout: timeout,
		}), spec.ModelGM, spec.ModelHelper
	case ProviderOpenAI:
		return NewOpenAIChat(ClientConfig{
			APIKey:         key,
			BaseURL:        OpenAIBaseURL,
			MaxRetries:     maxRetries,
			RequestTimeout: timeout,
		}), spec.ModelGM, spec.ModelHelper
	case ProviderGrok:
		return NewOpenAIChat(ClientConfig{
			APIKey:         key,
			BaseURL:        GrokBaseURL,
			MaxRetries:     maxRetries,
			RequestTimeout: timeout,
		}), spec.ModelGM, spec.ModelHelper
	case ProviderGemini:
		return NewOpenAIChat(ClientConfig{
			APIKey:         key,
			BaseURL:        GeminiBaseURL,
			MaxRetries:     maxRetries,
			RequestTimeout: timeout,
		}), spec.ModelGM, spec.ModelHelper
	default:
		return NewAnthropic(ClientConfig{
			APIKey:         key,
			BaseURL:        AnthropicBaseURL,
			MaxRetries:     maxRetries,
			RequestTimeout: timeout,
		}), spec.ModelGM, spec.ModelHelper
	}
}
