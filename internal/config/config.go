// Package config 加载 Whisperer 的运行配置：默认值 → TOML → 环境变量 → CLI flag
// 的四层覆盖。
//
// 设计取舍：
//   - 不引入 viper / koanf。stdlib `flag` + BurntSushi/toml 已够；这两个库会
//     带 50+ 间接依赖，对一个 TUI 工具不值。
//   - **API key 一律不进 TOML**——只走环境变量或 --api-key flag。TOML 文件经常
//     被 git/同步工具不小心带走；强约束让用户不会因为方便误把 key 提交。
//   - Duration 字段在 TOML 用字符串（"120s"），Load 时解析成 time.Duration，
//     避免 BurntSushi/toml 的 UnmarshalText boilerplate。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config 是 Whisperer 的运行时配置。所有字段都是可空——只有用户显式提供（toml /
// env / flag）才会非零。空值在 cmd/whisperer/main.go 的最后一段决定其默认行为。
type Config struct {
	// LLM provider 选择
	Provider    string `toml:"provider"`
	Model       string `toml:"model"`
	ModelHelper string `toml:"model_helper"`

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
	LLMTimeoutStr string        `toml:"llm_timeout"`     // "120s"
	LLMRetriesRaw *int          `toml:"llm_max_retries"` // 用指针区分"未设"与"显式 0"

	// Embedder section
	Embedder EmbedderSection `toml:"embedder"`
}

// EmbedderSection 是 [embedder] table。APIKey 不接受 TOML 输入，只读 env。
type EmbedderSection struct {
	Provider string `toml:"provider"`
	Model    string `toml:"model"`
	BaseURL  string `toml:"base_url"`
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

// Load 从 path 读 TOML；path == "" 时尝试 DefaultPath。
//
// 文件不存在不算错（首次运行无配置是正常情况），返回零值 Config。文件存在但
// 解析失败 → 返回错误，绝不静默吞错让用户感到"配置没生效"。
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	cfg := &Config{}
	if path == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if _, err := toml.Decode(string(raw), cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if err := cfg.parseDurations(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	if cfg.LLMRetriesRaw != nil {
		cfg.LLMMaxRetries = *cfg.LLMRetriesRaw
	}
	return cfg, nil
}

// parseDurations 把 TOML 里的 "120s" 字符串解析成 time.Duration。
func (c *Config) parseDurations() error {
	if c.LLMTimeoutStr != "" {
		d, err := time.ParseDuration(c.LLMTimeoutStr)
		if err != nil {
			return fmt.Errorf("invalid llm_timeout %q: %w", c.LLMTimeoutStr, err)
		}
		c.LLMTimeout = d
	}
	return nil
}

// EnvOverlay 把环境变量覆盖到 Config 上（在 TOML 之后、CLI flag 之前）。
//
// 支持的变量：
//   - WHISPERER_PROVIDER / WHISPERER_MODEL / WHISPERER_MODEL_HELPER
//   - WHISPERER_SCENARIO / WHISPERER_SAVE / WHISPERER_VARIANT
//   - WHISPERER_DB / WHISPERER_MEM_DIR / WHISPERER_TRACE_DIR / WHISPERER_META
//   - WHISPERER_LOG_FORMAT / WHISPERER_LOG_LEVEL
//   - WHISPERER_EMBEDDER / WHISPERER_EMBEDDER_MODEL / WHISPERER_EMBEDDER_BASE_URL
//
// **API key 永远只通过 ANTHROPIC_API_KEY / OPENROUTER_API_KEY / EMBEDDER_API_KEY
// 这几个独立变量传，不在这里覆盖**——保持密钥与一般配置的物理隔离。
func (c *Config) EnvOverlay() {
	envSet(&c.Provider, "WHISPERER_PROVIDER")
	envSet(&c.Model, "WHISPERER_MODEL")
	envSet(&c.ModelHelper, "WHISPERER_MODEL_HELPER")
	envSet(&c.Scenario, "WHISPERER_SCENARIO")
	envSet(&c.Save, "WHISPERER_SAVE")
	envSet(&c.Variant, "WHISPERER_VARIANT")
	envSet(&c.DBPath, "WHISPERER_DB")
	envSet(&c.MemDir, "WHISPERER_MEM_DIR")
	envSet(&c.TraceDir, "WHISPERER_TRACE_DIR")
	envSet(&c.MetaPath, "WHISPERER_META")
	envSet(&c.LogFormat, "WHISPERER_LOG_FORMAT")
	envSet(&c.LogLevel, "WHISPERER_LOG_LEVEL")
	envSet(&c.Lang, "WHISPERER_LANG")
	envSet(&c.Embedder.Provider, "WHISPERER_EMBEDDER")
	envSet(&c.Embedder.Model, "WHISPERER_EMBEDDER_MODEL")
	envSet(&c.Embedder.BaseURL, "WHISPERER_EMBEDDER_BASE_URL")
}

func envSet(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}
