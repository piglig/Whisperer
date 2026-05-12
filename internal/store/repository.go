package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Repository 在主连接或事务上提供 CRUD。同一签名既可用于 *sql.DB（store.Repo()）
// 也可用于 *sql.Tx（store.RunTurn 内传入），背后由 querier 抽象。
type Repository struct {
	q querier
}

// nowMS 是 time.Now().UnixMilli() 的可注入版本。测试可临时替换以得到稳定时间。
var nowMS = func() int64 { return time.Now().UnixMilli() }

// boolToInt / intToBool 简化 SQLite 0/1 ↔ Go bool 转换。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullableString 把空字符串映射为 SQL NULL，反之亦然。
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
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
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO saves (id, name, scenario_id, current_location_id, turn_count, time_of_day, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.ScenarioID, nullableString(s.CurrentLocationID), s.TurnCount,
		string(s.TimeOfDay), s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (r *Repository) GetSave(ctx context.Context, id string) (Save, error) {
	var s Save
	var loc sql.NullString
	var tod string
	err := r.q.QueryRowContext(ctx, `
		SELECT id, name, scenario_id, current_location_id, turn_count, time_of_day, created_at, updated_at
		FROM saves WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.ScenarioID, &loc, &s.TurnCount, &tod, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return Save{}, mapErr(err)
	}
	s.CurrentLocationID = loc.String
	s.TimeOfDay = TimeOfDay(tod)
	return s, nil
}

func (r *Repository) ListSaves(ctx context.Context) ([]Save, error) {
	rows, err := r.q.QueryContext(ctx, `
		SELECT id, name, scenario_id, current_location_id, turn_count, time_of_day, created_at, updated_at
		FROM saves ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Save{}
	for rows.Next() {
		var s Save
		var loc sql.NullString
		var tod string
		if err := rows.Scan(&s.ID, &s.Name, &s.ScenarioID, &loc, &s.TurnCount, &tod, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.CurrentLocationID = loc.String
		s.TimeOfDay = TimeOfDay(tod)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetTimeOfDay 设置存档的当前时间段。值非法时返回错误。
func (r *Repository) SetTimeOfDay(ctx context.Context, id string, t TimeOfDay) error {
	if !t.IsValid() {
		return fmt.Errorf("invalid time_of_day: %q", t)
	}
	res, err := r.q.ExecContext(ctx, `UPDATE saves SET time_of_day = ?, updated_at = ? WHERE id = ?`,
		string(t), nowMS(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteSave(ctx context.Context, id string) error {
	res, err := r.q.ExecContext(ctx, `DELETE FROM saves WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateSaveProgress(ctx context.Context, id, locationID string, turn int) error {
	res, err := r.q.ExecContext(ctx, `
		UPDATE saves SET current_location_id = ?, turn_count = ?, updated_at = ?
		WHERE id = ?`, nullableString(locationID), turn, nowMS(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// Investigator
// ============================================================================

func (r *Repository) UpsertInvestigator(ctx context.Context, inv Investigator) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO investigators (id, save_id, name, occupation, attrs_json, skills_json, hp, mp, san, inventory_json, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			save_id = excluded.save_id,
			name = excluded.name,
			occupation = excluded.occupation,
			attrs_json = excluded.attrs_json,
			skills_json = excluded.skills_json,
			hp = excluded.hp,
			mp = excluded.mp,
			san = excluded.san,
			inventory_json = excluded.inventory_json,
			active = excluded.active`,
		inv.ID, inv.SaveID, inv.Name, inv.Occupation, inv.AttrsJSON, inv.SkillsJSON,
		inv.HP, inv.MP, inv.SAN, inv.InventoryJSON, boolToInt(inv.Active),
	)
	return err
}

func (r *Repository) GetActiveInvestigator(ctx context.Context, saveID string) (Investigator, error) {
	var inv Investigator
	var active int
	err := r.q.QueryRowContext(ctx, `
		SELECT id, save_id, name, occupation, attrs_json, skills_json, hp, mp, san, inventory_json, active
		FROM investigators WHERE save_id = ? AND active = 1
		LIMIT 1`, saveID,
	).Scan(&inv.ID, &inv.SaveID, &inv.Name, &inv.Occupation, &inv.AttrsJSON, &inv.SkillsJSON,
		&inv.HP, &inv.MP, &inv.SAN, &inv.InventoryJSON, &active)
	if err != nil {
		return Investigator{}, mapErr(err)
	}
	inv.Active = active != 0
	return inv, nil
}

func (r *Repository) UpdateInvestigatorVitals(ctx context.Context, id string, hp, mp, san int) error {
	res, err := r.q.ExecContext(ctx, `
		UPDATE investigators SET hp = ?, mp = ?, san = ? WHERE id = ?`,
		hp, mp, san, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListInvestigators(ctx context.Context, saveID string) ([]Investigator, error) {
	rows, err := r.q.QueryContext(ctx, `
		SELECT id, save_id, name, occupation, attrs_json, skills_json, hp, mp, san, inventory_json, active
		FROM investigators WHERE save_id = ? ORDER BY name`, saveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Investigator{}
	for rows.Next() {
		var inv Investigator
		var active int
		if err := rows.Scan(&inv.ID, &inv.SaveID, &inv.Name, &inv.Occupation, &inv.AttrsJSON, &inv.SkillsJSON,
			&inv.HP, &inv.MP, &inv.SAN, &inv.InventoryJSON, &active); err != nil {
			return nil, err
		}
		inv.Active = active != 0
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *Repository) DeactivateInvestigator(ctx context.Context, id string) error {
	res, err := r.q.ExecContext(ctx, `UPDATE investigators SET active = 0 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// NPC
// ============================================================================

func (r *Repository) UpsertNPC(ctx context.Context, n NPC) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO npcs (id, save_id, name, personality, knowledge_json, relation_to_player, location_id, alive)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			save_id = excluded.save_id,
			name = excluded.name,
			personality = excluded.personality,
			knowledge_json = excluded.knowledge_json,
			relation_to_player = excluded.relation_to_player,
			location_id = excluded.location_id,
			alive = excluded.alive`,
		n.ID, n.SaveID, n.Name, n.Personality, n.KnowledgeJSON, n.RelationToPlayer,
		nullableString(n.LocationID), boolToInt(n.Alive),
	)
	return err
}

func (r *Repository) GetNPC(ctx context.Context, id string) (NPC, error) {
	var n NPC
	var loc sql.NullString
	var alive int
	err := r.q.QueryRowContext(ctx, `
		SELECT id, save_id, name, personality, knowledge_json, relation_to_player, location_id, alive
		FROM npcs WHERE id = ?`, id,
	).Scan(&n.ID, &n.SaveID, &n.Name, &n.Personality, &n.KnowledgeJSON, &n.RelationToPlayer, &loc, &alive)
	if err != nil {
		return NPC{}, mapErr(err)
	}
	n.LocationID = loc.String
	n.Alive = alive != 0
	return n, nil
}

func (r *Repository) ListNPCsAtLocation(ctx context.Context, saveID, locationID string) ([]NPC, error) {
	rows, err := r.q.QueryContext(ctx, `
		SELECT id, save_id, name, personality, knowledge_json, relation_to_player, location_id, alive
		FROM npcs WHERE save_id = ? AND location_id = ? AND alive = 1
		ORDER BY name`, saveID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NPC{}
	for rows.Next() {
		var n NPC
		var loc sql.NullString
		var alive int
		if err := rows.Scan(&n.ID, &n.SaveID, &n.Name, &n.Personality, &n.KnowledgeJSON, &n.RelationToPlayer, &loc, &alive); err != nil {
			return nil, err
		}
		n.LocationID = loc.String
		n.Alive = alive != 0
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateNPCRelation(ctx context.Context, id string, delta int) error {
	res, err := r.q.ExecContext(ctx, `
		UPDATE npcs SET relation_to_player = relation_to_player + ? WHERE id = ?`,
		delta, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) KillNPC(ctx context.Context, id string) error {
	res, err := r.q.ExecContext(ctx, `UPDATE npcs SET alive = 0 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// Location
// ============================================================================

func (r *Repository) UpsertLocation(ctx context.Context, l Location) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO locations (id, save_id, name, description, parent_id, connections_json, visited)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			save_id = excluded.save_id,
			name = excluded.name,
			description = excluded.description,
			parent_id = excluded.parent_id,
			connections_json = excluded.connections_json,
			visited = excluded.visited`,
		l.ID, l.SaveID, l.Name, l.Description, nullableString(l.ParentID),
		l.ConnectionsJSON, boolToInt(l.Visited),
	)
	return err
}

func (r *Repository) GetLocation(ctx context.Context, id string) (Location, error) {
	var l Location
	var parent sql.NullString
	var visited int
	err := r.q.QueryRowContext(ctx, `
		SELECT id, save_id, name, description, parent_id, connections_json, visited
		FROM locations WHERE id = ?`, id,
	).Scan(&l.ID, &l.SaveID, &l.Name, &l.Description, &parent, &l.ConnectionsJSON, &visited)
	if err != nil {
		return Location{}, mapErr(err)
	}
	l.ParentID = parent.String
	l.Visited = visited != 0
	return l, nil
}

func (r *Repository) MarkLocationVisited(ctx context.Context, id string) error {
	res, err := r.q.ExecContext(ctx, `UPDATE locations SET visited = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// Item
// ============================================================================

func (r *Repository) UpsertItem(ctx context.Context, it Item) error {
	if it.OwnerType == "" {
		it.OwnerType = OwnerNone
	}
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO items (id, save_id, name, description, owner_type, owner_id, properties_json, destroyed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			save_id = excluded.save_id,
			name = excluded.name,
			description = excluded.description,
			owner_type = excluded.owner_type,
			owner_id = excluded.owner_id,
			properties_json = excluded.properties_json,
			destroyed = excluded.destroyed`,
		it.ID, it.SaveID, it.Name, it.Description, string(it.OwnerType),
		nullableString(it.OwnerID), it.PropertiesJSON, boolToInt(it.Destroyed),
	)
	return err
}

func (r *Repository) GetItem(ctx context.Context, id string) (Item, error) {
	var it Item
	var owner sql.NullString
	var ownerType string
	var destroyed int
	err := r.q.QueryRowContext(ctx, `
		SELECT id, save_id, name, description, owner_type, owner_id, properties_json, destroyed
		FROM items WHERE id = ?`, id,
	).Scan(&it.ID, &it.SaveID, &it.Name, &it.Description, &ownerType, &owner, &it.PropertiesJSON, &destroyed)
	if err != nil {
		return Item{}, mapErr(err)
	}
	it.OwnerType = OwnerType(ownerType)
	it.OwnerID = owner.String
	it.Destroyed = destroyed != 0
	return it, nil
}

func (r *Repository) MoveItem(ctx context.Context, id string, ownerType OwnerType, ownerID string) error {
	res, err := r.q.ExecContext(ctx, `
		UPDATE items SET owner_type = ?, owner_id = ? WHERE id = ?`,
		string(ownerType), nullableString(ownerID), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DestroyItem(ctx context.Context, id string) error {
	res, err := r.q.ExecContext(ctx, `UPDATE items SET destroyed = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListDestroyedItems(ctx context.Context, saveID string) ([]Item, error) {
	rows, err := r.q.QueryContext(ctx, `
		SELECT id, save_id, name, description, owner_type, owner_id, properties_json, destroyed
		FROM items WHERE save_id = ? AND destroyed = 1
		ORDER BY name`, saveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	out := []Item{}
	for rows.Next() {
		var it Item
		var owner sql.NullString
		var ownerType string
		var destroyed int
		if err := rows.Scan(&it.ID, &it.SaveID, &it.Name, &it.Description, &ownerType, &owner, &it.PropertiesJSON, &destroyed); err != nil {
			return nil, err
		}
		it.OwnerType = OwnerType(ownerType)
		it.OwnerID = owner.String
		it.Destroyed = destroyed != 0
		out = append(out, it)
	}
	return out, rows.Err()
}

// ============================================================================
// Clue
// ============================================================================

func (r *Repository) UpsertClue(ctx context.Context, c Clue) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO clues (id, save_id, scenario_id, description, found, found_in_location_id, found_at_turn)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			save_id = excluded.save_id,
			scenario_id = excluded.scenario_id,
			description = excluded.description,
			found = excluded.found,
			found_in_location_id = excluded.found_in_location_id,
			found_at_turn = excluded.found_at_turn`,
		c.ID, c.SaveID, c.ScenarioID, c.Description, boolToInt(c.Found),
		nullableString(c.FoundInLocationID), nullableInt(c.FoundAtTurn),
	)
	return err
}

func (r *Repository) MarkClueFound(ctx context.Context, id, locationID string, turn int) error {
	res, err := r.q.ExecContext(ctx, `
		UPDATE clues SET found = 1, found_in_location_id = ?, found_at_turn = ?
		WHERE id = ?`, nullableString(locationID), nullableInt(turn), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListFoundClues(ctx context.Context, saveID string) ([]Clue, error) {
	rows, err := r.q.QueryContext(ctx, `
		SELECT id, save_id, scenario_id, description, found, found_in_location_id, found_at_turn
		FROM clues WHERE save_id = ? AND found = 1
		ORDER BY found_at_turn`, saveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Clue{}
	for rows.Next() {
		var c Clue
		var loc sql.NullString
		var turn sql.NullInt64
		var found int
		if err := rows.Scan(&c.ID, &c.SaveID, &c.ScenarioID, &c.Description, &found, &loc, &turn); err != nil {
			return nil, err
		}
		c.Found = found != 0
		c.FoundInLocationID = loc.String
		c.FoundAtTurn = int(turn.Int64)
		out = append(out, c)
	}
	return out, rows.Err()
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
	res, err := r.q.ExecContext(ctx, `
		INSERT INTO events (save_id, turn, type, description, related_entities_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.SaveID, e.Turn, string(e.Type), e.Description, e.RelatedEntitiesJSON, e.CreatedAt,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// ListEvents 返回 [fromTurn, toTurn] 闭区间事件，按 (turn, id) 升序。
// fromTurn = 0 表示不限下界；toTurn = 0 表示不限上界。
func (r *Repository) ListEvents(ctx context.Context, saveID string, fromTurn, toTurn int) ([]Event, error) {
	q := `
		SELECT id, save_id, turn, type, description, related_entities_json, created_at
		FROM events WHERE save_id = ?`
	args := []any{saveID}
	if fromTurn > 0 {
		q += ` AND turn >= ?`
		args = append(args, fromTurn)
	}
	if toTurn > 0 {
		q += ` AND turn <= ?`
		args = append(args, toTurn)
	}
	q += ` ORDER BY turn ASC, id ASC`
	rows, err := r.q.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var typ string
		if err := rows.Scan(&e.ID, &e.SaveID, &e.Turn, &typ, &e.Description, &e.RelatedEntitiesJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Type = EventType(typ)
		out = append(out, e)
	}
	return out, rows.Err()
}
