package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MetaState 是跨周目的轻量持久化——记录玩家在过去几局看过哪些 variant、走过哪些
// ending、揭开过哪些线索。GM 提示在新一局开始时把这份"先验"作为额外段落注入，
// 让 NPC 出现"似曾相识"的暗示，但不剧透具体真相。
//
// 文件落在 runs/meta.json（gitignored）。Load 不强制文件存在，新玩家直接拿到空结构。
type MetaState struct {
	CompletedVariants []string `json:"completed_variants"`
	CompletedEndings  []string `json:"completed_endings"`
	DiscoveredTruths  []string `json:"discovered_truths"`
	PlayCount         int      `json:"play_count"`
}

// LoadMeta 从 path 读取 meta；文件缺失视为空状态（不报错）。
// 解析失败返回错误，避免被损坏的元数据误导后续 prompt。
func LoadMeta(path string) (*MetaState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MetaState{}, nil
		}
		return nil, fmt.Errorf("meta: read %s: %w", path, err)
	}
	var m MetaState
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("meta: parse %s: %w", path, err)
	}
	return &m, nil
}

// SaveMeta 把 m 写到 path。父目录不存在时会被创建。原子性：先写 .tmp 再 rename。
func SaveMeta(path string, m *MetaState) error {
	if m == nil {
		return errors.New("meta: nil state")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("meta: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("meta: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("meta: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("meta: rename: %w", err)
	}
	return nil
}

// MarkCompletion 把一次通关的 variant + ending + 已揭开的关键真相记入 meta。
// 已存在的条目会被去重；play_count 始终 +1。
func (m *MetaState) MarkCompletion(variantID, endingID string, truthsRevealed []string) {
	m.PlayCount++
	if variantID != "" {
		m.CompletedVariants = uniqueAppend(m.CompletedVariants, variantID)
	}
	if endingID != "" {
		m.CompletedEndings = uniqueAppend(m.CompletedEndings, endingID)
	}
	for _, t := range truthsRevealed {
		if t != "" {
			m.DiscoveredTruths = uniqueAppend(m.DiscoveredTruths, t)
		}
	}
	sort.Strings(m.CompletedVariants)
	sort.Strings(m.CompletedEndings)
	sort.Strings(m.DiscoveredTruths)
}

// RenderForGM 返回一段 markdown 文本，注入到 GM system prompt 的"玩家先验"段。
// GM 被严格指示"知道但不剧透"，仅允许在 NPC 反应中产生"似曾相识"的暗示。
//
// 输出空字符串当且仅当 meta 完全为空（首次游玩）。
func (m *MetaState) RenderForGM() string {
	if m == nil || (m.PlayCount == 0 && len(m.CompletedVariants) == 0) {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "玩家累计游玩次数：%d。\n", m.PlayCount)
	if len(m.CompletedVariants) > 0 {
		fmt.Fprintf(&b, "已通关 variant：%s。\n", strings.Join(m.CompletedVariants, ", "))
	}
	if len(m.CompletedEndings) > 0 {
		fmt.Fprintf(&b, "已见结局：%s。\n", strings.Join(m.CompletedEndings, ", "))
	}
	if len(m.DiscoveredTruths) > 0 {
		fmt.Fprintf(&b, "曾揭开的关键真相：%s。\n", strings.Join(m.DiscoveredTruths, ", "))
	}
	b.WriteString("\n规则：你知道这位玩家「已活过几次」，但绝不可剧透本局真凶或具体线索。仅允许在以下情况让 NPC 出现「似曾相识」的反应：\n")
	b.WriteString("- 教士/老渔民/守塔人等长居者：偶尔愣神 1 拍、半句「我好像见过你」、做完某个动作后下意识看你；\n")
	b.WriteString("- 结局页可在元叙述中提示通关进度（「你已活过 N 次」）；\n")
	b.WriteString("- 不可让 NPC 直接说出上局真相，也不可让 NPC 主动配合（关系阈值仍按本局推动）。\n")
	return b.String()
}

func uniqueAppend(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
