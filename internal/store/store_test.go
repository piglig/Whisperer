package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore 打开内存库并自动 Close。
func newTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s, ctx
}

// makeSave 在指定 store 内插入一个有效 save，返回 saveID。
func makeSave(t *testing.T, ctx context.Context, r *Repository) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, Save{
		ID: id, Name: "test-save", ScenarioID: "fog_harbor",
	}))
	return id
}

// ---------------------------------------------------------------------------
// Open / schema
// ---------------------------------------------------------------------------

func TestOpen_AppliesSchema(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saves, err := r.ListSaves(ctx)
	require.NoError(t, err)
	assert.Empty(t, saves)
}

func TestOpen_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.db")
	ctx := context.Background()

	s, err := Open(ctx, path)
	require.NoError(t, err)
	saveID := makeSave(t, ctx, s.Repo())
	require.NoError(t, s.Close())

	// 重开 → 状态应保持
	s2, err := Open(ctx, path)
	require.NoError(t, err)
	defer s2.Close()
	got, err := s2.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, "test-save", got.Name)
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

func TestSave_CRUD(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()

	id := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, Save{ID: id, Name: "alpha", ScenarioID: "fog_harbor"}))

	got, err := r.GetSave(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "alpha", got.Name)
	assert.Equal(t, "opening", got.Stage)
	assert.Equal(t, 0, got.TurnCount)
	assert.NotZero(t, got.CreatedAt)
	assert.NotZero(t, got.UpdatedAt)

	// progress
	require.NoError(t, r.UpdateSaveProgress(ctx, id, "loc-1", 5))
	got, err = r.GetSave(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "loc-1", got.CurrentLocationID)
	assert.Equal(t, 5, got.TurnCount)

	require.NoError(t, r.SetStage(ctx, id, "investigation"))
	got, err = r.GetSave(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "investigation", got.Stage)

	// list
	require.NoError(t, r.CreateSave(ctx, Save{ID: uuid.NewString(), Name: "beta", ScenarioID: "fog_harbor"}))
	saves, err := r.ListSaves(ctx)
	require.NoError(t, err)
	assert.Len(t, saves, 2)

	// delete
	require.NoError(t, r.DeleteSave(ctx, id))
	_, err = r.GetSave(ctx, id)
	assert.ErrorIs(t, err, ErrNotFound)

	// double delete
	err = r.DeleteSave(ctx, id)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSave_NotFound(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	_, err := r.GetSave(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.True(t, errors.Is(err, ErrNotFound))

	err = r.UpdateSaveProgress(ctx, "missing", "l", 1)
	assert.ErrorIs(t, err, ErrNotFound)

	err = r.SetStage(ctx, "missing", "opening")
	assert.ErrorIs(t, err, ErrNotFound)

	err = r.SetStage(ctx, "missing", "")
	assert.ErrorContains(t, err, "invalid stage")
}

// ---------------------------------------------------------------------------
// Investigator
// ---------------------------------------------------------------------------

func TestInvestigator_RoundTrip(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)

	inv := Investigator{
		ID: "inv-1", SaveID: saveID, Name: "Lyra", Occupation: "Journalist",
		AttrsJSON: `{"STR":50}`, SkillsJSON: `{"Spot Hidden":60}`,
		HP: 12, MP: 10, SAN: 60, InventoryJSON: `["notebook"]`, Active: true,
	}
	require.NoError(t, r.UpsertInvestigator(ctx, inv))

	got, err := r.GetActiveInvestigator(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, inv, got)

	// vitals
	require.NoError(t, r.UpdateInvestigatorVitals(ctx, "inv-1", 8, 10, 55))
	got, err = r.GetActiveInvestigator(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, 8, got.HP)
	assert.Equal(t, 55, got.SAN)

	// upsert（再次写入相同 ID 不应报错）
	inv.Name = "Lyra (renamed)"
	require.NoError(t, r.UpsertInvestigator(ctx, inv))

	// list / deactivate
	list, err := r.ListInvestigators(ctx, saveID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	require.NoError(t, r.DeactivateInvestigator(ctx, "inv-1"))
	_, err = r.GetActiveInvestigator(ctx, saveID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestInvestigator_DeactivateMissing(t *testing.T) {
	s, ctx := newTestStore(t)
	err := s.Repo().DeactivateInvestigator(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestInvestigator_UpdateVitalsMissing(t *testing.T) {
	s, ctx := newTestStore(t)
	err := s.Repo().UpdateInvestigatorVitals(ctx, "missing", 1, 1, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

// ---------------------------------------------------------------------------
// NPC
// ---------------------------------------------------------------------------

func TestNPC_CRUD(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)
	require.NoError(t, r.UpsertLocation(ctx, Location{
		ID: "harbor", SaveID: saveID, Name: "Harbor", Description: "foggy",
	}))

	npc := NPC{
		ID: "npc-1", SaveID: saveID, Name: "Dr. Vance", Personality: "stern",
		KnowledgeJSON: `{"murder":"yes"}`, RelationToPlayer: 0,
		LocationID: "harbor", Alive: true,
	}
	require.NoError(t, r.UpsertNPC(ctx, npc))

	got, err := r.GetNPC(ctx, "npc-1")
	require.NoError(t, err)
	assert.Equal(t, npc, got)

	// list at location
	list, err := r.ListNPCsAtLocation(ctx, saveID, "harbor")
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// relation
	require.NoError(t, r.UpdateNPCRelation(ctx, "npc-1", -10))
	got, err = r.GetNPC(ctx, "npc-1")
	require.NoError(t, err)
	assert.Equal(t, -10, got.RelationToPlayer)

	// kill
	require.NoError(t, r.KillNPC(ctx, "npc-1"))
	list, err = r.ListNPCsAtLocation(ctx, saveID, "harbor")
	require.NoError(t, err)
	assert.Empty(t, list, "killed NPC should not appear in location listing")
	got, err = r.GetNPC(ctx, "npc-1")
	require.NoError(t, err)
	assert.False(t, got.Alive)
}

func TestNPC_NotFoundUpdates(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	_, err := r.GetNPC(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, r.UpdateNPCRelation(ctx, "missing", 1), ErrNotFound)
	assert.ErrorIs(t, r.KillNPC(ctx, "missing"), ErrNotFound)
}

// ---------------------------------------------------------------------------
// Location
// ---------------------------------------------------------------------------

func TestLocation_CRUD(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)

	loc := Location{
		ID: "harbor", SaveID: saveID, Name: "Harbor",
		Description: "foggy docks", ConnectionsJSON: `["pub"]`, Visited: false,
	}
	require.NoError(t, r.UpsertLocation(ctx, loc))

	got, err := r.GetLocation(ctx, "harbor")
	require.NoError(t, err)
	assert.Equal(t, loc, got)

	require.NoError(t, r.MarkLocationVisited(ctx, "harbor"))
	got, _ = r.GetLocation(ctx, "harbor")
	assert.True(t, got.Visited)

	_, err = r.GetLocation(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, r.MarkLocationVisited(ctx, "missing"), ErrNotFound)
}

// ---------------------------------------------------------------------------
// Item
// ---------------------------------------------------------------------------

func TestItem_LifecycleAndDestroy(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)

	it := Item{
		ID: "lantern", SaveID: saveID, Name: "Brass Lantern",
		Description: "old but functional", OwnerType: OwnerLocation, OwnerID: "harbor",
		PropertiesJSON: `{}`, Destroyed: false,
	}
	require.NoError(t, r.UpsertItem(ctx, it))

	got, err := r.GetItem(ctx, "lantern")
	require.NoError(t, err)
	assert.Equal(t, OwnerLocation, got.OwnerType)
	assert.Equal(t, "harbor", got.OwnerID)

	items, err := r.ListItems(ctx, saveID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "lantern", items[0].ID)

	// move to investigator
	require.NoError(t, r.MoveItem(ctx, "lantern", OwnerInvestigator, "inv-1"))
	got, _ = r.GetItem(ctx, "lantern")
	assert.Equal(t, OwnerInvestigator, got.OwnerType)
	assert.Equal(t, "inv-1", got.OwnerID)

	// destroy
	require.NoError(t, r.DestroyItem(ctx, "lantern"))
	got, _ = r.GetItem(ctx, "lantern")
	assert.True(t, got.Destroyed)

	// list destroyed
	list, err := r.ListDestroyedItems(ctx, saveID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "lantern", list[0].ID)
}

func TestItem_DefaultOwnerType(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)
	require.NoError(t, r.UpsertItem(ctx, Item{
		ID: "x", SaveID: saveID, Name: "x", Description: "x",
		// OwnerType 留空 → 应被填为 OwnerNone
	}))
	got, _ := r.GetItem(ctx, "x")
	assert.Equal(t, OwnerNone, got.OwnerType)
}

func TestItem_NotFound(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	_, err := r.GetItem(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, r.MoveItem(ctx, "missing", OwnerNone, ""), ErrNotFound)
	assert.ErrorIs(t, r.DestroyItem(ctx, "missing"), ErrNotFound)
}

// ---------------------------------------------------------------------------
// Clue
// ---------------------------------------------------------------------------

func TestClue_FoundFlow(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)

	c := Clue{
		ID: "letter", SaveID: saveID, ScenarioID: "fog_harbor",
		Description: "blood-stained letter",
	}
	require.NoError(t, r.UpsertClue(ctx, c))

	require.NoError(t, r.MarkClueFound(ctx, "letter", "harbor", 3))

	list, err := r.ListFoundClues(ctx, saveID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.True(t, list[0].Found)
	assert.Equal(t, "harbor", list[0].FoundInLocationID)
	assert.Equal(t, 3, list[0].FoundAtTurn)

	assert.ErrorIs(t, r.MarkClueFound(ctx, "missing", "x", 1), ErrNotFound)
}

// ---------------------------------------------------------------------------
// Event
// ---------------------------------------------------------------------------

func TestEvent_AppendAndQuery(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)

	for i, typ := range []EventType{EventNarrative, EventToolCall, EventStateChange} {
		_, err := r.AppendEvent(ctx, Event{
			SaveID: saveID, Turn: i + 1, Type: typ,
			Description: string(typ),
		})
		require.NoError(t, err)
	}

	all, err := r.ListEvents(ctx, saveID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	mid, err := r.ListEvents(ctx, saveID, 2, 2)
	require.NoError(t, err)
	require.Len(t, mid, 1)
	assert.Equal(t, EventToolCall, mid[0].Type)
	assert.NotEmpty(t, mid[0].RelatedEntitiesJSON)

	id, err := r.AppendEvent(ctx, Event{
		SaveID: saveID, Turn: 4, Type: EventDiceRoll, Description: "dice",
		RelatedEntitiesJSON: `["npc-1"]`,
	})
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

// ---------------------------------------------------------------------------
// 外键级联
// ---------------------------------------------------------------------------

func TestForeignKey_CascadeDelete(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	saveID := makeSave(t, ctx, r)

	require.NoError(t, r.UpsertInvestigator(ctx, Investigator{
		ID: "i", SaveID: saveID, Name: "i", Occupation: "x",
		AttrsJSON: "{}", SkillsJSON: "{}", InventoryJSON: "[]", Active: true,
	}))
	require.NoError(t, r.UpsertLocation(ctx, Location{
		ID: "l", SaveID: saveID, Name: "l", Description: "l",
	}))
	_, err := r.AppendEvent(ctx, Event{SaveID: saveID, Turn: 1, Type: EventNarrative, Description: "x"})
	require.NoError(t, err)

	require.NoError(t, r.DeleteSave(ctx, saveID))

	invs, _ := r.ListInvestigators(ctx, saveID)
	assert.Empty(t, invs, "investigators should cascade-delete")
	events, _ := r.ListEvents(ctx, saveID, 0, 0)
	assert.Empty(t, events)
	_, err = r.GetLocation(ctx, "l")
	assert.ErrorIs(t, err, ErrNotFound)
}

// ---------------------------------------------------------------------------
// Tx (RunTurn)
// ---------------------------------------------------------------------------

func TestRunTurn_CommitsOnSuccess(t *testing.T) {
	s, ctx := newTestStore(t)
	saveID := makeSave(t, ctx, s.Repo())

	err := s.RunTurn(ctx, func(ctx context.Context, r *Repository) error {
		return r.UpsertLocation(ctx, Location{
			ID: "l", SaveID: saveID, Name: "l", Description: "l",
		})
	})
	require.NoError(t, err)

	got, err := s.Repo().GetLocation(ctx, "l")
	require.NoError(t, err)
	assert.Equal(t, "l", got.ID)
}

func TestRunTurn_RollsBackOnError(t *testing.T) {
	s, ctx := newTestStore(t)
	saveID := makeSave(t, ctx, s.Repo())

	sentinel := errors.New("sla violation")
	err := s.RunTurn(ctx, func(ctx context.Context, r *Repository) error {
		if err := r.UpsertLocation(ctx, Location{
			ID: "l", SaveID: saveID, Name: "l", Description: "l",
		}); err != nil {
			return err
		}
		_, err := r.AppendEvent(ctx, Event{
			SaveID: saveID, Turn: 1, Type: EventNarrative, Description: "x",
		})
		if err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	// 全部回滚：location 与 event 都不该存在
	_, err = s.Repo().GetLocation(ctx, "l")
	assert.ErrorIs(t, err, ErrNotFound)
	events, err := s.Repo().ListEvents(ctx, saveID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, events)
}

// ---------------------------------------------------------------------------
// time injection
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// time of day
// ---------------------------------------------------------------------------

func TestSave_TimeOfDayDefault(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	id := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, Save{ID: id, Name: "x", ScenarioID: "y"}))
	got, err := r.GetSave(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, TimeMorning, got.TimeOfDay)
}

func TestSetTimeOfDay(t *testing.T) {
	s, ctx := newTestStore(t)
	r := s.Repo()
	id := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, Save{ID: id, Name: "x", ScenarioID: "y"}))

	require.NoError(t, r.SetTimeOfDay(ctx, id, TimeNight))
	got, _ := r.GetSave(ctx, id)
	assert.Equal(t, TimeNight, got.TimeOfDay)

	assert.Error(t, r.SetTimeOfDay(ctx, id, "midnight"))
	assert.ErrorIs(t, r.SetTimeOfDay(ctx, "missing", TimeMorning), ErrNotFound)
}

func TestTimeOfDay_NextAndIsValid(t *testing.T) {
	assert.True(t, TimeMorning.IsValid())
	assert.True(t, TimeAfternoon.IsValid())
	assert.True(t, TimeNight.IsValid())
	assert.False(t, TimeOfDay("noon").IsValid())

	assert.Equal(t, TimeAfternoon, TimeMorning.Next())
	assert.Equal(t, TimeNight, TimeAfternoon.Next())
	assert.Equal(t, TimeMorning, TimeNight.Next())
	assert.Equal(t, TimeMorning, TimeOfDay("bogus").Next())
}

func TestNowMS_Injectable(t *testing.T) {
	old := nowMS
	t.Cleanup(func() { nowMS = old })
	nowMS = func() int64 { return 12345 }

	s, ctx := newTestStore(t)
	r := s.Repo()
	id := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, Save{ID: id, Name: "x", ScenarioID: "y"}))
	got, _ := r.GetSave(ctx, id)
	assert.Equal(t, int64(12345), got.CreatedAt)
	assert.Equal(t, int64(12345), got.UpdatedAt)
}
