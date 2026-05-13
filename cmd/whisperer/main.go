// Whisperer CLI 入口。
//
// 用法（Anthropic 官方）:
//
//	export ANTHROPIC_API_KEY=sk-ant-...
//	whisperer --db save.db --memory ./mem --scenario fog_harbor
//
// 用法（OpenRouter）:
//
//	export OPENROUTER_API_KEY=sk-or-...
//	whisperer --provider openrouter
//
// 缺省加载内置 fog_harbor 剧本，自动创建 save 与一名占位调查员。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	wlog "github.com/zhuzhenwu/whisperer/internal/log"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
	"github.com/zhuzhenwu/whisperer/internal/tui"
)

const (
	providerAnthropic  = "anthropic"
	providerOpenRouter = "openrouter"
)

func main() {
	dbPath := flag.String("db", "whisperer.db", "SQLite save file path")
	memDir := flag.String("memory", "mem", "memory persistent dir; empty for in-memory")
	scenarioID := flag.String("scenario", "fog_harbor", "bundled scenario id")
	saveID := flag.String("save", "", "existing save id to load; empty creates a new save")
	provider := flag.String("provider", autoProvider(), "LLM provider: anthropic | openrouter")
	apiKey := flag.String("api-key", "", "API key (overrides env). Anthropic→ANTHROPIC_API_KEY, OpenRouter→OPENROUTER_API_KEY")
	modelOverride := flag.String("model", "", "override GM model id (full vendor/model on OpenRouter)")
	modelHelperOverride := flag.String("model-helper", "", "override Haiku/helper model id")
	smoke := flag.Bool("smoke", false, "smoke test mode: do not call any LLM, print rules samples")
	variantID := flag.String("variant", "", "force a specific variant id (default: weighted random)")
	seed := flag.Int64("seed", 0, "deterministic variant selection seed (0 = unix nano)")
	metaPath := flag.String("meta", "runs/meta.json", "cross-run meta file path; '-' to disable")
	logFormat := flag.String("log-format", "text", "log handler format: text | json")
	logLevel := flag.String("log-level", "info", "log level: debug | info | warn | error")
	embedderProvider := flag.String("embedder", "fake", "embedder provider: fake | openai | openai-compat | cohere | ollama | localai")
	embedderModel := flag.String("embedder-model", "", "embedder model id (provider-specific; defaults supplied for openai/cohere)")
	embedderKey := flag.String("embedder-key", "", "embedder API key (overrides EMBEDDER_API_KEY)")
	embedderBaseURL := flag.String("embedder-base-url", "", "embedder base URL (required for openai-compat; optional for ollama)")
	flag.Parse()

	wlog.SetDefault(wlog.New(*logFormat, *logLevel, os.Stderr))

	if *smoke {
		runSmoke()
		return
	}

	resolvedKey := resolveKey(*provider, *apiKey)
	if resolvedKey == "" {
		slog.Error("no API key found",
			"provider", *provider,
			"env_var", envKeyName(*provider),
			"hint", "set the env var or pass --api-key")
		os.Exit(2)
	}

	ctx := context.Background()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		fail("open store", err)
	}
	defer st.Close()

	embedderKeyResolved := *embedderKey
	if embedderKeyResolved == "" {
		embedderKeyResolved = os.Getenv("EMBEDDER_API_KEY")
	}
	embedder, err := memory.NewEmbedder(memory.EmbedderConfig{
		Provider: *embedderProvider,
		APIKey:   embedderKeyResolved,
		Model:    *embedderModel,
		BaseURL:  *embedderBaseURL,
	})
	if err != nil {
		fail("build embedder", err)
	}
	mem, err := memory.New(*memDir, embedder)
	if err != nil {
		fail("open memory", err)
	}
	defer mem.Close()
	slog.Info("memory initialized",
		"dir", *memDir,
		"embedder", *embedderProvider,
		"embedder_model", *embedderModel)

	baseScn, err := scenario.LoadBundled(*scenarioID)
	if err != nil {
		fail("load scenario", err)
	}

	// 跨周目 meta 加载（首次游玩或文件缺失返回空状态）。
	var meta *scenario.MetaState
	resolvedMetaPath := *metaPath
	if resolvedMetaPath == "-" {
		resolvedMetaPath = ""
		meta = &scenario.MetaState{}
	} else {
		meta, err = scenario.LoadMeta(resolvedMetaPath)
		if err != nil {
			fail("load meta", err)
		}
	}

	currentSaveID, scn, chosenVariant, opening, err := ensureSaveWithVariant(
		ctx, st, baseScn, *saveID, *variantID, *seed, mem,
	)
	if err != nil {
		fail("ensure save", err)
	}
	_ = chosenVariant

	llm, modelGM, modelNPC := buildLLM(*provider, resolvedKey)
	if *modelOverride != "" {
		modelGM = anthropic.Model(*modelOverride)
	}
	if *modelHelperOverride != "" {
		modelNPC = anthropic.Model(*modelHelperOverride)
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Store:         st,
		Memory:        mem,
		Scenario:      scn,
		LLMGM:         llm,
		LLMNPC:        llm,
		ModelGM:       modelGM,
		ModelNPC:      modelNPC,
		SaveID:        currentSaveID,
		RNG:           rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0xc0ffee)),
		AutosaveEvery: 3,
		VariantID:     chosenVariant,
		Meta:          meta,
		MetaPath:      resolvedMetaPath,
	})
	if err != nil {
		fail("build orchestrator", err)
	}

	model := tui.New(ctx, orch, st, opening)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fail("tui", err)
	}
}

func runSmoke() {
	slog.Info("smoke check passed", "llm_called", false)
}

// autoProvider 探测环境变量决定默认 provider。
//   - 仅 ANTHROPIC_API_KEY → anthropic
//   - 仅 OPENROUTER_API_KEY → openrouter
//   - 都有 / 都没有 → anthropic（保守默认；用户可显式 --provider openrouter 覆盖）
func autoProvider() string {
	hasAnthropic := os.Getenv("ANTHROPIC_API_KEY") != ""
	hasOpenRouter := os.Getenv("OPENROUTER_API_KEY") != ""
	if !hasAnthropic && hasOpenRouter {
		return providerOpenRouter
	}
	return providerAnthropic
}

func envKeyName(provider string) string {
	if normalizedProvider(provider) == providerOpenRouter {
		return "OPENROUTER_API_KEY"
	}
	return "ANTHROPIC_API_KEY"
}

func resolveKey(provider, override string) string {
	if override != "" {
		return override
	}
	return os.Getenv(envKeyName(provider))
}

func normalizedProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case providerOpenRouter, "or":
		return providerOpenRouter
	default:
		return providerAnthropic
	}
}

// buildLLM 根据 provider 构造 Anthropic 客户端 + 选择模型常量。
func buildLLM(provider, key string) (*agent.Anthropic, anthropic.Model, anthropic.Model) {
	switch normalizedProvider(provider) {
	case providerOpenRouter:
		return agent.NewAnthropic(agent.ClientConfig{
				AuthToken: key,
				BaseURL:   agent.OpenRouterBaseURL,
			}),
			agent.OpenRouterModelGM,
			agent.OpenRouterModelHelper
	default:
		return agent.NewAnthropic(agent.ClientConfig{
				APIKey:  key,
				BaseURL: agent.AnthropicBaseURL, // 显式指定，避免 SDK 读取用户环境里的 ANTHROPIC_BASE_URL
			}),
			agent.ModelGM, agent.ModelHelper
	}
}

// ensureSaveWithVariant 若 saveID 为空则创建新 save + 选定 variant + Apply effective scenario；
// 否则按存档中已记录的 variant_id 重新 merge effective scenario，保证读档的一致性。
//
// 参数：
//   - forceVariant：CLI --variant 强制指定（空时随机）；新 save 模式才生效
//   - seed：CLI --seed 控制确定性；0 取 unix nano
//
// 返回：saveID / effective scenario / 选中 variant id（可空）/ 开场白 / 错误。
func ensureSaveWithVariant(
	ctx context.Context,
	st *store.Store,
	base *scenario.Scenario,
	saveID, forceVariant string,
	seed int64,
	mem *memory.Memory,
) (string, *scenario.Scenario, string, string, error) {
	repo := st.Repo()
	if saveID != "" {
		sv, err := repo.GetSave(ctx, saveID)
		if err != nil {
			return "", nil, "", "", fmt.Errorf("save %q not found: %w", saveID, err)
		}
		eff, vid, err := scenario.SelectVariantByID(base, sv.VariantID)
		if err != nil {
			return "", nil, "", "", fmt.Errorf("re-select variant %q: %w", sv.VariantID, err)
		}
		return saveID, eff, vid, "", nil
	}
	id := uuid.NewString()

	var eff *scenario.Scenario
	var chosen string
	var err error
	if forceVariant != "" {
		eff, chosen, err = scenario.SelectVariantByID(base, forceVariant)
	} else {
		s := seed
		if s == 0 {
			s = time.Now().UnixNano()
		}
		rng := rand.New(rand.NewPCG(uint64(s), 0xfeed))
		eff, chosen, err = scenario.SelectVariant(base, rng)
	}
	if err != nil {
		return "", nil, "", "", fmt.Errorf("select variant: %w", err)
	}

	if err := repo.CreateSave(ctx, store.Save{
		ID: id, Name: "untitled", ScenarioID: base.ID, VariantID: chosen,
	}); err != nil {
		return "", nil, "", "", err
	}
	if err := repo.UpsertInvestigator(ctx, store.Investigator{
		ID: uuid.NewString(), SaveID: id,
		Name: "未命名调查员", Occupation: "记者",
		AttrsJSON:  `{"STR":50,"CON":60,"SIZ":55,"DEX":60,"APP":50,"INT":75,"POW":60,"EDU":80}`,
		SkillsJSON: `{"Spot Hidden":50,"Library Use":60,"Listen":40,"Psychology":40}`,
		HP:         12, MP: 12, SAN: 60,
		InventoryJSON: `["笔记本","钢笔"]`,
		Active:        true,
	}); err != nil {
		return "", nil, "", "", err
	}
	engine := scenario.New(eff, repo, mem)
	if err := engine.Apply(ctx, id); err != nil {
		return "", nil, "", "", fmt.Errorf("scenario apply: %w", err)
	}
	variantHint := ""
	if chosen != "" {
		variantHint = fmt.Sprintf("（variant: %s）", chosen)
	}
	opening := fmt.Sprintf(
		"《%s》开场%s。剧本 id: %s。当前位置: %s。\n（输入 /help 查看命令；输入你想做的事开始游戏。）",
		base.Title, variantHint, base.ID, base.Start.Location,
	)
	return id, eff, chosen, opening, nil
}

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
