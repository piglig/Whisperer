package scenario

import (
	"context"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

// DriftStatus 是 drift detector 的判定结果。
type DriftStatus int

const (
	DriftOK DriftStatus = iota
	DriftSoft
	DriftHard
)

func (d DriftStatus) String() string {
	switch d {
	case DriftOK:
		return "ok"
	case DriftSoft:
		return "soft"
	case DriftHard:
		return "hard"
	}
	return "unknown"
}

// DefaultSoftThreshold / DefaultHardThreshold 控制偏离阈值；与需求文档对齐。
const (
	DefaultSoftThreshold = 2
	DefaultHardThreshold = 3
)

// Detector 评估玩家是否偏离主线。
type Detector struct {
	scenario     *Scenario
	repo         *store.Repository
	softThresh   int
	hardThresh   int
}

// NewDetector 构造 detector。
func NewDetector(s *Scenario, repo *store.Repository) *Detector {
	return &Detector{
		scenario:   s,
		repo:       repo,
		softThresh: DefaultSoftThreshold,
		hardThresh: DefaultHardThreshold,
	}
}

// SetThresholds 用于测试或调参。
func (d *Detector) SetThresholds(soft, hard int) {
	d.softThresh = soft
	d.hardThresh = hard
}

// Tick 计算从最近一次"主线推进"到 currentTurn 的间隔回合数，依阈值返回状态。
//
// 主线推进定义（与需求文档 §5.5 与 spec 05 对齐）：满足任一即视为推进
//   - events 中存在 type=trigger_fired
//   - events 中存在 type=scenario（剧本注入的关键事件）
//   - events 中存在 description 含 "clue_found:" 前缀的 narrative（mark_clue_found 时
//     由 orchestrator 写入，本里程碑约定）
//   - 当前 turn 内 saves.current_location_id 切到了未访问 location（由调用方在
//     transition 时再加 narrative 提示，本判定按"已访问 locations 数量是否新增"）
//
// 简化实现：逐回合扫 events，找到最大的 progress turn；不足则 DriftSoft / Hard。
func (d *Detector) Tick(ctx context.Context, saveID string, currentTurn int) (DriftStatus, error) {
	events, err := d.repo.ListEvents(ctx, saveID, 0, 0)
	if err != nil {
		return DriftOK, err
	}
	lastProgress := 0
	for _, ev := range events {
		if isProgressEvent(ev) && ev.Turn > lastProgress {
			lastProgress = ev.Turn
		}
	}
	gap := currentTurn - lastProgress
	switch {
	case gap >= d.hardThresh:
		return DriftHard, nil
	case gap >= d.softThresh:
		return DriftSoft, nil
	default:
		return DriftOK, nil
	}
}

func isProgressEvent(ev store.Event) bool {
	switch string(ev.Type) {
	case FiredEventType, "scenario":
		return true
	}
	return strings.HasPrefix(ev.Description, "clue_found:")
}
