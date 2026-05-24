package director

import (
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func isFogHarbor(id string) bool {
	return id == "fog_harbor" || id == "fh"
}

func evaluateFogHarborAdvice(s Snapshot) Advice {
	found := s.FoundClues
	if found == nil {
		found = map[string]bool{}
	}
	fired := s.FiredTriggers
	if fired == nil {
		fired = map[string]bool{}
	}

	if !found["blood_letter"] || !found["tide_chart"] {
		return Advice{
			PrimaryObjective: "确认露西失踪的第一现场",
			Reason:           "你还缺少能把失踪案从传言变成案件的基础证据。",
			Urgency:          UrgencyNormal,
			SuggestedAction:  "先检查码头公告栏、退潮线和巡警所里的潮汐记录。",
		}
	}
	if shouldProtectAnna(s, found, fired) {
		urgency := UrgencyWarning
		if s.Turn >= 18 || threatSeverity(s.Threats, "anna_danger") >= 3 {
			urgency = UrgencyCritical
		}
		return Advice{
			PrimaryObjective: "保护安娜",
			Reason:           "你已经发现安娜被卷入筛选链条；继续拖延可能触发新的失踪。",
			Urgency:          urgency,
			SuggestedAction:  "去酒馆找安娜或玛丽莎，安排她离开范斯能接触到的地方。",
			Risk:             threatLabel(s.Threats, "anna_danger"),
		}
	}
	if !found["ledger"] {
		return Advice{
			PrimaryObjective: "拿到账册和出诊链条",
			Reason:           "码头与潮汐只能证明异常，账册能把人类共谋链串起来。",
			Urgency:          UrgencyNormal,
			SuggestedAction:  "夜里去钨灯酒馆，等玛丽莎打烊后追问出诊记录和药品订单。",
			BlockedBy:        []string{"ledger", "doctor_visits_log"},
		}
	}
	if !found["parish_record"] {
		return Advice{
			PrimaryObjective: "核对三十年失踪记录",
			Reason:           "账册只能证明当代协作，教区登记簿能证明这是长期契约。",
			Urgency:          UrgencyNormal,
			SuggestedAction:  "去圣安德鲁老教堂争取卡尔文神父信任，要求查看登记簿。",
			BlockedBy:        []string{"parish_record"},
		}
	}
	if needsReefEvidence(found, fired) {
		urgency := UrgencyWarning
		if fired["reef_cave_open"] {
			urgency = UrgencyCritical
		}
		return Advice{
			PrimaryObjective: "进入礁洞取得非人证据",
			Reason:           "你已经掌握人类层面的共谋，但还缺能解释献祭源头的核心证据。",
			Urgency:          urgency,
			SuggestedAction:  "暴风低潮后从灯塔铁梯下到礁洞，记录岩壁拓片和祭品室。",
			BlockedBy:        missing(found, "reef_carvings", "sacrifice_chamber"),
			Risk:             threatLabel(s.Threats, "reef_window"),
		}
	}
	if !fired["culprit_confronted"] {
		return Advice{
			PrimaryObjective: "对峙当代执行者",
			Reason:           "关键证据已经足够，现在需要让执行链条暴露并进入结局收束。",
			Urgency:          UrgencyCritical,
			SuggestedAction:  "带着礁洞拓片、账册和登记簿去对峙当前嫌疑人。",
			BlockedBy:        []string{"culprit_confronted"},
		}
	}
	return Advice{
		PrimaryObjective: "整理证据并离港",
		Reason:           "案件主线已经收束，剩下的是保存证据、保护证人并决定报道口径。",
		Urgency:          UrgencyNormal,
		SuggestedAction:  "把账册、拓片、登记簿和证人证言整理成完整报道。",
	}
}

func fogHarborAdviceKeywords(advice Advice) []string {
	switch advice.PrimaryObjective {
	case "确认露西失踪的第一现场":
		return []string{"公告栏", "退潮", "潮汐", "巡警所", "海莲娜", "码头"}
	case "保护安娜":
		return []string{"安娜", "玛丽莎", "酒馆", "保护", "离开"}
	case "拿到账册和出诊链条":
		return []string{"账册", "出诊", "玛丽莎", "酒馆", "药品"}
	case "核对三十年失踪记录":
		return []string{"教堂", "神父", "登记簿", "古约", "历年"}
	case "进入礁洞取得非人证据":
		return []string{"礁洞", "灯塔", "铁梯", "拓片", "祭品", "岩壁"}
	case "对峙当代执行者":
		return []string{"对峙", "范斯", "罗克", "神父", "证据", "拓片", "账册"}
	case "整理证据并离港":
		return []string{"证据", "报道", "离港", "整理", "笔记"}
	default:
		return nil
	}
}

func fogHarborActionKindScore(advice Advice, action orchestrator.SuggestedAction) int {
	switch action.Action.Kind {
	case orchestrator.IntentInvestigate:
		if advice.PrimaryObjective == "确认露西失踪的第一现场" || advice.PrimaryObjective == "进入礁洞取得非人证据" {
			return 3
		}
	case orchestrator.IntentTalk:
		if advice.PrimaryObjective == "保护安娜" ||
			advice.PrimaryObjective == "拿到账册和出诊链条" ||
			advice.PrimaryObjective == "核对三十年失踪记录" ||
			advice.PrimaryObjective == "对峙当代执行者" {
			return 3
		}
	case orchestrator.IntentUseItem:
		if len(advice.BlockedBy) > 0 {
			return 1
		}
	}
	return 0
}

func shouldProtectAnna(s Snapshot, found, fired map[string]bool) bool {
	if !found["anna_warning"] || fired["culprit_confronted"] {
		return false
	}
	if threatSeverity(s.Threats, "anna_danger") >= 1 {
		return true
	}
	return s.Turn >= 12
}

func needsReefEvidence(found, fired map[string]bool) bool {
	if found["reef_carvings"] && found["sacrifice_chamber"] {
		return false
	}
	return fired["reef_cave_open"] || found["ledger"] && found["parish_record"]
}

func missing(found map[string]bool, ids ...string) []string {
	var out []string
	for _, id := range ids {
		if !found[id] {
			out = append(out, id)
		}
	}
	return out
}

func threatSeverity(threats []scenario.ThreatStatus, id string) int {
	for _, threat := range threats {
		if threat.ID == id {
			return threat.Severity
		}
	}
	return 0
}

func threatLabel(threats []scenario.ThreatStatus, id string) string {
	for _, threat := range threats {
		if threat.ID == id {
			return strings.TrimSpace(threat.Name + "：" + threat.StateLabel)
		}
	}
	return ""
}
