package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type ActionGuardResult struct {
	Allowed          bool         `json:"allowed"`
	Reason           string       `json:"reason,omitempty"`
	Suggestions      []string     `json:"suggestions,omitempty"`
	NormalizedAction PlayerAction `json:"normalized_action"`
}

func guardPlayerAction(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, action PlayerAction) ActionGuardResult {
	result := ActionGuardResult{Allowed: true, NormalizedAction: action}
	if sourceResult := guardActionSource(ctx, repo, saveID, scn, result); !sourceResult.Allowed {
		return sourceResult
	}
	switch action.Kind {
	case IntentMove:
		return guardMoveAction(ctx, repo, saveID, scn, result)
	case IntentTalk:
		return guardTalkAction(ctx, repo, saveID, result)
	case IntentUseItem:
		return guardUseItemAction(ctx, repo, saveID, result)
	case IntentReport:
		return guardReportAction(ctx, repo, saveID, scn, result)
	default:
		return result
	}
}

func guardActionSource(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, result ActionGuardResult) ActionGuardResult {
	source := result.NormalizedAction.Source
	if source.Kind == "" {
		return result
	}
	switch source.Kind {
	case "lead":
		return guardLeadSource(ctx, repo, saveID, scn, result)
	case "dialogue_option":
		return guardDialogueOptionSource(ctx, repo, saveID, scn, result)
	case "item_action":
		return guardItemActionSource(ctx, repo, saveID, scn, result)
	case "fallback":
		return result
	default:
		return deny(result, "未知的剧本行动来源："+source.Kind)
	}
}

func guardLeadSource(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, result ActionGuardResult) ActionGuardResult {
	target, ok := firstTarget(result.NormalizedAction, "location")
	if !ok {
		return deny(result, "这个推荐调查行动缺少地点目标。")
	}
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取当前存档状态。")
	}
	if target.ID != sv.CurrentLocationID {
		return deny(result, "这个调查行动不属于当前地点。", "请从当前场景的可选行动里重新选择。")
	}
	loc, ok := scenarioLocation(scn, target.ID)
	if !ok {
		return deny(result, "剧本中找不到这个地点："+target.ID)
	}
	locID, idxText, ok := strings.Cut(result.NormalizedAction.Source.ID, ":")
	if !ok {
		return deny(result, "这个调查行动来源格式无效。")
	}
	if locID != target.ID {
		return deny(result, "这个调查行动来源和目标地点不一致。", "请重新选择当前显示的可选行动。")
	}
	idx, err := strconv.Atoi(idxText)
	if err != nil || idx < 0 || idx >= len(loc.Leads) {
		return deny(result, "这个调查行动已经不在当前地点可选项中。", "请重新选择当前显示的可选行动。")
	}
	return result
}

func guardDialogueOptionSource(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, result ActionGuardResult) ActionGuardResult {
	target, ok := firstTarget(result.NormalizedAction, "npc")
	if !ok {
		return deny(result, "这个对话行动缺少人物目标。")
	}
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取当前存档状态。")
	}
	npc, err := repo.GetNPC(ctx, target.ID)
	if err != nil {
		return deny(result, "这个人物目前不存在："+target.ID)
	}
	snpc, ok := scenarioNPC(scn, target.ID)
	if !ok {
		return deny(result, "剧本中找不到这个人物："+target.ID)
	}
	found, err := foundClueSet(ctx, repo, saveID)
	if err != nil {
		return deny(result, "无法读取已发现线索。")
	}
	stage := scenario.NormalizeStage(scn, sv.Stage)
	for _, opt := range scenario.DialogueOptionsFor(snpc, stage, found) {
		if opt.ID == result.NormalizedAction.Source.ID {
			return result
		}
	}
	return deny(result,
		fmt.Sprintf("%s 的这个对话选项当前不可用。", npc.Name),
		"请根据当前阶段和已发现线索重新选择可用对话。",
	)
}

func guardItemActionSource(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, result ActionGuardResult) ActionGuardResult {
	target, ok := firstTarget(result.NormalizedAction, "item")
	if !ok {
		return deny(result, "这个物品行动缺少物品目标。")
	}
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取当前存档状态。")
	}
	item, err := repo.GetItem(ctx, target.ID)
	if err != nil {
		return deny(result, "这个物品目前不存在："+target.ID)
	}
	sitem, ok := scenarioItem(scn, target.ID)
	if !ok {
		return deny(result, "剧本中找不到这个物品："+target.ID)
	}
	found, err := foundClueSet(ctx, repo, saveID)
	if err != nil {
		return deny(result, "无法读取已发现线索。")
	}
	stage := scenario.NormalizeStage(scn, sv.Stage)
	for _, action := range scenario.ItemActionsFor(sitem, item, stage, sv.CurrentLocationID, found) {
		if action.ID == result.NormalizedAction.Source.ID {
			return result
		}
	}
	return deny(result,
		fmt.Sprintf("%s 的这个物品行动当前不可用。", item.Name),
		"请确认阶段、地点、物品归属和线索条件后再尝试。",
	)
}

func guardMoveAction(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, result ActionGuardResult) ActionGuardResult {
	target, ok := firstTarget(result.NormalizedAction, "location")
	if !ok {
		return deny(result, "你想去哪里还不够明确。", "选择一个地点，或直接输入“去酒馆 / 去巡警所 / 去灯塔”。")
	}
	loc, err := repo.GetLocation(ctx, target.ID)
	if err != nil {
		return deny(result, "这个地点目前不存在："+target.ID, "从右侧案件卡的可见地点里选择下一步。")
	}
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取当前存档状态。")
	}
	if sv.CurrentLocationID == "" || sv.CurrentLocationID == loc.ID {
		return result
	}
	if !locationsConnected(scn, sv.CurrentLocationID, loc.ID) {
		current := sv.CurrentLocationID
		if curLoc, err := repo.GetLocation(ctx, sv.CurrentLocationID); err == nil {
			current = curLoc.Name
		}
		return deny(result,
			fmt.Sprintf("从%s不能直接前往%s。", current, loc.Name),
			"先移动到相邻地点，或调查当前地点寻找新的路径。",
		)
	}
	return result
}

func guardTalkAction(ctx context.Context, repo *store.Repository, saveID string, result ActionGuardResult) ActionGuardResult {
	target, ok := firstTarget(result.NormalizedAction, "npc")
	if !ok {
		return deny(result, "你想和谁交谈还不够明确。", "选择在场人物，或输入“问范斯……”这类明确目标。")
	}
	npc, err := repo.GetNPC(ctx, target.ID)
	if err != nil {
		return deny(result, "这个人物目前不存在："+target.ID, "从右侧人物卡选择在场人物。")
	}
	if !npc.Alive {
		return deny(result, npc.Name+"已经无法交谈。")
	}
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取当前存档状态。")
	}
	if npc.LocationID != "" && sv.CurrentLocationID != "" && npc.LocationID != sv.CurrentLocationID {
		place := npc.LocationID
		if loc, err := repo.GetLocation(ctx, npc.LocationID); err == nil {
			place = loc.Name
		}
		return deny(result,
			fmt.Sprintf("%s不在当前地点。", npc.Name),
			"先前往"+place+"，再和"+npc.Name+"交谈。",
		)
	}
	return result
}

func guardUseItemAction(ctx context.Context, repo *store.Repository, saveID string, result ActionGuardResult) ActionGuardResult {
	target, ok := firstTarget(result.NormalizedAction, "item")
	if !ok {
		return deny(result, "你想使用哪个物品还不够明确。", "选择背包或当前地点里的物品。")
	}
	item, err := repo.GetItem(ctx, target.ID)
	if err != nil {
		return deny(result, "这个物品目前不存在："+target.ID, "从背包或当前地点物品中选择。")
	}
	if item.Destroyed {
		return deny(result, item.Name+"已经损毁，不能再使用。")
	}
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取当前存档状态。")
	}
	if itemAccessible(ctx, repo, sv.CurrentLocationID, item) {
		return result
	}
	return deny(result,
		item.Name+"不在你能直接使用的位置。",
		"先找到或取得"+item.Name+"，再尝试使用。",
	)
}

func guardReportAction(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, result ActionGuardResult) ActionGuardResult {
	if scn == nil || len(scn.KeyClues) == 0 {
		return result
	}
	found, err := repo.ListFoundClues(ctx, saveID)
	if err != nil {
		return deny(result, "无法读取已发现线索。")
	}
	foundSet := map[string]bool{}
	for _, clue := range found {
		foundSet[clue.ID] = true
	}
	missing := []string{}
	for _, id := range scn.KeyClues {
		if !foundSet[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return result
	}
	return deny(result,
		fmt.Sprintf("现在证据还不足，至少还缺 %d 条关键线索。", len(missing)),
		"继续调查目标、风险和人物关系；结案前尽量补齐关键证据。",
	)
}

func itemAccessible(ctx context.Context, repo *store.Repository, currentLocationID string, item store.Item) bool {
	switch item.OwnerType {
	case store.OwnerInvestigator:
		return true
	case store.OwnerLocation:
		return item.OwnerID == "" || item.OwnerID == currentLocationID
	case store.OwnerNPC:
		npc, err := repo.GetNPC(ctx, item.OwnerID)
		return err == nil && npc.Alive && npc.LocationID == currentLocationID
	case store.OwnerNone:
		return false
	default:
		return false
	}
}

func locationsConnected(scn *scenario.Scenario, from, to string) bool {
	if from == to {
		return true
	}
	if scn == nil {
		return true
	}
	for _, loc := range scn.Locations {
		if loc.ID != from {
			continue
		}
		if len(loc.Connections) == 0 {
			return true
		}
		for _, id := range loc.Connections {
			if id == to {
				return true
			}
		}
		return false
	}
	return true
}

func scenarioLocation(scn *scenario.Scenario, id string) (scenario.SLocation, bool) {
	if scn == nil {
		return scenario.SLocation{}, false
	}
	for _, loc := range scn.Locations {
		if loc.ID == id {
			return loc, true
		}
	}
	return scenario.SLocation{}, false
}

func scenarioNPC(scn *scenario.Scenario, id string) (scenario.SNPC, bool) {
	if scn == nil {
		return scenario.SNPC{}, false
	}
	for _, npc := range scn.NPCs {
		if npc.ID == id {
			return npc, true
		}
	}
	return scenario.SNPC{}, false
}

func scenarioItem(scn *scenario.Scenario, id string) (scenario.SItem, bool) {
	if scn == nil {
		return scenario.SItem{}, false
	}
	for _, item := range scn.Items {
		if item.ID == id {
			return item, true
		}
	}
	return scenario.SItem{}, false
}

func foundClueSet(ctx context.Context, repo *store.Repository, saveID string) (map[string]bool, error) {
	clues, err := repo.ListFoundClues(ctx, saveID)
	if err != nil {
		return nil, err
	}
	found := make(map[string]bool, len(clues))
	for _, clue := range clues {
		found[clue.ID] = true
	}
	return found, nil
}

func firstTarget(action PlayerAction, kind string) (ActionTarget, bool) {
	for _, target := range action.Targets {
		if target.Kind == kind && target.ID != "" {
			return target, true
		}
	}
	return ActionTarget{}, false
}

func deny(result ActionGuardResult, reason string, suggestions ...string) ActionGuardResult {
	result.Allowed = false
	result.Reason = reason
	result.Suggestions = suggestions
	return result
}

func guardNarrative(result ActionGuardResult) string {
	if result.Allowed {
		return ""
	}
	lines := []string{"行动未执行：" + result.Reason}
	for _, suggestion := range result.Suggestions {
		if suggestion != "" {
			lines = append(lines, "建议："+suggestion)
		}
	}
	return joinLines(lines)
}

func buildGuardDecision(action PlayerAction, result ActionGuardResult) TurnDecision {
	return TurnDecision{
		Intent:      action.Kind,
		PlayerInput: action.Raw,
		Action:      result.NormalizedAction,
		Checks: []DecisionCheck{{
			Code:    "action_guard",
			Passed:  result.Allowed,
			Message: result.Reason,
		}},
	}
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
