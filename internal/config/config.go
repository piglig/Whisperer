// Package config 加载 Whisperer 的运行配置：默认值 → TOML → 环境变量 → CLI flag
// 的四层覆盖。
//
// API key 一律不进 TOML——只走环境变量或 --api-key flag。TOML 文件经常被
// git/同步工具不小心带走；强约束让用户不会因为方便误把 key 提交。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config 是 Whisperer 的运行时配置。Load 返回默认值、TOML 与环境变量合并后的结果；
// CLI flags 在 cmd/whisperer/main.go 中继续作为最后一层覆盖。
type Config struct {
	// LLM provider 选择
	Provider    string `toml:"provider"`
	Model       string `toml:"model"`
	ModelHelper string `toml:"model_helper"`

	// 语义记忆 embedding。API key 不进 TOML；EmbedderAPIKeyEnv 指向环境变量名。
	Embedder          string `toml:"embedder"`
	EmbedderModel     string `toml:"embedder_model"`
	EmbedderBaseURL   string `toml:"embedder_base_url"`
	EmbedderAPIKeyEnv string `toml:"embedder_api_key_env"`

	// LLM-as-judge，默认关闭；开启后使用 helper LLM 检查 NPC 一致性、知识投射等。
	EnableJudge bool   `toml:"enable_judge"`
	JudgeModel  string `toml:"judge_model"`

	// 剧本与存档
	Scenario string `toml:"scenario"`
	Save     string `toml:"save"`
	Variant  string `toml:"variant"`
	Seed     int64  `toml:"seed"`
	MetaPath string `toml:"meta_path"`

	// 持久化路径
	DBPath   string `toml:"db_path"`
	MemDir   string `toml:"memory_dir"`
	TraceDir string `toml:"trace_dir"`

	// 日志
	LogFormat string `toml:"log_format"`
	LogLevel  string `toml:"log_level"`

	// UI 语言 tag（BCP-47，例如 "zh-CN" / "en"）。空 → 运行时按 $LANG 自动检测。
	Lang string `toml:"lang"`

	// LLM 网络层
	LLMMaxRetries int           `toml:"-"`
	LLMTimeout    time.Duration `toml:"-"`
	LLMTimeoutStr string        `toml:"llm_timeout"`     // "120s"，由 Load 解析到 LLMTimeout
	LLMRetriesRaw *int          `toml:"llm_max_retries"` // 用指针区分"未设"与"显式 0"
}

// Defaults 返回内置默认值。它们也会作为 koanf 的第一层 provider 参与合并。
func Defaults() Config {
	return Config{
		Provider:          "",
		Embedder:          "openai",
		EmbedderModel:     "text-embedding-3-small",
		EmbedderBaseURL:   "https://api.openai.com/v1",
		EmbedderAPIKeyEnv: "OPENAI_API_KEY",
		Scenario:          "fog_harbor",
		DBPath:            "whisperer.db",
		MemDir:            "mem",
		MetaPath:          "runs/meta.json",
		LogFormat:         "text",
		LogLevel:          "info",
		LLMTimeout:        120 * time.Second,
		TraceDir:          "runs",
		LLMTimeoutStr:     "120s",
	}
}

// DefaultPath 返回平台规范的默认配置文件位置。
//   - $XDG_CONFIG_HOME/whisperer/config.toml（如果设了 XDG_CONFIG_HOME）
//   - $HOME/.config/whisperer/config.toml（POSIX 默认）
//   - %APPDATA%/whisperer/config.toml（Windows）
//
// 找不到任何 home 时返回空字符串。
func DefaultPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "whisperer", "config.toml")
	}
	if app := os.Getenv("APPDATA"); app != "" {
		return filepath.Join(app, "whisperer", "config.toml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "whisperer", "config.toml")
	}
	return ""
}

// Load 从 path 读 TOML 并叠加 WHISPERER_* 环境变量；path == "" 时尝试 DefaultPath。
//
// 文件不存在不算错（首次运行无配置是正常情况），返回默认 Config。文件存在但
// 解析失败 → 返回错误，绝不静默吞错让用户感到"配置没生效"。
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	k := koanf.New(".")
	if err := k.Load(confmap.Provider(defaultMap(), "."), nil); err != nil {
		return nil, fmt.Errorf("config: defaults: %w", err)
	}
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("config: read %s: %w", path, err)
			}
		} else if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}
	if err := k.Load(env.ProviderWithValue("WHISPERER_", ".", envKey), nil); err != nil {
		return nil, fmt.Errorf("config: env: %w", err)
	}

	cfg := Defaults()
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "toml"}); err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	if err := cfg.parseScalars(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	if cfg.LLMRetriesRaw != nil {
		cfg.LLMMaxRetries = *cfg.LLMRetriesRaw
	}
	return &cfg, nil
}

func defaultMap() map[string]interface{} {
	d := Defaults()
	return map[string]interface{}{
		"provider":             d.Provider,
		"embedder":             d.Embedder,
		"embedder_model":       d.EmbedderModel,
		"embedder_base_url":    d.EmbedderBaseURL,
		"embedder_api_key_env": d.EmbedderAPIKeyEnv,
		"enable_judge":         d.EnableJudge,
		"judge_model":          d.JudgeModel,
		"scenario":             d.Scenario,
		"db_path":              d.DBPath,
		"memory_dir":           d.MemDir,
		"meta_path":            d.MetaPath,
		"log_format":           d.LogFormat,
		"log_level":            d.LogLevel,
		"llm_timeout":          d.LLMTimeoutStr,
		"llm_max_retries":      3,
		"trace_dir":            d.TraceDir,
	}
}

func envKey(key, value string) (string, interface{}) {
	switch strings.TrimPrefix(key, "WHISPERER_") {
	case "PROVIDER":
		return "provider", value
	case "MODEL":
		return "model", value
	case "MODEL_HELPER":
		return "model_helper", value
	case "EMBEDDER":
		return "embedder", value
	case "EMBEDDER_MODEL":
		return "embedder_model", value
	case "EMBEDDER_BASE_URL":
		return "embedder_base_url", value
	case "EMBEDDER_API_KEY_ENV":
		return "embedder_api_key_env", value
	case "ENABLE_JUDGE":
		return "enable_judge", value
	case "JUDGE_MODEL":
		return "judge_model", value
	case "SCENARIO":
		return "scenario", value
	case "SAVE":
		return "save", value
	case "VARIANT":
		return "variant", value
	case "DB":
		return "db_path", value
	case "MEM_DIR":
		return "memory_dir", value
	case "TRACE_DIR":
		return "trace_dir", value
	case "META":
		return "meta_path", value
	case "LOG_FORMAT":
		return "log_format", value
	case "LOG_LEVEL":
		return "log_level", value
	case "LANG":
		return "lang", value
	case "LLM_TIMEOUT":
		return "llm_timeout", value
	case "LLM_MAX_RETRIES":
		return "llm_max_retries", value
	default:
		return "", nil
	}
}

// parseScalars 把 TOML/env 里的字符串标量解析成强类型字段。
func (c *Config) parseScalars() error {
	if c.LLMTimeoutStr != "" {
		d, err := time.ParseDuration(c.LLMTimeoutStr)
		if err != nil {
			return fmt.Errorf("invalid llm_timeout %q: %w", c.LLMTimeoutStr, err)
		}
		c.LLMTimeout = d
	}
	return nil
}
