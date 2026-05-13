// Package i18n 给 Whisperer 提供国际化字符串查找。
//
// 设计取舍：
//   - 选 nicksnyder/go-i18n/v2：行业标准，支持复数 / 性别 / message context；
//     比手写 map 多一些语义但同样适合静态翻译表。
//   - 翻译目录走 TOML（与 config / scenario 风格一致），embed 进二进制——发布
//     单文件就携带所有语言。
//   - 自动检测 $LANG / $LC_ALL / $LC_MESSAGES（标准 POSIX 顺序）；未识别 → zh-CN。
//   - --lang flag 永远胜过环境变量。
//
// 使用：
//
//	t, err := i18n.New("")              // 空 → 自动检测
//	t, err := i18n.New("en")            // 显式
//	greeting := t.T("wizard.welcome")   // 简单查
//	msg := t.T("error.api_key_invalid", map[string]any{"Provider": "anthropic"})
package i18n

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localesFS embed.FS

// supportedLocales 是 (BCP47 tag → 文件名) 的映射。新增一种语言 = 在 locales/
// 下加 <file>.toml + 在这里 register。
//
// 注意 language.SimplifiedChinese.String() 是 "zh-Hans"，不是 "zh-CN"——所以
// 这里把 tag 与文件名解耦，避免 tag.String() 与磁盘文件名不一致。
var supportedLocales = []localeFile{
	{tag: language.SimplifiedChinese, file: "zh-CN.toml"},
	{tag: language.English, file: "en.toml"},
}

type localeFile struct {
	tag  language.Tag
	file string
}

// Translator 是面向调用方的翻译入口。线程安全（go-i18n localizer 内部加锁）。
type Translator struct {
	bundle    *goi18n.Bundle
	localizer *goi18n.Localizer
	chosen    language.Tag
}

// New 构造 Translator。lang 取值优先级：
//   - 非空（如 "en" / "zh-CN" / 任何 BCP 47 tag）→ 直接用
//   - 空 → DetectLang() 从环境变量推断
//
// 解析失败时回退到 zh-CN，不返回错误（i18n 不应阻塞主流程）。
func New(lang string) (*Translator, error) {
	if lang == "" {
		lang = DetectLang()
	}

	bundle := goi18n.NewBundle(language.SimplifiedChinese)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	for _, loc := range supportedLocales {
		path := "locales/" + loc.file
		raw, err := localesFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("i18n: load %s: %w", path, err)
		}
		if _, err := bundle.ParseMessageFileBytes(raw, path); err != nil {
			return nil, fmt.Errorf("i18n: parse %s: %w", path, err)
		}
	}

	localizer := goi18n.NewLocalizer(bundle, lang, "zh-CN")
	chosen := language.SimplifiedChinese
	if t, err := language.Parse(lang); err == nil {
		// Localizer 内部会做 best-match；这里记录用户传入的 tag 用于诊断。
		chosen = t
	}

	return &Translator{bundle: bundle, localizer: localizer, chosen: chosen}, nil
}

// T 查询 key 对应的翻译。data 可为 nil 或 map[string]any 用作模板渲染：
//
//	t.T("error.api_key_invalid", map[string]any{"Provider": "openrouter"})
//
// 找不到 key 时返回 "[missing: <key>]"——不返回错误，避免散布 nil-check 到 UI 路径。
func (t *Translator) T(key string, data ...map[string]any) string {
	if t == nil {
		return "[i18n nil: " + key + "]"
	}
	cfg := &goi18n.LocalizeConfig{MessageID: key}
	if len(data) > 0 {
		cfg.TemplateData = data[0]
	}
	out, err := t.localizer.Localize(cfg)
	if err != nil {
		var notFound *goi18n.MessageNotFoundErr
		if errors.As(err, &notFound) {
			return fmt.Sprintf("[missing: %s]", key)
		}
		return fmt.Sprintf("[i18n error: %s: %v]", key, err)
	}
	return out
}

// Lang 返回 New 解析到的语言 tag（便于 --version 输出）。
func (t *Translator) Lang() string {
	if t == nil {
		return "zh-CN"
	}
	return t.chosen.String()
}

// DetectLang 按 POSIX 顺序读 LC_ALL → LC_MESSAGES → LANG，提取语言部分；
// 无法解析时返回 "zh-CN"。
//
// 输入示例 → 输出：
//   - "en_US.UTF-8"      → "en-US"
//   - "zh_CN.UTF-8"      → "zh-CN"
//   - "C" / "POSIX" / "" → "zh-CN"
func DetectLang() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		// "en_US.UTF-8" → "en_US"
		if dot := strings.IndexByte(v, '.'); dot > 0 {
			v = v[:dot]
		}
		// "@modifier" 后缀
		if at := strings.IndexByte(v, '@'); at > 0 {
			v = v[:at]
		}
		// POSIX 风格 "zh_CN" → BCP47 "zh-CN"
		v = strings.Replace(v, "_", "-", 1)
		if _, err := language.Parse(v); err == nil {
			return v
		}
	}
	return "zh-CN"
}
