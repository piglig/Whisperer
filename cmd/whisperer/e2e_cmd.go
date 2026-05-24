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

	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	wlog "github.com/zhuzhenwu/whisperer/internal/log"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func runE2ESubcommand(args []string) {
	fs := flag.NewFlagSet("e2e", flag.ExitOnError)
	provider := fs.String("provider", agent.AutoProvider(), "anthropic | openrouter | openai | grok | gemini")
	apiKey := fs.String("api-key", "", "override provider API key; else read env")
	input := fs.String("input", "我刚到雾港码头，先环顾四周，再向最近的人打听情况。", "user input for the first turn")
	inputsFile := fs.String("inputs-file", "", "file with one player input per line; supersedes --input/--turns")
	turns := fs.Int("turns", 1, "turn count; ignored if --inputs-file is given")
	scenarioID := fs.String("scenario", "fog_harbor", "bundled scenario id")
	modelOverride := fs.String("model", "", "override GM model id")
	modelHelperOverride := fs.String("model-helper", "", "override helper model id")
	enableJudge := fs.Bool("enable-judge", false, "enable LLM-as-judge semantic SLA checks")
	judgeModel := fs.String("judge-model", "", "override judge model id; empty uses helper model")
	stopOnEnding := fs.Bool("stop-on-ending", true, "halt as soon as an ending is reached")
	variantID := fs.String("variant", "", "force variant id; empty for weighted random")
	seed := fs.Int64("seed", 0, "deterministic variant seed (0 = unix nano)")
	logFormat := fs.String("log-format", "text", "log handler format: text | json")
	logLevel := fs.String("log-level", "info", "log level: debug | info | warn | error")
	llmMaxRetries := fs.Int("llm-max-retries", 3, "max retries on transient LLM failures")
	llmTimeout := fs.Duration("llm-timeout", 120*time.Second, "per-LLM-call hard timeout")
	traceDir := fs.String("trace-dir", "runs", "directory to append per-turn JSONL traces; '-' to disable")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(2)
	}

	wlog.SetDefault(wlog.New(*logFormat, *logLevel, os.Stderr))

	key := *apiKey
	if key == "" {
		key = os.Getenv(agent.ProviderInfo(*provider).EnvKey)
	}
	if key == "" {
		slog.Error("no API key found",
			"provider", *provider,
			"env_var", agent.ProviderInfo(*provider).EnvKey,
			"hint", "set the env var or pass --api-key")
		os.Exit(2)
	}

	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	e2eMust("open store", err)
	defer st.Close()

	mem, err := memory.New("", memory.NewFakeEmbedder(0))
	e2eMust("open memory", err)
	defer mem.Close()

	baseScn, err := scenario.LoadBundled(*scenarioID)
	e2eMust("load scenario", err)
	scn, chosenVariant, err := selectE2EVariant(baseScn, *variantID, *seed)
	e2eMust("select variant", err)
	if chosenVariant != "" {
		slog.Info("variant selected", "variant_id", chosenVariant)
	}

	saveID := uuid.NewString()
	e2eMust("create save", st.Repo().CreateSave(ctx, store.Save{
		ID: saveID, Name: "e2e", ScenarioID: baseScn.ID, VariantID: chosenVariant,
	}))
	e2eMust("create investigator", st.Repo().UpsertInvestigator(ctx, store.Investigator{
		ID: uuid.NewString(), SaveID: saveID,
		Name: "Lyra Marsh", Occupation: "记者",
		AttrsJSON:     `{"STR":50,"CON":60,"SIZ":55,"DEX":60,"APP":50,"INT":75,"POW":60,"EDU":80}`,
		SkillsJSON:    `{"Spot Hidden":50,"Library Use":60,"Listen":40,"Psychology":40}`,
		HP:            12,
		MP:            12,
		SAN:           60,
		InventoryJSON: `["笔记本","钢笔"]`,
		Active:        true,
	}))
	e2eMust("scenario apply", scenario.New(scn, st.Repo(), mem).Apply(ctx, saveID))

	llm, modelGM, modelNPC := agent.BuildLLM(*provider, key, *llmMaxRetries, *llmTimeout)
	if *modelOverride != "" {
		modelGM = agent.Model(*modelOverride)
	}
	if *modelHelperOverride != "" {
		modelNPC = agent.Model(*modelHelperOverride)
	}
	modelJudge := modelNPC
	if *judgeModel != "" {
		modelJudge = agent.Model(*judgeModel)
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Store:     st,
		Memory:    mem,
		Scenario:  scn,
		LLMGM:     llm,
		LLMNPC:    llm,
		ModelGM:   modelGM,
		ModelNPC:  modelNPC,
		Judge:     buildJudge(*enableJudge, llm, modelJudge),
		SaveID:    saveID,
		RNG:       rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0xc0ffee)),
		VariantID: chosenVariant,
		TraceDir:  *traceDir,
	})
	e2eMust("build orchestrator", err)

	inputs, totalTurns := buildE2EInputPlan(*input, *inputsFile, *turns)
	runE2ETurns(ctx, orch, inputs, totalTurns, *stopOnEnding)
}

func selectE2EVariant(base *scenario.Scenario, variantID string, seed int64) (*scenario.Scenario, string, error) {
	if variantID != "" {
		return scenario.SelectVariantByID(base, variantID)
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return scenario.SelectVariant(base, rand.New(rand.NewPCG(uint64(seed), 0xfeed)))
}

func runE2ETurns(ctx context.Context, orch *orchestrator.Orchestrator, inputs []string, totalTurns int, stopOnEnding bool) {
	totalIn, totalOut := int64(0), int64(0)
	totalCost := 0.0
	totalDur := time.Duration(0)
	for i := 1; i <= totalTurns; i++ {
		playerInput := nextE2EInput(inputs, i)
		fmt.Printf("\n=== Turn %d ===\n[player] %s\n", i, playerInput)
		start := time.Now()
		res, err := orch.RunTurn(ctx, playerInput)
		dur := time.Since(start)
		e2eMust(fmt.Sprintf("run turn %d", i), err)

		totalIn += res.Trace.InputTokens
		totalOut += res.Trace.OutputTokens
		totalCost += res.Trace.TotalCostUSD
		totalDur += dur
		printE2ESummary(res, dur)
		if res.Ending != nil {
			fmt.Printf("\n[ENDING %s/%s] %s\n", res.Ending.Kind, res.Ending.ID, res.Ending.Description)
			if stopOnEnding {
				break
			}
		}
	}
	fmt.Printf("\n=== Run summary ===\n")
	fmt.Printf("  total_input_tokens : %d\n", totalIn)
	fmt.Printf("  total_output_tokens: %d\n", totalOut)
	fmt.Printf("  total_cost_usd     : $%.4f  (≈ $%.2f/100K tokens)\n", totalCost, costPerHundredK(totalIn+totalOut, totalCost))
	fmt.Printf("  wall_clock         : %s\n", totalDur)
}

func costPerHundredK(totalTokens int64, totalCost float64) float64 {
	if totalTokens == 0 {
		return 0
	}
	return totalCost / float64(totalTokens) * 100_000
}

func buildE2EInputPlan(firstInput, inputsFile string, turns int) ([]string, int) {
	if inputsFile == "" {
		return []string{firstInput}, turns
	}
	raw, err := os.ReadFile(inputsFile)
	e2eMust("read inputs file", err)
	var list []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		list = append(list, line)
	}
	if len(list) == 0 {
		e2eMust("inputs file", fmt.Errorf("no usable lines in %s", inputsFile))
	}
	return list, len(list)
}

func nextE2EInput(plan []string, turn int) string {
	if turn-1 < len(plan) {
		return plan[turn-1]
	}
	return "继续按 GM 引导推进。"
}

func printE2ESummary(res orchestrator.TurnResult, dur time.Duration) {
	fmt.Printf("[gm] %s\n", strings.TrimSpace(res.Narrative))
	fmt.Printf("\n[trace]\n")
	fmt.Printf("  duration         : %s\n", dur)
	fmt.Printf("  iterations       : %d (truncated=%v)\n", res.Trace.Iterations, res.Trace.Truncated)
	fmt.Printf("  input_tokens     : %d\n", res.Trace.InputTokens)
	fmt.Printf("  output_tokens    : %d\n", res.Trace.OutputTokens)
	fmt.Printf("  cost_usd         : $%.4f  (in $%.4f / out $%.4f)\n", res.Trace.TotalCostUSD, res.Trace.InputCostUSD, res.Trace.OutputCostUSD)
	fmt.Printf("  tool_calls       : %d\n", len(res.Trace.ToolCalls))
	for _, tc := range res.Trace.ToolCalls {
		marker := " "
		if tc.IsError {
			marker = "!"
		}
		fmt.Printf("    [%s iter=%d] %s %s\n", marker, tc.Iter, tc.Name, truncateE2E(string(tc.Input), 120))
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

func truncateE2E(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func e2eMust(label string, err error) {
	if err == nil {
		return
	}
	attrs := []any{"err", err}
	var anyErr any = err
	if b, jerr := json.Marshal(anyErr); jerr == nil {
		attrs = append(attrs, "raw", string(b))
	}
	slog.Error(label, attrs...)
	os.Exit(1)
}
