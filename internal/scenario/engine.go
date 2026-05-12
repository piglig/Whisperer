package scenario

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// FiredEventType 是触发器 fired 状态在 events 表中的类型常量。
const FiredEventType = "trigger_fired"

// FiredTrigger 描述本次 Evaluate 触发的一项。
type FiredTrigger struct {
	ID      string
	Actions []Action
}

// Engine 把 Scenario 与 store / memory 绑定，提供初始化（Apply）与回合后求值。
type Engine struct {
	scenario *Scenario
	repo     *store.Repository
	memory   *memory.Memory
}

// New 构造 Engine。memory 可为 nil；为 nil 时 Apply 不会写入向量库。
func New(s *Scenario, repo *store.Repository, mem *memory.Memory) *Engine {
	return &Engine{scenario: s, repo: repo, memory: mem}
}

// Apply 把剧本数据落到 store（NPC / location / clue / item / start），并把
// NPC 档案与线索描述同步到 memory。重复调用幂等：所有写入都是 upsert。
func (e *Engine) Apply(ctx context.Context, saveID string) error {
	s := e.scenario

	// 1) 时间 / 位置
	tod := store.TimeMorning
	if s.Start.TimeOfDay != "" {
		tod = store.TimeOfDay(s.Start.TimeOfDay)
	}
	if err := e.repo.SetTimeOfDay(ctx, saveID, tod); err != nil {
		return fmt.Errorf("apply: set time: %w", err)
	}
	if err := e.repo.UpdateSaveProgress(ctx, saveID, s.Start.Location, 0); err != nil {
		return fmt.Errorf("apply: set start location: %w", err)
	}

	// 2) Locations
	for _, l := range s.Locations {
		conn, _ := json.Marshal(l.Connections)
		if err := e.repo.UpsertLocation(ctx, store.Location{
			ID:              l.ID,
			SaveID:          saveID,
			Name:            l.Name,
			Description:     l.Description,
			ParentID:        l.ParentID,
			ConnectionsJSON: string(conn),
		}); err != nil {
			return fmt.Errorf("apply: upsert location %s: %w", l.ID, err)
		}
	}
	// 标记 start.location 为已访问
	if err := e.repo.MarkLocationVisited(ctx, s.Start.Location); err != nil {
		return fmt.Errorf("apply: mark start visited: %w", err)
	}

	// 3) NPCs
	for _, n := range s.NPCs {
		know := "{}"
		if len(n.Knowledge) > 0 {
			b, _ := json.Marshal(n.Knowledge)
			know = string(b)
		}
		if err := e.repo.UpsertNPC(ctx, store.NPC{
			ID:               n.ID,
			SaveID:           saveID,
			Name:             n.Name,
			Personality:      n.Personality,
			KnowledgeJSON:    know,
			RelationToPlayer: n.RelationToPlayer,
			LocationID:       n.Location,
			Alive:            true,
		}); err != nil {
			return fmt.Errorf("apply: upsert npc %s: %w", n.ID, err)
		}
		if e.memory != nil {
			profile := fmt.Sprintf("Name: %s\nPersonality: %s\nKnowledge: %s\nRelationToPlayer: %d",
				n.Name, n.Personality, know, n.RelationToPlayer)
			_ = e.memory.UpsertNPCProfile(ctx, n.ID, profile, map[string]string{
				"npc_id": n.ID,
				"name":   n.Name,
			})
		}
	}

	// 4) Clues
	for _, c := range s.Clues {
		if err := e.repo.UpsertClue(ctx, store.Clue{
			ID:          c.ID,
			SaveID:      saveID,
			ScenarioID:  s.ID,
			Description: c.Description,
		}); err != nil {
			return fmt.Errorf("apply: upsert clue %s: %w", c.ID, err)
		}
		if e.memory != nil {
			_ = e.memory.UpsertClue(ctx, c.ID, c.Description, map[string]string{"clue_id": c.ID})
		}
	}

	// 5) Items
	for _, it := range s.Items {
		owner := store.OwnerType(it.OwnerType)
		if owner == "" {
			owner = store.OwnerNone
		}
		if err := e.repo.UpsertItem(ctx, store.Item{
			ID:             it.ID,
			SaveID:         saveID,
			Name:           it.Name,
			Description:    it.Description,
			OwnerType:      owner,
			OwnerID:        it.OwnerID,
			PropertiesJSON: "{}",
		}); err != nil {
			return fmt.Errorf("apply: upsert item %s: %w", it.ID, err)
		}
	}

	return nil
}

// Evaluate 在每回合结束后调用：扫描所有未触发条件，命中则执行动作并写 fired 事件。
func (e *Engine) Evaluate(ctx context.Context, saveID string) ([]FiredTrigger, error) {
	fired, err := e.firedSet(ctx, saveID)
	if err != nil {
		return nil, err
	}

	view, err := e.snapshot(ctx, saveID, fired)
	if err != nil {
		return nil, err
	}

	out := []FiredTrigger{}
	for _, t := range e.scenario.Triggers {
		if fired[t.ID] {
			continue
		}
		ok, err := evalCondition(t.When, view)
		if err != nil {
			return nil, fmt.Errorf("trigger %s: %w", t.ID, err)
		}
		if !ok {
			continue
		}
		if err := e.executeActions(ctx, saveID, view.Save.TurnCount, t.Then); err != nil {
			return nil, fmt.Errorf("trigger %s: actions: %w", t.ID, err)
		}
		// 写 fired 事件并刷新 fired 集合 / view（让后续 trigger 能看到）。
		_, err = e.repo.AppendEvent(ctx, store.Event{
			SaveID:      saveID,
			Turn:        view.Save.TurnCount,
			Type:        FiredEventType,
			Description: t.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("trigger %s: write fired event: %w", t.ID, err)
		}
		fired[t.ID] = true
		view.Fired[t.ID] = true
		out = append(out, FiredTrigger{ID: t.ID, Actions: t.Then})
	}
	return out, nil
}

// CheckEndings 返回首个 condition 命中的 ending；没有则 nil。
func (e *Engine) CheckEndings(ctx context.Context, saveID string) (*Ending, error) {
	fired, err := e.firedSet(ctx, saveID)
	if err != nil {
		return nil, err
	}
	view, err := e.snapshot(ctx, saveID, fired)
	if err != nil {
		return nil, err
	}
	for i := range e.scenario.Endings {
		ok, err := evalCondition(e.scenario.Endings[i].When, view)
		if err != nil {
			return nil, fmt.Errorf("ending %s: %w", e.scenario.Endings[i].ID, err)
		}
		if ok {
			return &e.scenario.Endings[i], nil
		}
	}
	return nil, nil
}

// stateView 是触发器条件求值时使用的"快照"。
type stateView struct {
	Save           store.Save
	VisitedLocs    map[string]bool
	FoundClues     map[string]bool
	DeadNPCs       map[string]bool
	NPCRelations   map[string]int
	Fired          map[string]bool
}

func (e *Engine) firedSet(ctx context.Context, saveID string) (map[string]bool, error) {
	events, err := e.repo.ListEvents(ctx, saveID, 0, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, ev := range events {
		if string(ev.Type) == FiredEventType {
			out[ev.Description] = true
		}
	}
	return out, nil
}

func (e *Engine) snapshot(ctx context.Context, saveID string, fired map[string]bool) (stateView, error) {
	v := stateView{
		VisitedLocs:  map[string]bool{},
		FoundClues:   map[string]bool{},
		DeadNPCs:     map[string]bool{},
		NPCRelations: map[string]int{},
		Fired:        fired,
	}

	sv, err := e.repo.GetSave(ctx, saveID)
	if err != nil {
		return v, err
	}
	v.Save = sv

	for _, l := range e.scenario.Locations {
		loc, err := e.repo.GetLocation(ctx, l.ID)
		if err == nil && loc.Visited {
			v.VisitedLocs[l.ID] = true
		}
	}
	clues, err := e.repo.ListFoundClues(ctx, saveID)
	if err != nil {
		return v, err
	}
	for _, c := range clues {
		v.FoundClues[c.ID] = true
	}
	for _, n := range e.scenario.NPCs {
		npc, err := e.repo.GetNPC(ctx, n.ID)
		if err != nil {
			continue
		}
		if !npc.Alive {
			v.DeadNPCs[n.ID] = true
		}
		v.NPCRelations[n.ID] = npc.RelationToPlayer
	}
	return v, nil
}

func evalCondition(c Condition, v stateView) (bool, error) {
	switch {
	case len(c.All) > 0:
		for _, sub := range c.All {
			ok, err := evalCondition(sub, v)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	case len(c.Any) > 0:
		for _, sub := range c.Any {
			ok, err := evalCondition(sub, v)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case c.Not != nil:
		ok, err := evalCondition(*c.Not, v)
		if err != nil {
			return false, err
		}
		return !ok, nil
	case c.LocationVisited != "":
		return v.VisitedLocs[c.LocationVisited], nil
	case c.ClueFound != "":
		return v.FoundClues[c.ClueFound], nil
	case c.NPCDead != "":
		return v.DeadNPCs[c.NPCDead], nil
	case c.NPCRelationLT != nil:
		return v.NPCRelations[c.NPCRelationLT.NPC] < c.NPCRelationLT.Value, nil
	case c.NPCRelationGT != nil:
		return v.NPCRelations[c.NPCRelationGT.NPC] > c.NPCRelationGT.Value, nil
	case c.TimeOfDay != "":
		return string(v.Save.TimeOfDay) == c.TimeOfDay, nil
	case c.TurnGE > 0:
		return v.Save.TurnCount >= c.TurnGE, nil
	case c.TriggerFired != "":
		return v.Fired[c.TriggerFired], nil
	}
	return false, fmt.Errorf("unrecognized condition")
}

func (e *Engine) executeActions(ctx context.Context, saveID string, turn int, actions []Action) error {
	for i, a := range actions {
		if err := e.executeAction(ctx, saveID, turn, a); err != nil {
			return fmt.Errorf("action[%d]: %w", i, err)
		}
	}
	return nil
}

func (e *Engine) executeAction(ctx context.Context, saveID string, turn int, a Action) error {
	switch {
	case a.AddEvent != nil:
		_, err := e.repo.AppendEvent(ctx, store.Event{
			SaveID:      saveID,
			Turn:        turn,
			Type:        store.EventType(a.AddEvent.Type),
			Description: a.AddEvent.Description,
		})
		return err
	case a.MarkClueFound != nil:
		return e.repo.MarkClueFound(ctx, a.MarkClueFound.ClueID, a.MarkClueFound.LocationID, turn)
	case a.UpdateNPCRelation != nil:
		return e.repo.UpdateNPCRelation(ctx, a.UpdateNPCRelation.NPCID, a.UpdateNPCRelation.Delta)
	case a.KillNPC != "":
		return e.repo.KillNPC(ctx, a.KillNPC)
	case a.AdvanceTime > 0:
		return e.advanceTime(ctx, saveID, a.AdvanceTime)
	}
	return fmt.Errorf("unrecognized action")
}

func (e *Engine) advanceTime(ctx context.Context, saveID string, stages int) error {
	sv, err := e.repo.GetSave(ctx, saveID)
	if err != nil {
		return err
	}
	t := sv.TimeOfDay
	for range stages {
		t = t.Next()
	}
	return e.repo.SetTimeOfDay(ctx, saveID, t)
}
