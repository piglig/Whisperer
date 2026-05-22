package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zhuzhenwu/whisperer/internal/store/storesqlc"
)

// Repository 在主连接或事务上提供 CRUD。同一签名既可用于 *sql.DB（store.Repo()）
// 也可用于 *sql.Tx（store.RunTurn 内传入），背后由 querier 抽象。
type Repository struct {
	q querier
}

// nowMS 是 time.Now().UnixMilli() 的可注入版本。测试可临时替换以得到稳定时间。
var nowMS = func() int64 { return time.Now().UnixMilli() }

// boolToInt 简化 SQLite 0/1 ↔ Go bool 转换。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func sqlNullInt64(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(i), Valid: true}
}

func checkRowsAffected(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func mapRows[I, O any](rows []I, convert func(I) O) []O {
	out := make([]O, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
	}
	return out
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ============================================================================
// Save
// ============================================================================

func (r *Repository) CreateSave(ctx context.Context, s Save) error {
	now := nowMS()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.TimeOfDay == "" {
		s.TimeOfDay = TimeMorning
	}
	if s.Stage == "" {
		s.Stage = "opening"
	}
	return storesqlc.New(r.q).CreateSave(ctx, storesqlc.CreateSaveParams{
		ID:                s.ID,
		Name:              s.Name,
		ScenarioID:        s.ScenarioID,
		VariantID:         s.VariantID,
		Stage:             s.Stage,
		CurrentLocationID: sqlNullString(s.CurrentLocationID),
		TurnCount:         int64(s.TurnCount),
		TimeOfDay:         string(s.TimeOfDay),
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	})
}

func (r *Repository) GetSave(ctx context.Context, id string) (Save, error) {
	row, err := storesqlc.New(r.q).GetSave(ctx, id)
	if err != nil {
		return Save{}, mapErr(err)
	}
	return saveFromSQLC(row), nil
}

func (r *Repository) ListSaves(ctx context.Context) ([]Save, error) {
	rows, err := storesqlc.New(r.q).ListSaves(ctx)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, saveFromSQLC), nil
}

// SetTimeOfDay 设置存档的当前时间段。值非法时返回错误。
func (r *Repository) SetTimeOfDay(ctx context.Context, id string, t TimeOfDay) error {
	if !t.IsValid() {
		return fmt.Errorf("invalid time_of_day: %q", t)
	}
	return checkRowsAffected(storesqlc.New(r.q).SetTimeOfDay(ctx, storesqlc.SetTimeOfDayParams{
		TimeOfDay: string(t),
		UpdatedAt: nowMS(),
		ID:        id,
	}))
}

func (r *Repository) SetStage(ctx context.Context, id, stage string) error {
	if stage == "" {
		return fmt.Errorf("invalid stage: empty")
	}
	return checkRowsAffected(storesqlc.New(r.q).SetStage(ctx, storesqlc.SetStageParams{
		Stage:     stage,
		UpdatedAt: nowMS(),
		ID:        id,
	}))
}

func (r *Repository) DeleteSave(ctx context.Context, id string) error {
	return checkRowsAffected(storesqlc.New(r.q).DeleteSave(ctx, id))
}

func (r *Repository) UpdateSaveProgress(ctx context.Context, id, locationID string, turn int) error {
	return checkRowsAffected(storesqlc.New(r.q).UpdateSaveProgress(ctx, storesqlc.UpdateSaveProgressParams{
		CurrentLocationID: sqlNullString(locationID),
		TurnCount:         int64(turn),
		UpdatedAt:         nowMS(),
		ID:                id,
	}))
}

func saveFromSQLC(s storesqlc.Save) Save {
	return Save{
		ID:                s.ID,
		Name:              s.Name,
		ScenarioID:        s.ScenarioID,
		VariantID:         s.VariantID,
		Stage:             s.Stage,
		CurrentLocationID: s.CurrentLocationID.String,
		TurnCount:         int(s.TurnCount),
		TimeOfDay:         TimeOfDay(s.TimeOfDay),
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}
}

// ============================================================================
// Investigator
// ============================================================================

func (r *Repository) UpsertInvestigator(ctx context.Context, inv Investigator) error {
	return storesqlc.New(r.q).UpsertInvestigator(ctx, storesqlc.UpsertInvestigatorParams{
		ID:            inv.ID,
		SaveID:        inv.SaveID,
		Name:          inv.Name,
		Occupation:    inv.Occupation,
		AttrsJson:     inv.AttrsJSON,
		SkillsJson:    inv.SkillsJSON,
		Hp:            int64(inv.HP),
		Mp:            int64(inv.MP),
		San:           int64(inv.SAN),
		InventoryJson: inv.InventoryJSON,
		Active:        int64(boolToInt(inv.Active)),
	})
}

func (r *Repository) GetActiveInvestigator(ctx context.Context, saveID string) (Investigator, error) {
	row, err := storesqlc.New(r.q).GetActiveInvestigator(ctx, saveID)
	if err != nil {
		return Investigator{}, mapErr(err)
	}
	return investigatorFromSQLC(row), nil
}

func (r *Repository) UpdateInvestigatorVitals(ctx context.Context, id string, hp, mp, san int) error {
	return checkRowsAffected(storesqlc.New(r.q).UpdateInvestigatorVitals(ctx, storesqlc.UpdateInvestigatorVitalsParams{
		Hp:  int64(hp),
		Mp:  int64(mp),
		San: int64(san),
		ID:  id,
	}))
}

func (r *Repository) ListInvestigators(ctx context.Context, saveID string) ([]Investigator, error) {
	rows, err := storesqlc.New(r.q).ListInvestigators(ctx, saveID)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, investigatorFromSQLC), nil
}

func (r *Repository) DeactivateInvestigator(ctx context.Context, id string) error {
	return checkRowsAffected(storesqlc.New(r.q).DeactivateInvestigator(ctx, id))
}

func investigatorFromSQLC(inv storesqlc.Investigator) Investigator {
	return Investigator{
		ID:            inv.ID,
		SaveID:        inv.SaveID,
		Name:          inv.Name,
		Occupation:    inv.Occupation,
		AttrsJSON:     inv.AttrsJson,
		SkillsJSON:    inv.SkillsJson,
		HP:            int(inv.Hp),
		MP:            int(inv.Mp),
		SAN:           int(inv.San),
		InventoryJSON: inv.InventoryJson,
		Active:        inv.Active != 0,
	}
}

// ============================================================================
// NPC
// ============================================================================

func (r *Repository) UpsertNPC(ctx context.Context, n NPC) error {
	return storesqlc.New(r.q).UpsertNPC(ctx, storesqlc.UpsertNPCParams{
		ID:               n.ID,
		SaveID:           n.SaveID,
		Name:             n.Name,
		Personality:      n.Personality,
		KnowledgeJson:    n.KnowledgeJSON,
		RelationToPlayer: int64(n.RelationToPlayer),
		LocationID:       sqlNullString(n.LocationID),
		Alive:            int64(boolToInt(n.Alive)),
	})
}

func (r *Repository) GetNPC(ctx context.Context, id string) (NPC, error) {
	row, err := storesqlc.New(r.q).GetNPC(ctx, id)
	if err != nil {
		return NPC{}, mapErr(err)
	}
	return npcFromSQLC(row), nil
}

func (r *Repository) ListNPCsAtLocation(ctx context.Context, saveID, locationID string) ([]NPC, error) {
	rows, err := storesqlc.New(r.q).ListNPCsAtLocation(ctx, storesqlc.ListNPCsAtLocationParams{
		SaveID:     saveID,
		LocationID: sqlNullString(locationID),
	})
	if err != nil {
		return nil, err
	}
	return mapRows(rows, npcFromSQLC), nil
}

func (r *Repository) UpdateNPCRelation(ctx context.Context, id string, delta int) error {
	return checkRowsAffected(storesqlc.New(r.q).UpdateNPCRelation(ctx, storesqlc.UpdateNPCRelationParams{
		RelationToPlayer: int64(delta),
		ID:               id,
	}))
}

func (r *Repository) KillNPC(ctx context.Context, id string) error {
	return checkRowsAffected(storesqlc.New(r.q).KillNPC(ctx, id))
}

func npcFromSQLC(n storesqlc.NPC) NPC {
	return NPC{
		ID:               n.ID,
		SaveID:           n.SaveID,
		Name:             n.Name,
		Personality:      n.Personality,
		KnowledgeJSON:    n.KnowledgeJson,
		RelationToPlayer: int(n.RelationToPlayer),
		LocationID:       n.LocationID.String,
		Alive:            n.Alive != 0,
	}
}

// ============================================================================
// Location
// ============================================================================

func (r *Repository) UpsertLocation(ctx context.Context, l Location) error {
	return storesqlc.New(r.q).UpsertLocation(ctx, storesqlc.UpsertLocationParams{
		ID:              l.ID,
		SaveID:          l.SaveID,
		Name:            l.Name,
		Description:     l.Description,
		ParentID:        sqlNullString(l.ParentID),
		ConnectionsJson: l.ConnectionsJSON,
		Visited:         int64(boolToInt(l.Visited)),
	})
}

func (r *Repository) GetLocation(ctx context.Context, id string) (Location, error) {
	row, err := storesqlc.New(r.q).GetLocation(ctx, id)
	if err != nil {
		return Location{}, mapErr(err)
	}
	return locationFromSQLC(row), nil
}

func (r *Repository) MarkLocationVisited(ctx context.Context, id string) error {
	return checkRowsAffected(storesqlc.New(r.q).MarkLocationVisited(ctx, id))
}

func locationFromSQLC(l storesqlc.Location) Location {
	return Location{
		ID:              l.ID,
		SaveID:          l.SaveID,
		Name:            l.Name,
		Description:     l.Description,
		ParentID:        l.ParentID.String,
		ConnectionsJSON: l.ConnectionsJson,
		Visited:         l.Visited != 0,
	}
}

// ============================================================================
// Item
// ============================================================================

func (r *Repository) UpsertItem(ctx context.Context, it Item) error {
	if it.OwnerType == "" {
		it.OwnerType = OwnerNone
	}
	return storesqlc.New(r.q).UpsertItem(ctx, storesqlc.UpsertItemParams{
		ID:             it.ID,
		SaveID:         it.SaveID,
		Name:           it.Name,
		Description:    it.Description,
		OwnerType:      string(it.OwnerType),
		OwnerID:        sqlNullString(it.OwnerID),
		PropertiesJson: it.PropertiesJSON,
		Destroyed:      int64(boolToInt(it.Destroyed)),
	})
}

func (r *Repository) GetItem(ctx context.Context, id string) (Item, error) {
	row, err := storesqlc.New(r.q).GetItem(ctx, id)
	if err != nil {
		return Item{}, mapErr(err)
	}
	return itemFromSQLC(row), nil
}

func (r *Repository) ListItems(ctx context.Context, saveID string) ([]Item, error) {
	rows, err := storesqlc.New(r.q).ListItems(ctx, saveID)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, itemFromSQLC), nil
}

func (r *Repository) MoveItem(ctx context.Context, id string, ownerType OwnerType, ownerID string) error {
	return checkRowsAffected(storesqlc.New(r.q).MoveItem(ctx, storesqlc.MoveItemParams{
		OwnerType: string(ownerType),
		OwnerID:   sqlNullString(ownerID),
		ID:        id,
	}))
}

func (r *Repository) DestroyItem(ctx context.Context, id string) error {
	return checkRowsAffected(storesqlc.New(r.q).DestroyItem(ctx, id))
}

func (r *Repository) ListDestroyedItems(ctx context.Context, saveID string) ([]Item, error) {
	rows, err := storesqlc.New(r.q).ListDestroyedItems(ctx, saveID)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, itemFromSQLC), nil
}

func itemFromSQLC(it storesqlc.Item) Item {
	return Item{
		ID:             it.ID,
		SaveID:         it.SaveID,
		Name:           it.Name,
		Description:    it.Description,
		OwnerType:      OwnerType(it.OwnerType),
		OwnerID:        it.OwnerID.String,
		PropertiesJSON: it.PropertiesJson,
		Destroyed:      it.Destroyed != 0,
	}
}

// ============================================================================
// Clue
// ============================================================================

func (r *Repository) UpsertClue(ctx context.Context, c Clue) error {
	return storesqlc.New(r.q).UpsertClue(ctx, storesqlc.UpsertClueParams{
		ID:                c.ID,
		SaveID:            c.SaveID,
		ScenarioID:        c.ScenarioID,
		Description:       c.Description,
		Found:             int64(boolToInt(c.Found)),
		FoundInLocationID: sqlNullString(c.FoundInLocationID),
		FoundAtTurn:       sqlNullInt64(c.FoundAtTurn),
	})
}

func (r *Repository) MarkClueFound(ctx context.Context, id, locationID string, turn int) error {
	return checkRowsAffected(storesqlc.New(r.q).MarkClueFound(ctx, storesqlc.MarkClueFoundParams{
		FoundInLocationID: sqlNullString(locationID),
		FoundAtTurn:       sqlNullInt64(turn),
		ID:                id,
	}))
}

func (r *Repository) ListFoundClues(ctx context.Context, saveID string) ([]Clue, error) {
	rows, err := storesqlc.New(r.q).ListFoundClues(ctx, saveID)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, clueFromSQLC), nil
}

func clueFromSQLC(c storesqlc.Clue) Clue {
	return Clue{
		ID:                c.ID,
		SaveID:            c.SaveID,
		ScenarioID:        c.ScenarioID,
		Description:       c.Description,
		Found:             c.Found != 0,
		FoundInLocationID: c.FoundInLocationID.String,
		FoundAtTurn:       int(c.FoundAtTurn.Int64),
	}
}

// ============================================================================
// Event
// ============================================================================

func (r *Repository) AppendEvent(ctx context.Context, e Event) (int64, error) {
	if e.CreatedAt == 0 {
		e.CreatedAt = nowMS()
	}
	if e.RelatedEntitiesJSON == "" {
		e.RelatedEntitiesJSON = "[]"
	}
	id, err := storesqlc.New(r.q).AppendEvent(ctx, storesqlc.AppendEventParams{
		SaveID:              e.SaveID,
		Turn:                int64(e.Turn),
		Type:                string(e.Type),
		Description:         e.Description,
		RelatedEntitiesJson: e.RelatedEntitiesJSON,
		CreatedAt:           e.CreatedAt,
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ListEvents 返回 [fromTurn, toTurn] 闭区间事件，按 (turn, id) 升序。
// fromTurn = 0 表示不限下界；toTurn = 0 表示不限上界。
func (r *Repository) ListEvents(ctx context.Context, saveID string, fromTurn, toTurn int) ([]Event, error) {
	rows, err := storesqlc.New(r.q).ListEvents(ctx, storesqlc.ListEventsParams{
		SaveID:  saveID,
		Column2: int64(fromTurn),
		Turn:    int64(fromTurn),
		Column4: int64(toTurn),
		Turn_2:  int64(toTurn),
	})
	if err != nil {
		return nil, err
	}
	return mapRows(rows, eventFromSQLC), nil
}

func eventFromSQLC(e storesqlc.Event) Event {
	return Event{
		ID:                  e.ID,
		SaveID:              e.SaveID,
		Turn:                int(e.Turn),
		Type:                EventType(e.Type),
		Description:         e.Description,
		RelatedEntitiesJSON: e.RelatedEntitiesJson,
		CreatedAt:           e.CreatedAt,
	}
}
