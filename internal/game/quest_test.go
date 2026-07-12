package game

import (
	"testing"
	"time"
)

func TestQuestComboScoresWithHeatAndExpires(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)

	quest.OnNormalFood(game, HeatByLevel(4), now)
	if quest.Combo != 1 || game.Score != 17 {
		t.Fatalf("after first food combo=%d score=%d, want combo 1 score 17", quest.Combo, game.Score)
	}
	quest.OnNormalFood(game, HeatByLevel(4), now.Add(time.Second))
	if quest.Combo != 2 || game.Score != 38 {
		t.Fatalf("after second food combo=%d score=%d, want combo 2 score 38", quest.Combo, game.Score)
	}
	quest.Tick(game, HeatByLevel(4), now.Add(time.Second).Add(comboWindow))
	if quest.Combo != 0 {
		t.Fatalf("combo = %d, want expired", quest.Combo)
	}
}

func TestQuestGoldenByteSpawnCollectAndFullBoard(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(4, 4, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 4, 4)
	for i := 0; i < 7; i++ {
		quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Duration(i)*time.Second))
	}
	if !quest.Golden.Active {
		t.Fatal("golden byte did not spawn after seventh food")
	}
	if quest.Golden.Position == game.Food {
		t.Fatalf("golden byte spawned on normal food at %#v", quest.Golden.Position)
	}
	for _, point := range game.Snake {
		if quest.Golden.Position == point {
			t.Fatalf("golden byte spawned on snake at %#v", quest.Golden.Position)
		}
	}
	quest.Tick(game, HeatByLevel(1), now.Add(7*time.Second).Add(goldenByteTTL))
	if quest.Golden.Active {
		t.Fatal("golden byte remained active after timeout")
	}
	for i := 0; i < 7; i++ {
		quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Duration(20+i)*time.Second))
	}
	quest.OnGoldenByte(game, HeatByLevel(1), now.Add(8*time.Second))
	if quest.Golden.Active || quest.GoldenCollected != 1 {
		t.Fatalf("golden state active=%t collected=%d", quest.Golden.Active, quest.GoldenCollected)
	}

	full := NewSnakeGame(1, 1, func(int) int { return 0 })
	full.Snake = []Point{{X: 0, Y: 0}}
	full.Food = Point{X: -1, Y: -1}
	quest = NewQuestState(GameModeQuest, now, fixedRandom(0), 1, 1)
	for i := 0; i < 7; i++ {
		quest.OnNormalFood(full, HeatByLevel(1), now)
	}
	if quest.Golden.Active {
		t.Fatal("golden byte spawned on full board")
	}
}

func TestQuestGoldenByteUsesInjectedRandomSequence(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(4, 4, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 1}, {X: 1, Y: 2}}
	game.Food = Point{X: 0, Y: 0}
	random := &sequenceRandom{values: []int{0, 0, 1}}
	quest := NewQuestState(GameModeQuest, now, random, 4, 4)

	quest.NormalFood = 6
	quest.OnNormalFood(game, HeatByLevel(1), now)
	if !quest.Golden.Active {
		t.Fatal("first golden byte did not spawn")
	}
	first := quest.Golden.Position

	quest.Golden.Active = false
	quest.NormalFood = 13
	quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Second))
	if !quest.Golden.Active {
		t.Fatal("second golden byte did not spawn")
	}
	second := quest.Golden.Position

	if first == second {
		t.Fatalf("golden byte positions = %#v and %#v, want sequence-selected candidates", first, second)
	}
}

func TestQuestMissionSelectionProgressAndBonusOnce(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Mission = Mission{ID: MissionCombo5, Label: "Reach combo x5", Target: 5}

	for i := 0; i < 5; i++ {
		quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Duration(i)*time.Second))
	}
	if !quest.MissionCompleted || quest.MissionProgress != 5 {
		t.Fatalf("mission completed=%t progress=%d", quest.MissionCompleted, quest.MissionProgress)
	}
	first := quest.Complete(game, HeatByLevel(3), now.Add(10*time.Second))
	second := quest.Complete(game, HeatByLevel(3), now.Add(11*time.Second))
	if first.MissionBonus != missionBonus || second.MissionBonus != missionBonus {
		t.Fatalf("mission bonus first=%d second=%d", first.MissionBonus, second.MissionBonus)
	}
}

func TestQuestMissionSelectionHonorsInjectedIndex(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	quest := NewQuestState(GameModeQuest, now, fixedRandom(2), 8, 8)

	if quest.Mission.ID != MissionSurvive60 {
		t.Fatalf("Mission.ID = %q, want %q", quest.Mission.ID, MissionSurvive60)
	}
}

func TestQuestUpgradeChoicesAndEffects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)

	choices := PickUpgradeChoices(fixedRandom(0))
	if len(choices) != 3 || choices[0].Upgrade == choices[1].Upgrade || choices[1].Upgrade == choices[2].Upgrade || choices[0].Upgrade == choices[2].Upgrade {
		t.Fatalf("choices are not unique: %#v", choices)
	}
	quest.PendingChoices = []UpgradeChoice{{Upgrade: UpgradeShield}}
	if !quest.ApplyUpgrade(0, now) || quest.ShieldCharges != 1 {
		t.Fatalf("shield upgrade not applied: %#v", quest)
	}
	game.Over = true
	if !quest.TryShieldRecovery(game) || game.Over || quest.ShieldCharges != 0 {
		t.Fatalf("shield recovery failed: over=%t charges=%d", game.Over, quest.ShieldCharges)
	}
	quest.PendingChoices = []UpgradeChoice{{Upgrade: UpgradeSlowClock}}
	quest.ApplyUpgrade(0, now)
	if got := quest.EffectiveInterval(85*time.Millisecond, now.Add(time.Second)); got < normalMinimumSpeed {
		t.Fatalf("slow interval = %s, want at least %s", got, normalMinimumSpeed)
	}
	quest.PendingChoices = []UpgradeChoice{{Upgrade: UpgradeDoubleByte}}
	quest.ApplyUpgrade(0, now)
	quest.OnNormalFood(game, HeatByLevel(1), now)
	if quest.DoubleCharges != 2 {
		t.Fatalf("double charges = %d, want 2", quest.DoubleCharges)
	}
}

func TestQuestUpgradeOrderingHonorsInjectedIndices(t *testing.T) {
	choices := PickUpgradeChoices(&sequenceRandom{values: []int{2, 0, 0}})

	want := []Upgrade{UpgradeDoubleByte, UpgradeSlowClock, UpgradeShield}
	for index, upgrade := range want {
		if choices[index].Upgrade != upgrade {
			t.Fatalf("choices[%d] = %q, want %q; choices=%#v", index, choices[index].Upgrade, upgrade, choices)
		}
	}
}

func TestQuestCompletionSurvivalBonusIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.BaseScore = 100
	game.Score = 100

	first := quest.Complete(game, HeatByLevel(6), now)
	second := quest.Complete(game, HeatByLevel(6), now.Add(time.Second))
	if first.SurvivalBonus != survivalBonus || second.SurvivalBonus != survivalBonus {
		t.Fatalf("survival bonus first=%d second=%d", first.SurvivalBonus, second.SurvivalBonus)
	}
	if first.FinalScore != second.FinalScore {
		t.Fatalf("final score changed from %d to %d", first.FinalScore, second.FinalScore)
	}
}

type fixedRandom int

func (r fixedRandom) Intn(max int) int {
	return int(r)
}

type sequenceRandom struct {
	values []int
	index  int
}

func (r *sequenceRandom) Intn(max int) int {
	if len(r.values) == 0 {
		return 0
	}
	value := r.values[r.index%len(r.values)]
	r.index++
	return value
}
