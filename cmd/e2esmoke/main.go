// e2esmoke 跑一次真实 LLM RunTurn，验证从 prompt → tool 调用 → 状态写入 → SLA
// 全链路是否真的能跑通。打印 trace 摘要后退出，不启动 TUI。
//
// 用法:
//
//	OPENROUTER_API_KEY=sk-or-... go run ./cmd/e2esmoke -input "我环顾码头四周"
//	ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/e2esmoke -provider anthropic
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	wlog "github.com/zhuzhenwu/whisperer/internal/log"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func main() {
	provider := flag.String("provider", autoProvider(), "anthropic | openrouter")
	apiKey := flag.String("api-key", "", "override; else read env")
	input := flag.String("input", "我刚到雾港码头，先环顾四周，再向最近的人打听情况。", "user input for the first turn")
	inputsFile := flag.String("inputs-file", "", "path to a file with one player input per line; supersedes --input/--turns. Empty lines and lines starting with # are skipped.")
	turns := flag.Int("turns", 1, "how many turns to run; ignored if --inputs-file is given")
	scenarioID := flag.String("scenario", "fog_harbor", "bundled scenario id")
	modelOverride := flag.String("model", "", "override GM model id")
	modelHelperOverride := flag.String("model-helper", "", "override Haiku/helper model id")
	stopOnEnding := flag.Bool("stop-on-ending", true, "halt as soon as the scenario ending is reached")
	variantID := flag.String("variant", "", "force a specific variant id; empty for weighted random")
	seed := flag.Int64("seed", 0, "deterministic variant selection seed (0 = unix nano)")
	logFormat := flag.String("log-format", "text", "log handler format: text | json")
	logLevel := flag.String("log-level", "info", "log level: debug | info | warn | error")
	llmMaxRetries := flag.Int("llm-max-retries", 3, "max retries on transient LLM failures (0 = SDK default)")
	llmTimeout := flag.Duration("llm-timeout", 120*time.Second, "per-LLM-call hard timeout (0 = no timeout)")
	traceDir := flag.String("trace-dir", "runs", "directory to append per-turn JSONL traces; '-' to disable")
	flag.Parse()

	wlog.SetDefault(wlog.New(*logFormat, *logLevel, os.Stderr))

	key := *apiKey
	if key == "" {
		key = os.Getenv(envName(*provider))
	}
	if key == "" {
		slog.Error("no API key found",
			"provider", *provider,
			"env_var", envName(*provider),
			"hint", "set the env var or pass --api-key")
		os.Exit(2)
	}

	ctx := context.Background()

	st, err := store.Open(ctx, ":memory:")
	must("open store", err)
	defer st.Close()

	mem, err := memory.New("", memory.NewFakeEmbedder(0))
	must("open memory", err)
	defer mem.Close()

	baseScn, err := scenario.LoadBundled(*scenarioID)
	must("load scenario", err)

	var scn *scenario.Scenario
	var chosenVariant string
	if *variantID != "" {
		scn, chosenVariant, err = scenario.SelectVariantByID(baseScn, *variantID)
	} else {
		s := *seed
		if s == 0 {
			s = time.Now().UnixNano()
		}
		rng := rand.New(rand.NewPCG(uint64(s), 0xfeed))
		scn, chosenVariant, err = scenario.SelectVariant(baseScn, rng)
	}
	must("select variant", err)
	if chosenVariant != "" {
		slog.Info("variant selected", "variant_id", chosenVariant)
	}

	saveID := uuid.NewString()
	must("create save", st.Repo().CreateSave(ctx, store.Save{
		ID: saveID, Name: "e2e", ScenarioID: baseScn.ID, VariantID: chosenVariant,
	}))
	must("create investigator", st.Repo().UpsertInvestigator(ctx, store.Investigator{
		ID: uuid.NewString(), SaveID: saveID,
		Name: "Lyra Marsh", Occupation: "记者",
		AttrsJSON:  `{"STR":50,"CON":60,"SIZ":55,"DEX":60,"APP":50,"INT":75,"POW":60,"EDU":80}`,
		SkillsJSON: `{"Spot Hidden":50,"Library Use":60,"Listen":40,"Psychology":40}`,
		HP:         12, MP: 12, SAN: 60,
		InventoryJSON: `["笔记本","钢笔"]`,
		Active:        true,
	}))
	engine := scenario.New(scn, st.Repo(), mem)
	must("scenario apply", engine.Apply(ctx, saveID))

	llm, modelGM, modelNPC := buildLLM(*provider, key, *llmMaxRetries, *llmTimeout)
	if *modelOverride != "" {
		modelGM = anthropic.Model(*modelOverride)
	}
	if *modelHelperOverride != "" {
		modelNPC = anthropic.Model(*modelHelperOverride)
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Store:     st,
		Memory:    mem,
		Scenario:  scn,
		LLMGM:     llm,
		LLMNPC:    llm,
		ModelGM:   modelGM,
		ModelNPC:  modelNPC,
		SaveID:    saveID,
		RNG:       rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0xc0ffee)),
		VariantID: chosenVariant,
		TraceDir:  *traceDir,
	})
	must("build orchestrator", err)

	inputs, totalTurns := buildInputPlan(*input, *inputsFile, *turns)

	totalIn, totalOut := int64(0), int64(0)
	totalCost := 0.0
	totalDur := time.Duration(0)
	for i := 1; i <= totalTurns; i++ {
		ti := nextInput(inputs, i)
		fmt.Printf("\n=== Turn %d ===\n", i)
		fmt.Printf("[player] %s\n", ti)

		start := time.Now()
		res, err := orch.RunTurn(ctx, ti)
		dur := time.Since(start)
		must(fmt.Sprintf("run turn %d", i), err)

		totalIn += res.Trace.InputTokens
		totalOut += res.Trace.OutputTokens
		totalCost += res.Trace.TotalCostUSD
		totalDur += dur

		printSummary(res, dur)

		if res.Ending != nil {
			fmt.Printf("\n[ENDING %s/%s] %s\n", res.Ending.Kind, res.Ending.ID, res.Ending.Description)
			if *stopOnEnding {
				break
			}
		}
	}

	fmt.Printf("\n=== Run summary ===\n")
	fmt.Printf("  total_input_tokens : %d\n", totalIn)
	fmt.Printf("  total_output_tokens: %d\n", totalOut)
	fmt.Printf("  total_cost_usd     : $%.4f  (≈ $%.2f/100K tokens)\n",
		totalCost, costPerHundredK(totalIn+totalOut, totalCost))
	fmt.Printf("  wall_clock         : %s\n", totalDur)
}

// costPerHundredK 把总消耗摊算到每 100K tokens 的 USD，便于对比模型档位。
// totalTokens == 0 时返回 0。
func costPerHundredK(totalTokens int64, totalCost float64) float64 {
	if totalTokens == 0 {
		return 0
	}
	return totalCost / float64(totalTokens) * 100_000
}

// buildInputPlan 决定本次跑的输入序列。
//
//   - 若 inputsFile 非空：读文件，每行一条 input；跳过空行和 # 注释行。
//     总回合数 = 文件行数。
//   - 否则：第 1 回合用 firstInput，剩余回合走 nextInput 的退化值；
//     总回合数 = turns flag。
func buildInputPlan(firstInput, inputsFile string, turns int) ([]string, int) {
	if inputsFile != "" {
		raw, err := os.ReadFile(inputsFile)
		must("read inputs file", err)
		var list []string
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			list = append(list, line)
		}
		if len(list) == 0 {
			must("inputs file", fmt.Errorf("no usable lines in %s", inputsFile))
		}
		return list, len(list)
	}
	return []string{firstInput}, turns
}

// nextInput 选第 i 回合（1-based）的玩家输入；超出脚本时退化为"继续推进"。
func nextInput(plan []string, i int) string {
	if i-1 < len(plan) {
		return plan[i-1]
	}
	return "继续按 GM 引导推进。"
}

func printSummary(res orchestrator.TurnResult, dur time.Duration) {
	fmt.Printf("[gm] %s\n", strings.TrimSpace(res.Narrative))

	fmt.Printf("\n[trace]\n")
	fmt.Printf("  duration         : %s\n", dur)
	fmt.Printf("  iterations       : %d (truncated=%v)\n", res.Trace.Iterations, res.Trace.Truncated)
	fmt.Printf("  input_tokens     : %d\n", res.Trace.InputTokens)
	fmt.Printf("  output_tokens    : %d\n", res.Trace.OutputTokens)
	fmt.Printf("  cost_usd         : $%.4f  (in $%.4f / out $%.4f)\n",
		res.Trace.TotalCostUSD, res.Trace.InputCostUSD, res.Trace.OutputCostUSD)
	fmt.Printf("  tool_calls       : %d\n", len(res.Trace.ToolCalls))
	for _, tc := range res.Trace.ToolCalls {
		marker := " "
		if tc.IsError {
			marker = "!"
		}
		fmt.Printf("    [%s iter=%d] %s %s\n", marker, tc.Iter, tc.Name, truncate(string(tc.Input), 120))
	}

	fmt.Printf("\n[scenario]\n")
	fmt.Printf("  drift            : %s\n", res.Drift.String())
	fmt.Printf("  fired_triggers   : %d\n", len(res.Fired))
	for _, f := range res.Fired {
		fmt.Printf("    - %s\n", f.ID)
	}

	fmt.Printf("\n[sla]\n")
	fmt.Printf("  passed           : %v\n", res.SLAReport.Passed)
	fmt.Printf("  ending_forced    : %v\n", res.SLAReport.EndingForced)
	for _, v := range res.SLAReport.Violations {
		fmt.Printf("    [%s] %s\n", v.Code, v.Message)
	}

	fmt.Printf("\n[save]\n")
	fmt.Printf("  turn             : %d\n", res.Save.TurnCount)
	fmt.Printf("  current_location : %s\n", res.Save.CurrentLocationID)
	fmt.Printf("  time_of_day      : %s\n", res.Save.TimeOfDay)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// 与 cmd/whisperer/main.go 同样的 provider 解析逻辑，独立避免循环依赖。

func autoProvider() string {
	if os.Getenv("ANTHROPIC_API_KEY") == "" && os.Getenv("OPENROUTER_API_KEY") != "" {
		return "openrouter"
	}
	return "anthropic"
}

func envName(p string) string {
	if normalized(p) == "openrouter" {
		return "OPENROUTER_API_KEY"
	}
	return "ANTHROPIC_API_KEY"
}

func normalized(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "openrouter", "or":
		return "openrouter"
	default:
		return "anthropic"
	}
}

func buildLLM(provider, key string, maxRetries int, timeout time.Duration) (*agent.Anthropic, anthropic.Model, anthropic.Model) {
	if normalized(provider) == "openrouter" {
		return agent.NewAnthropic(agent.ClientConfig{
				AuthToken:      key,
				BaseURL:        agent.OpenRouterBaseURL,
				MaxRetries:     maxRetries,
				RequestTimeout: timeout,
			}),
			agent.OpenRouterModelGM,
			agent.OpenRouterModelHelper
	}
	return agent.NewAnthropic(agent.ClientConfig{
			APIKey:         key,
			BaseURL:        agent.AnthropicBaseURL,
			MaxRetries:     maxRetries,
			RequestTimeout: timeout,
		}),
		agent.ModelGM, agent.ModelHelper
}

func must(label string, err error) {
	if err != nil {
		attrs := []any{"err", err}
		// json marshal pretty 抓出更多 SDK 嵌套错误细节
		var anyErr any = err
		if b, jerr := json.Marshal(anyErr); jerr == nil {
			attrs = append(attrs, "raw", string(b))
		}
		slog.Error(label, attrs...)
		os.Exit(1)
	}
}
