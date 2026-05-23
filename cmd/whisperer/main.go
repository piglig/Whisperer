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
// 用法（OpenAI / Grok / Gemini）:
//
//	export OPENAI_API_KEY=sk-...
//	whisperer --provider openai
//	export XAI_API_KEY=xai-...
//	whisperer --provider grok
//	export GEMINI_API_KEY=...
//	whisperer --provider gemini
//
// 缺省加载内置 fog_harbor 剧本，自动创建 save 与一名快速模板调查员。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/clierror"
	"github.com/zhuzhenwu/whisperer/internal/config"
	"github.com/zhuzhenwu/whisperer/internal/i18n"
	"github.com/zhuzhenwu/whisperer/internal/investigator"
	wlog "github.com/zhuzhenwu/whisperer/internal/log"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
	"github.com/zhuzhenwu/whisperer/internal/tui"
	"github.com/zhuzhenwu/whisperer/internal/tui/saveselect"
)

// globalTranslator 在 main 解析完 --lang 后注入；fail() 取它做 i18n 渲染。
// 早于 i18n 初始化的错误（极少；只有 flag 解析阶段）走 nil-safe 退化路径。
var globalTranslator *i18n.Translator

func main() {
	// Phase 1: 预扫描 args 找到 --config，先把 TOML 加载好作为后续 flag 的默认值。
	configPath := preParseConfigFlag(os.Args[1:])
	resolvedConfigPath := resolveWizardConfigPath(configPath)

	// 极早分支：`whisperer init` 子命令在加载 config 之前就跑向导（避免空配置撞错误）
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInitSubcommand(resolvedConfigPath)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "scenario" {
		runScenarioSubcommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "fog-harbor" {
		runFogHarborSubcommand(os.Args[2:])
		return
	}

	fileCfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(2)
	}

	// 首次运行向导：configPath 文件不存在 + stdin 是 tty + 没传 init 子命令时
	// 自动触发，让用户写一个 config 再继续。
	if shouldRunWizard(nil, resolvedConfigPath) {
		// 注意 i18n 此时还没初始化——用 stub 走默认 zh-CN
		stubTr, _ := i18n.New("")
		_, _, werr := runWizard(newWizardEnv(stubTr), resolvedConfigPath)
		if werr != nil {
			fmt.Fprintf(os.Stderr, "wizard: %v\n", werr)
			os.Exit(2)
		}
		// 重新读 config 让后续 flag 默认值用上向导写入的值
		fileCfg, err = config.Load(resolvedConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(2)
		}
	}

	// Phase 2: 注册全部 flag，把 fileCfg 作为它们的初始值。flag.Parse 之后，
	// 命令行显式给的 flag 会覆盖 fileCfg 上的值；没给的就保留 fileCfg / 默认值。
	_ = flag.String("config", configPath, "config file path (default: $XDG_CONFIG_HOME/whisperer/config.toml)")

	dbPath := flag.String("db", fileCfg.DBPath, "SQLite save file path")
	memDir := flag.String("memory", fileCfg.MemDir, "memory persistent dir; empty for in-memory")
	scenarioID := flag.String("scenario", fileCfg.Scenario, "bundled scenario id")
	saveID := flag.String("save", fileCfg.Save, "existing save id to load; empty creates a new save")
	provider := flag.String("provider", defaultProvider(fileCfg.Provider), "LLM provider: anthropic | openrouter | openai | grok | gemini")
	apiKey := flag.String("api-key", "", "API key (overrides provider env var)")
	modelOverride := flag.String("model", fileCfg.Model, "override GM model id (full vendor/model on OpenRouter)")
	modelHelperOverride := flag.String("model-helper", fileCfg.ModelHelper, "override Haiku/helper model id")
	smoke := flag.Bool("smoke", false, "smoke test mode: do not call any LLM, print rules samples")
	investigatorTemplate := flag.String("investigator-template", "journalist", "new-save investigator template: journalist | private_eye | doctor")
	variantID := flag.String("variant", fileCfg.Variant, "force a specific variant id (default: weighted random)")
	seed := flag.Int64("seed", fileCfg.Seed, "deterministic variant selection seed (0 = unix nano)")
	metaPath := flag.String("meta", fileCfg.MetaPath, "cross-run meta file path; '-' to disable")
	logFormat := flag.String("log-format", fileCfg.LogFormat, "log handler format: text | json")
	logLevel := flag.String("log-level", fileCfg.LogLevel, "log level: debug | info | warn | error")
	llmMaxRetries := flag.Int("llm-max-retries", fileCfg.LLMMaxRetries, "max retries on transient LLM failures (0 = SDK default)")
	llmTimeout := flag.Duration("llm-timeout", fileCfg.LLMTimeout, "per-LLM-call hard timeout (0 = no timeout)")
	traceDir := flag.String("trace-dir", fileCfg.TraceDir, "directory to append per-turn JSONL traces; '-' to disable")
	lang := flag.String("lang", fileCfg.Lang, "UI language tag (zh-CN | en | auto); empty/auto = detect from $LANG / $LC_ALL")
	flag.Parse()

	wlog.SetDefault(wlog.New(*logFormat, *logLevel, os.Stderr))

	resolvedLang := *lang
	if resolvedLang == "" || resolvedLang == "auto" {
		resolvedLang = i18n.DetectLang()
	}
	tr, err := i18n.New(resolvedLang)
	if err != nil {
		fail("init i18n", err)
	}
	globalTranslator = tr

	if *smoke {
		runSmoke()
		return
	}

	resolvedKey := resolveKey(*provider, *apiKey)
	if resolvedKey == "" {
		slog.Error("no API key found",
			"provider", *provider,
			"env_var", agent.ProviderInfo(*provider).EnvKey,
			"hint", "set the env var or pass --api-key")
		os.Exit(2)
	}

	ctx := context.Background()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		fail("open store", err)
	}
	defer st.Close()

	baseScn, err := scenario.LoadBundled(*scenarioID)
	if err != nil {
		fail("load scenario", err)
	}

	// 若 --save 为空且数据库里有存档，先弹存档选择器。
	if *saveID == "" {
		chosen, quit, err := runSaveSelector(ctx, st, *memDir)
		if err != nil {
			fail("save selector", err)
		}
		if quit {
			return
		}
		*saveID = chosen // 为空表示玩家选择了"新建"
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

	currentSaveID, scn, chosenVariant, opening, needsScenarioApply, err := ensureSaveWithVariant(
		ctx, st, baseScn, *saveID, *variantID, *seed, *investigatorTemplate,
	)
	if err != nil {
		fail("ensure save", err)
	}
	_ = chosenVariant

	saveMemDir := memoryDirForSave(*memDir, currentSaveID)
	mem, err := memory.New(saveMemDir, memory.NewFakeEmbedder(0))
	if err != nil {
		fail("open memory", err)
	}
	defer mem.Close()
	slog.Info("memory initialized", "dir", saveMemDir)

	if needsScenarioApply {
		engine := scenario.New(scn, st.Repo(), mem)
		if err := engine.Apply(ctx, currentSaveID); err != nil {
			fail("scenario apply", err)
		}
	}

	llm, modelGM, modelNPC := agent.BuildLLM(*provider, resolvedKey, *llmMaxRetries, *llmTimeout)
	if *modelOverride != "" {
		modelGM = agent.Model(*modelOverride)
	}
	if *modelHelperOverride != "" {
		modelNPC = agent.Model(*modelHelperOverride)
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
		TraceDir:      *traceDir,
	})
	if err != nil {
		fail("build orchestrator", err)
	}

	model := tui.New(ctx, orch, st, opening, scn)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fail("tui", err)
	}
}

func runSmoke() {
	slog.Info("smoke check passed", "llm_called", false)
}

func resolveKey(provider, override string) string {
	if override != "" {
		return override
	}
	return os.Getenv(agent.ProviderInfo(provider).EnvKey)
}

// ensureSaveWithVariant 若 saveID 为空则创建新 save + 选定 variant；
// 否则按存档中已记录的 variant_id 重新 merge effective scenario，保证读档的一致性。
//
// 参数：
//   - forceVariant：CLI --variant 强制指定（空时随机）；新 save 模式才生效
//   - seed：CLI --seed 控制确定性；0 取 unix nano
//
// 返回：saveID / effective scenario / 选中 variant id（可空）/ 开场白 /
// 是否需要 Apply scenario / 错误。
func ensureSaveWithVariant(
	ctx context.Context,
	st *store.Store,
	base *scenario.Scenario,
	saveID, forceVariant string,
	seed int64,
	investigatorTemplateID string,
) (string, *scenario.Scenario, string, string, bool, error) {
	repo := st.Repo()
	if saveID != "" {
		sv, err := repo.GetSave(ctx, saveID)
		if err != nil {
			return "", nil, "", "", false, fmt.Errorf("save %q not found: %w", saveID, err)
		}
		eff, vid, err := scenario.SelectVariantByID(base, sv.VariantID)
		if err != nil {
			return "", nil, "", "", false, fmt.Errorf("re-select variant %q: %w", sv.VariantID, err)
		}
		return saveID, eff, vid, "", false, nil
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
		return "", nil, "", "", false, fmt.Errorf("select variant: %w", err)
	}

	if err := repo.CreateSave(ctx, store.Save{
		ID: id, Name: "untitled", ScenarioID: base.ID, VariantID: chosen,
	}); err != nil {
		return "", nil, "", "", false, err
	}
	tmpl, ok := investigator.ByID(investigatorTemplateID)
	if !ok {
		return "", nil, "", "", false, investigator.ErrUnknownTemplate(investigatorTemplateID)
	}
	if err := repo.UpsertInvestigator(ctx, investigator.NewFromTemplate(id, tmpl)); err != nil {
		return "", nil, "", "", false, err
	}
	opening := scenario.RenderOpeningBriefing(eff)
	return id, eff, chosen, opening, true, nil
}

func memoryDirForSave(baseDir, saveID string) string {
	if baseDir == "" {
		return ""
	}
	return filepath.Join(baseDir, saveID)
}

// runSaveSelector 在已有存档存在时弹出 TUI 选择器。
// 返回 (chosenSaveID, quit, err)：
//   - quit=true 表示玩家主动退出，主程序应直接返回
//   - chosenSaveID 空字符串表示玩家选择"新建"，主程序走默认 ensureSaveWithVariant
//   - 数据库里完全没有存档时跳过选择器，直接落到新建流程
func runSaveSelector(ctx context.Context, st *store.Store, memDir string) (string, bool, error) {
	repo := st.Repo()
	saves, err := repo.ListSaves(ctx)
	if err != nil {
		return "", false, fmt.Errorf("list saves: %w", err)
	}
	if len(saves) == 0 {
		return "", false, nil
	}

	onDelete := func(id string) error {
		dir := memoryDirForSave(memDir, id)
		if dir == "" {
			return nil
		}
		return os.RemoveAll(dir)
	}

	model := saveselect.New(saves, repo, onDelete)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := prog.Run()
	if err != nil {
		return "", false, err
	}
	res := finalModel.(saveselect.Model).Result()
	switch res.Action {
	case saveselect.ActionQuit:
		return "", true, nil
	case saveselect.ActionLoad:
		return res.SaveID, false, nil
	case saveselect.ActionNew:
		return "", false, nil
	}
	return "", false, nil
}

// runInitSubcommand 处理 `whisperer init` ——重新跑向导，把结果写到 configPath。
// 与首次运行向导共享 runWizard，但允许覆盖既有文件。
func runInitSubcommand(configPath string) {
	tr, _ := i18n.New("")
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "wizard: cannot resolve config path; pass --config <path>")
		os.Exit(2)
	}
	if _, _, err := runWizard(newWizardEnv(tr), configPath); err != nil {
		fmt.Fprintf(os.Stderr, "wizard: %v\n", err)
		os.Exit(2)
	}
}

// preParseConfigFlag 在 flag.Parse 之前手工扫一次 args 找 --config / -config。
//
// 必要因为：要先知道 config 文件路径，才能用 TOML 内容作为后续 flag.StringVar 的
// 默认值——而 flag.StringVar 必须在 flag.Parse 前注册完毕。
func preParseConfigFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--config" || a == "-config":
			if i+1 < len(args) {
				return args[i+1]
			}
		case strings.HasPrefix(a, "--config="):
			return strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-config="):
			return strings.TrimPrefix(a, "-config=")
		}
	}
	return ""
}

func defaultProvider(provider string) string {
	if provider != "" {
		return provider
	}
	return agent.AutoProvider()
}

func fail(msg string, err error) {
	// 友好提示走 stderr 给用户看；同时打一条 slog.Debug 把原始 error 留给观察层。
	fmt.Fprintln(os.Stderr, clierror.Format(globalTranslator, msg, err))
	slog.Debug("fail", "op", msg, "err", err)
	os.Exit(1)
}
