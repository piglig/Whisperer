package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/config"
	"github.com/zhuzhenwu/whisperer/internal/i18n"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

// wizardEnv 把所有外部依赖（提示输出、用户输入、退出码）抽进结构体，方便测试
// 注入 stub。生产路径在 runWizard 调用 newWizardEnv() 时绑定 stdin / stderr。
type wizardEnv struct {
	in          *bufio.Reader
	out         io.Writer
	tr          *i18n.Translator
	interactive bool
}

func newWizardEnv(tr *i18n.Translator) *wizardEnv {
	return &wizardEnv{
		in:          bufio.NewReader(os.Stdin),
		out:         os.Stderr,
		tr:          tr,
		interactive: stdinIsTTY(),
	}
}

// wizardResult 是向导收集到的偏好。空字段 → 用户跳过 / 接受默认。
type wizardResult struct {
	Provider  string
	APIKey    string // 仅返回给调用方做提示，不写文件
	APIKeyEnv string // 提示用户该把 key 放到哪个环境变量
	Scenario  string
	Lang      string // "auto" / "en" / "zh-CN"
}

// runWizard 是首次运行 / `whisperer init` 路径。
//
//   - configPath 是最终落盘路径
//   - tr 是已初始化的 i18n.Translator
//
// 返回：
//   - written: 是否真的把 config 写盘（用户可能在确认环节选 n）
//   - result:  收集到的偏好（即使没写盘也返回，便于把 key 提示打出来）
//   - err:     I/O 类失败
func runWizard(env *wizardEnv, configPath string) (written bool, result wizardResult, err error) {
	tr := env.tr

	env.println()
	env.println(tr.T("wizard.welcome_title"))
	env.println(strings.Repeat("─", 32))
	env.println(tr.T("wizard.welcome_body", map[string]any{"Path": configPath}))
	env.println()

	if env.interactive {
		return runInteractiveWizard(env, configPath)
	}
	return runScriptedWizard(env, configPath)
}

func runInteractiveWizard(env *wizardEnv, configPath string) (written bool, result wizardResult, err error) {
	result.Provider = agent.ProviderAnthropic
	result.Scenario = defaultScenario()
	result.Lang = ""
	confirm := true

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择 LLM provider").
				Description("用 ↑/↓ 浏览，Enter 确认；不需要记 provider 拼写。").
				Options(providerSelectOptions()...).
				Value(&result.Provider),
			huh.NewInput().
				Title("API key").
				Description("可跳过；key 不会写入配置文件，稍后用环境变量提供也可以。").
				Placeholder("留空跳过").
				EchoMode(huh.EchoModePassword).
				Value(&result.APIKey),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择剧本").
				Description("只展示玩家可见信息；剧本真相不会暴露。").
				Options(scenarioSelectOptions()...).
				Value(&result.Scenario),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择界面语言").
				Options(
					huh.NewOption("自动检测（推荐）", ""),
					huh.NewOption("简体中文", "zh-CN"),
					huh.NewOption("English", "en"),
				).
				Value(&result.Lang),
			huh.NewConfirm().
				Title("写入配置文件？").
				Description(configPath).
				Affirmative("写入").
				Negative("取消").
				Value(&confirm),
		),
	).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithOutput(env.out).
		WithInput(os.Stdin).
		WithAccessible(os.Getenv("WHISPERER_ACCESSIBLE") != "")

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			env.println(env.tr.T("wizard.aborted"))
			return false, result, nil
		}
		return false, result, err
	}
	spec := agent.ProviderInfo(result.Provider)
	result.APIKeyEnv = spec.EnvKey
	result.APIKey = strings.TrimSpace(result.APIKey)
	if !confirm {
		env.println(env.tr.T("wizard.aborted"))
		return false, result, nil
	}
	if result.APIKey == "" {
		env.println(env.tr.T("wizard.api_key_blank_warning", map[string]any{"EnvVar": result.APIKeyEnv}))
	}
	if err := writeWizardConfig(configPath, result); err != nil {
		return false, result, err
	}
	env.println(env.tr.T("wizard.written", map[string]any{"Path": configPath}))
	if result.APIKey == "" {
		env.println(env.tr.T("wizard.api_key_hint"))
	}
	return true, result, nil
}

func runScriptedWizard(env *wizardEnv, configPath string) (written bool, result wizardResult, err error) {
	tr := env.tr

	for {
		env.println(providerMenuText(tr.T("wizard.choose_provider")))
		_, _ = fmt.Fprint(env.out, "> ")
		choice, err := env.readLine()
		if err != nil {
			return false, result, err
		}
		spec, ok := parseProviderChoice(choice)
		if !ok {
			env.println(tr.T("wizard.choose_provider_invalid", map[string]any{"Choice": strings.TrimSpace(choice)}))
			continue
		}
		result.Provider = spec.Name
		result.APIKeyEnv = spec.EnvKey
		break
	}
	env.println()

	env.println(tr.T("wizard.api_key_prompt", map[string]any{"EnvVar": result.APIKeyEnv}))
	_, _ = fmt.Fprint(env.out, "> ")
	keyLine, err := env.readLine()
	if err != nil {
		return false, result, err
	}
	result.APIKey = strings.TrimSpace(keyLine)
	if result.APIKey == "" {
		env.println(tr.T("wizard.api_key_blank_warning", map[string]any{"EnvVar": result.APIKeyEnv}))
	}
	env.println()

	for {
		env.println(scenarioMenuText(tr.T("wizard.choose_scenario")))
		_, _ = fmt.Fprint(env.out, "> ")
		choice, err := env.readLine()
		if err != nil {
			return false, result, err
		}
		if id, ok := parseScenarioChoice(choice); ok {
			result.Scenario = id
			break
		}
		env.println(tr.T("wizard.choose_scenario_invalid", map[string]any{
			"Choice": strings.TrimSpace(choice),
		}))
	}
	env.println()

	env.println(languageMenuText(tr.T("wizard.choose_lang")))
	_, _ = fmt.Fprint(env.out, "> ")
	langChoice, err := env.readLine()
	if err != nil {
		return false, result, err
	}
	result.Lang = parseLanguageChoice(langChoice)
	env.println()

	env.println(tr.T("wizard.confirm", map[string]any{"Path": configPath}))
	_, _ = fmt.Fprint(env.out, "> ")
	yn, err := env.readLine()
	if err != nil {
		return false, result, err
	}
	yn = strings.ToLower(strings.TrimSpace(yn))
	if yn == "n" || yn == "no" {
		env.println(tr.T("wizard.aborted"))
		return false, result, nil
	}

	if err := writeWizardConfig(configPath, result); err != nil {
		return false, result, err
	}
	env.println(tr.T("wizard.written", map[string]any{"Path": configPath}))
	if result.APIKey == "" {
		env.println(tr.T("wizard.api_key_hint"))
	}
	return true, result, nil
}

func providerSelectOptions() []huh.Option[string] {
	providers := []string{
		agent.ProviderAnthropic,
		agent.ProviderOpenAI,
		agent.ProviderGrok,
		agent.ProviderGemini,
		agent.ProviderOpenRouter,
	}
	options := make([]huh.Option[string], 0, len(providers))
	for _, provider := range providers {
		spec := agent.ProviderInfo(provider)
		options = append(options, huh.NewOption(providerLabel(spec), spec.Name).Selected(provider == agent.ProviderAnthropic))
	}
	return options
}

func providerLabel(spec agent.ProviderSpec) string {
	return fmt.Sprintf("%s · %s", spec.Name, spec.EnvKey)
}

func scenarioSelectOptions() []huh.Option[string] {
	infos := bundledScenarios()
	options := make([]huh.Option[string], 0, len(infos))
	for i, info := range infos {
		label := fmt.Sprintf("%s · %d 地点 · %d 人物 · %d 线索",
			info.Title, info.Locations, info.NPCs, info.Clues)
		if info.Variants > 0 {
			label += fmt.Sprintf(" · %d 变体", info.Variants)
		}
		options = append(options, huh.NewOption(label, info.ID).Selected(i == 0))
	}
	return options
}

func bundledScenarios() []scenario.BundledInfo {
	infos, err := scenario.ListBundled()
	if err == nil && len(infos) > 0 {
		return infos
	}
	return []scenario.BundledInfo{{ID: "fog_harbor", Title: "雾港疑案"}}
}

func defaultScenario() string {
	return bundledScenarios()[0].ID
}

func providerMenuText(title string) string {
	lines := []string{title}
	for i, opt := range providerSelectOptions() {
		lines = append(lines, fmt.Sprintf("  %d. %s", i+1, opt.Key))
	}
	return strings.Join(lines, "\n")
}

func parseProviderChoice(choice string) (agent.ProviderSpec, bool) {
	choice = strings.TrimSpace(choice)
	if idx, err := strconv.Atoi(choice); err == nil {
		options := providerSelectOptions()
		if idx >= 1 && idx <= len(options) {
			return agent.ProviderInfo(options[idx-1].Value), true
		}
		return agent.ProviderSpec{}, false
	}
	return agent.ParseProvider(choice)
}

func scenarioMenuText(title string) string {
	lines := []string{title}
	for i, info := range bundledScenarios() {
		label := info.Title
		if info.Locations > 0 {
			label += fmt.Sprintf(" · %d 地点 · %d 人物 · %d 线索", info.Locations, info.NPCs, info.Clues)
		}
		lines = append(lines, fmt.Sprintf("  %d. %s", i+1, label))
	}
	return strings.Join(lines, "\n")
}

func parseScenarioChoice(choice string) (string, bool) {
	choice = strings.TrimSpace(choice)
	infos := bundledScenarios()
	if choice == "" {
		return infos[0].ID, true
	}
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx >= 1 && idx <= len(infos) {
			return infos[idx-1].ID, true
		}
		return "", false
	}
	for _, info := range infos {
		if choice == info.ID {
			return info.ID, true
		}
	}
	return "", false
}

func languageMenuText(title string) string {
	return strings.Join([]string{
		title,
		"  1. 自动检测（推荐）",
		"  2. 简体中文",
		"  3. English",
	}, "\n")
}

func parseLanguageChoice(choice string) string {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "", "1", "auto":
		return ""
	case "2", "zh-cn", "zh", "cn":
		return "zh-CN"
	case "3", "en":
		return "en"
	default:
		return strings.TrimSpace(choice)
	}
}

// readLine 读 stdin 一行；EOF 视作空字符串（让向导在 piped 输入下也能退出干净）。
func (e *wizardEnv) readLine() (string, error) {
	line, err := e.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// printf / println 把 fmt.Fprint(ln) 的 error 返回值统一吞掉。终端 UI 写入失败
// 没有合理的恢复路径——继续走流程让 readLine 兜底比反复检查噪音少。
func (e *wizardEnv) println(args ...any) {
	_, _ = fmt.Fprintln(e.out, args...)
}

// writeWizardConfig 把向导收集的偏好写到 path（TOML 格式）。
//
// 注意：API key 严禁写盘——只通过环境变量提供。这里只写 provider / scenario /
// lang 三项，其他保持注释 / 空，让用户自行编辑 example-config.toml 风格。
func writeWizardConfig(path string, r wizardResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	var b strings.Builder
	b.WriteString("# Generated by `whisperer init`. Edit freely.\n")
	b.WriteString("# API keys are intentionally NOT stored here — use the environment\n")
	b.WriteString("# variable shown when you ran the wizard, or `--api-key`.\n\n")
	fmt.Fprintf(&b, "provider = %q\n", r.Provider)
	fmt.Fprintf(&b, "scenario = %q\n", r.Scenario)
	if r.Lang != "" {
		fmt.Fprintf(&b, "lang = %q\n", r.Lang)
	}
	// log defaults
	b.WriteString("log_format = \"text\"\n")
	b.WriteString("log_level  = \"info\"\n")
	b.WriteString("# See docs/example-config.toml for model / timeout knobs.\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// shouldRunWizard 决定是否在 main 启动时自动跑向导。规则：
//   - 用户传了 init 子命令 → 必跑
//   - configPath 文件存在 → 不跑
//   - stdin 不是 tty（脚本/重定向） → 不跑（避免阻塞 CI）
//   - 否则 → 跑
func shouldRunWizard(args []string, configPath string) bool {
	for _, a := range args {
		if a == "init" {
			return true
		}
	}
	if _, err := os.Stat(configPath); err == nil {
		return false
	}
	if !stdinIsTTY() {
		return false
	}
	return true
}

// stdinIsTTY 判断 stdin 是否连到了交互式终端。简单实现：检查 mode 是否含
// CharDevice。
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// resolveWizardConfigPath 返回向导要落盘的路径——优先用 --config flag 给的路径，
// 否则走 config.DefaultPath()。空时返回 "" 由调用方处理。
func resolveWizardConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return config.DefaultPath()
}
