package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
)

type memoryRuntimeConfig struct {
	Kind       string
	Model      string
	BaseURL    string
	Key        string
	KeyEnv     string
	PersistDir string
}

func buildMemory(cfg memoryRuntimeConfig) (*memory.Memory, string, error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.Kind))
	if kind == "" {
		kind = "openai"
	}
	switch kind {
	case "off", "none", "disabled":
		return nil, "off", nil
	case "fake":
		mem, err := memory.New(cfg.PersistDir, memory.NewFakeEmbedder(0))
		return mem, "fake", err
	case "openai", "openai_compat", "openai-compatible", "compat":
		keyEnv := strings.TrimSpace(cfg.KeyEnv)
		if keyEnv == "" {
			keyEnv = "OPENAI_API_KEY"
		}
		key := os.Getenv(keyEnv)
		if cfg.Key != "" {
			key = cfg.Key
		}
		emb, err := memory.NewOpenAIEmbedder(key, cfg.BaseURL, cfg.Model)
		if err != nil {
			return nil, "", fmt.Errorf("embedder %s (%s): %w", kind, keyEnv, err)
		}
		mem, err := memory.New(cfg.PersistDir, emb)
		return mem, "openai", err
	default:
		return nil, "", fmt.Errorf("unknown embedder %q (use openai, fake, or off)", cfg.Kind)
	}
}

func buildJudge(enabled bool, llm agent.LLM, model agent.Model) sla.Judge {
	if !enabled {
		return nil
	}
	return sla.NewHaikuJudge(llm, model)
}
