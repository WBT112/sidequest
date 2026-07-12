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

func TestQuestPickupSpawnsOnFifthNormalFood(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}}
	game.Food = Point{X: 0, Y: 0}
	quest := NewQuestState(GameModeQuest, now, &sequenceRandom{values: []int{0, 0, 0}}, 8, 8)

	for index := 0; index < 4; index++ {
		quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Duration(index)*time.Second))
		if quest.Pickup.Active {
			t.Fatalf("pickup spawned after %d foods, want no pickup before fifth", index+1)
		}
	}
	quest.OnNormalFood(game, HeatByLevel(1), now.Add(5*time.Second))
	if !quest.Pickup.Active || quest.Pickup.Upgrade != UpgradeShield {
		t.Fatalf("pickup = %#v, want active shield", quest.Pickup)
	}
}

func TestQuestPickupSelectionCanSelectEachType(t *testing.T) {
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}}

	for index, upgrade := range pickupPool {
		selected, ok := selectPickup(fixedRandom(index), availablePickups(game))
		if !ok {
			t.Fatalf("selectPickup(%d) returned false", index)
		}
		if selected != upgrade {
			t.Fatalf("selectPickup(%d) = %q, want %q", index, selected, upgrade)
		}
	}
}

func TestQuestPickupSymbolsAreDistinctAndNamed(t *testing.T) {
	seen := map[string]Upgrade{}
	for _, upgrade := range pickupPool {
		symbol := PickupSymbol(upgrade)
		if len([]rune(symbol)) != 2 {
			t.Fatalf("PickupSymbol(%q) = %q, want two runes", upgrade, symbol)
		}
		if previous, exists := seen[symbol]; exists {
			t.Fatalf("symbol %q used by both %q and %q", symbol, previous, upgrade)
		}
		seen[symbol] = upgrade
		if PickupName(upgrade) == "" || PickupName(upgrade) == "Double Byte" {
			t.Fatalf("PickupName(%q) = %q", upgrade, PickupName(upgrade))
		}
	}
	if PickupSymbol(UpgradeDoubleScore) == "<>" {
		t.Fatal("Double Score symbol conflicts with Golden Byte")
	}
}

func TestQuestFiltersUnavailablePatchPickup(t *testing.T) {
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}}

	for _, upgrade := range availablePickups(game) {
		if upgrade == UpgradePatch {
			t.Fatal("Patch was available for minimum-length snake")
		}
	}
}

func TestQuestPickupPlacementExcludesOccupiedCells(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}}
	game.Food = Point{X: 2, Y: 2}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 5, Y: 5}, ExpiresAt: now.Add(time.Second)}

	if !quest.spawnPickup(game, now) {
		t.Fatal("spawnPickup returned false")
	}
	if quest.Pickup.Position == game.Food || quest.Pickup.Position == quest.Golden.Position {
		t.Fatalf("pickup spawned on occupied point %#v", quest.Pickup.Position)
	}
	for _, point := range game.Snake {
		if quest.Pickup.Position == point {
			t.Fatalf("pickup spawned on snake at %#v", point)
		}
	}
}

func TestQuestPickupExpires(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}

	quest.Tick(game, HeatByLevel(1), now.Add(pickupTTL))

	if quest.Pickup.Active {
		t.Fatal("pickup remained active after TTL")
	}
}

func TestQuestPickupCollectionAppliesEffects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)

	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.OnPickupCollected(game, HeatByLevel(1), now)
	if quest.Shield.Charges != 1 || !quest.Shield.ExpiresAt.Equal(now.Add(shieldDuration)) {
		t.Fatalf("shield = %#v, want one timed charge", quest.Shield)
	}
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeDoubleScore, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.OnPickupCollected(game, HeatByLevel(1), now)
	if quest.DoubleCharges != doubleScoreCharges || !quest.DoubleUntil.Equal(now.Add(doubleScoreDuration)) {
		t.Fatalf("double charges=%d until=%s", quest.DoubleCharges, quest.DoubleUntil)
	}
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradePatch, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}, {X: 2, Y: 4}, {X: 1, Y: 4}}
	quest.OnPickupCollected(game, HeatByLevel(1), now)
	if len(game.Snake) != 1 {
		t.Fatalf("snake length after patch = %d, want 1", len(game.Snake))
	}
}

func TestQuestDoubleScoreCapTimeoutAndNormalFoodOnly(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.DoubleCharges = doubleScoreCap - 1
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeDoubleScore, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.OnPickupCollected(game, HeatByLevel(1), now)
	if quest.DoubleCharges != doubleScoreCap {
		t.Fatalf("DoubleCharges = %d, want cap %d", quest.DoubleCharges, doubleScoreCap)
	}

	quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Second))
	if quest.DoubleCharges != doubleScoreCap-1 || game.Score != 20 {
		t.Fatalf("after normal food charges=%d score=%d, want one doubled food", quest.DoubleCharges, game.Score)
	}
	quest.Golden = GoldenByte{Active: true}
	quest.OnGoldenByte(game, HeatByLevel(1), now.Add(2*time.Second))
	if quest.DoubleCharges != doubleScoreCap-1 {
		t.Fatalf("Golden Byte consumed Double Score charge, charges=%d", quest.DoubleCharges)
	}
	quest.Tick(game, HeatByLevel(1), now.Add(doubleScoreDuration))
	if quest.DoubleCharges != 0 {
		t.Fatalf("DoubleCharges = %d after timeout, want 0", quest.DoubleCharges)
	}
}

func TestQuestTimedEffectsRefreshAndExpire(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)

	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeSlowClock, ExpiresAt: now.Add(pickupTTL)}
	quest.OnPickupCollected(game, HeatByLevel(1), now)
	firstSlow := quest.SlowUntil
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeSlowClock, ExpiresAt: now.Add(pickupTTL)}
	quest.OnPickupCollected(game, HeatByLevel(1), now.Add(time.Second))
	if !quest.SlowUntil.After(firstSlow) {
		t.Fatalf("SlowUntil did not refresh: first=%s second=%s", firstSlow, quest.SlowUntil)
	}
	if got := quest.EffectiveInterval(85*time.Millisecond, now.Add(2*time.Second)); got < normalMinimumSpeed {
		t.Fatalf("slow interval = %s, want at least %s", got, normalMinimumSpeed)
	}

	turboQuest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	turboQuest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeTurbo, ExpiresAt: now.Add(pickupTTL)}
	turboQuest.OnPickupCollected(game, HeatByLevel(1), now)
	if got := turboQuest.EffectiveInterval(100*time.Millisecond, now.Add(time.Second)); got < turboMinimumSpeed || got >= 100*time.Millisecond {
		t.Fatalf("turbo interval = %s, want faster but safe", got)
	}
	turboQuest.Tick(game, HeatByLevel(1), now.Add(turboDuration))
	if !turboQuest.TurboUntil.IsZero() {
		t.Fatal("turbo remained active after timeout")
	}
}

func TestQuestComboKeeperFreezesComboExpiry(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Combo = 3
	quest.ComboExpiresAt = now.Add(time.Second)
	quest.ComboKeeperUntil = now.Add(comboKeeperDuration)

	quest.Tick(game, HeatByLevel(1), now.Add(5*time.Second))
	if quest.Combo != 3 {
		t.Fatalf("combo = %d, want preserved by keeper", quest.Combo)
	}
	quest.Tick(game, HeatByLevel(1), now.Add(comboKeeperDuration))
	if quest.Combo != 3 || !quest.ComboExpiresAt.After(now.Add(comboKeeperDuration)) {
		t.Fatalf("combo=%d expires=%s, want restarted combo window", quest.Combo, quest.ComboExpiresAt)
	}
}

func TestQuestCollisionEffectsUsePriority(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(6, 6, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 3, Y: 2}, {X: 1, Y: 2}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 0}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 6, 6)
	quest.Phase = TimedCharge{Charges: 1, ExpiresAt: now.Add(phaseDuration)}
	quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(shieldDuration)}
	quest.Warp = TimedCharge{Charges: 1, ExpiresAt: now.Add(warpDuration)}

	result := game.Step()
	if result != StepHitSelf {
		t.Fatalf("Step result = %v, want self collision", result)
	}
	result = quest.TryCollisionEffects(game, result, now)
	if result != StepMoved || game.Over || game.Snake[0] != (Point{X: 4, Y: 2}) {
		t.Fatalf("phase result=%v over=%t head=%#v", result, game.Over, game.Snake[0])
	}
	if quest.Phase.Charges != 0 || quest.Shield.Charges != 1 || quest.Warp.Charges != 1 {
		t.Fatalf("charges after priority phase: phase=%#v shield=%#v warp=%#v", quest.Phase, quest.Shield, quest.Warp)
	}
}

func TestQuestPhaseUnsafeFallbackKeepsCollision(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(1, 1, func(int) int { return 0 })
	game.Snake = []Point{{X: 0, Y: 0}}
	game.Dir = DirectionRight
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 1, 1)
	quest.Phase = TimedCharge{Charges: 1, ExpiresAt: now.Add(phaseDuration)}

	result := game.Step()
	result = quest.TryCollisionEffects(game, result, now)

	if result != StepHitWall || !game.Over || quest.Phase.Charges != 1 {
		t.Fatalf("phase fallback result=%v over=%t charges=%d", result, game.Over, quest.Phase.Charges)
	}
}

func TestQuestWarpUsesSafeDestinationAndFallback(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(4, 4, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 1}, {X: 2, Y: 1}}
	game.Dir = DirectionRight
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 4, 4)
	quest.Warp = TimedCharge{Charges: 1, ExpiresAt: now.Add(warpDuration)}

	result := game.Step()
	result = quest.TryCollisionEffects(game, result, now)
	if result != StepMoved || game.Over || quest.Warp.Charges != 0 {
		t.Fatalf("warp failed: result=%v over=%t charges=%d", result, game.Over, quest.Warp.Charges)
	}
	if game.Snake[0].X < 0 || game.Snake[0].X >= game.Width || game.Snake[0].Y < 0 || game.Snake[0].Y >= game.Height {
		t.Fatalf("warp head out of bounds: %#v", game.Snake[0])
	}

	full := NewSnakeGame(1, 1, func(int) int { return 0 })
	full.Snake = []Point{{X: 0, Y: 0}}
	full.Dir = DirectionRight
	quest.Warp = TimedCharge{Charges: 1, ExpiresAt: now.Add(warpDuration)}
	result = full.Step()
	result = quest.TryCollisionEffects(full, result, now)
	if result != StepHitWall || !full.Over || quest.Warp.Charges != 1 {
		t.Fatalf("warp fallback result=%v over=%t charges=%d", result, full.Over, quest.Warp.Charges)
	}
}

func TestQuestCrashAndCompletionClearRoundEffects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeWarp, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(shieldDuration)}
	quest.Phase = TimedCharge{Charges: 1, ExpiresAt: now.Add(phaseDuration)}
	quest.Warp = TimedCharge{Charges: 1, ExpiresAt: now.Add(warpDuration)}
	quest.SlowUntil = now.Add(slowClockDuration)
	quest.DoubleCharges = 2
	quest.DoubleUntil = now.Add(doubleScoreDuration)
	quest.ComboKeeperUntil = now.Add(comboKeeperDuration)
	quest.TurboUntil = now.Add(turboDuration)

	quest.OnCrash()
	if quest.Pickup.Active || quest.Shield.Charges != 0 || quest.Phase.Charges != 0 || quest.Warp.Charges != 0 || quest.DoubleCharges != 0 || !quest.SlowUntil.IsZero() || !quest.ComboKeeperUntil.IsZero() || !quest.TurboUntil.IsZero() {
		t.Fatalf("effects not cleared after crash: %#v", quest)
	}

	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(shieldDuration)}
	quest.Complete(game, HeatByLevel(1), now)
	if quest.Pickup.Active || quest.Shield.Charges != 0 {
		t.Fatalf("effects not cleared after completion: pickup=%#v shield=%#v", quest.Pickup, quest.Shield)
	}
}

func TestQuestResizeObjectsRelocatesInvalidPickupWithoutTimerReset(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	expiresAt := now.Add(pickupTTL)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 7, Y: 7}, ExpiresAt: expiresAt}

	game.Resize(2, 2)
	quest.ResizeObjects(game)

	if !quest.Pickup.Active {
		t.Fatal("pickup was cleared after resize, want relocated")
	}
	if quest.Pickup.ExpiresAt != expiresAt {
		t.Fatalf("pickup expiry = %s, want preserved %s", quest.Pickup.ExpiresAt, expiresAt)
	}
	if quest.Pickup.Position.X < 0 || quest.Pickup.Position.X >= game.Width || quest.Pickup.Position.Y < 0 || quest.Pickup.Position.Y >= game.Height {
		t.Fatalf("pickup relocated out of bounds: %#v", quest.Pickup.Position)
	}
}

func TestQuestFoodPlacementExcludesActivePickup(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Food = Point{X: 3, Y: 2}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 5, 5)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: game.Food, ExpiresAt: now.Add(pickupTTL)}

	if !quest.EnsureFood(game) {
		t.Fatal("EnsureFood returned false")
	}
	if game.Food == quest.Pickup.Position {
		t.Fatalf("food remained on pickup at %#v", game.Food)
	}
}

func TestQuestFoodPlacementExcludesActiveGoldenByte(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Food = Point{X: 3, Y: 2}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 5, 5)
	quest.Golden = GoldenByte{Active: true, Position: game.Food, ExpiresAt: now.Add(goldenByteTTL)}

	if !quest.EnsureFood(game) {
		t.Fatal("EnsureFood returned false")
	}
	if game.Food == quest.Golden.Position {
		t.Fatalf("food remained on Golden Byte at %#v", game.Food)
	}
}

func TestQuestFoodPlacementExcludesPickupAndGoldenByte(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(3, 2, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 0}, {X: 0, Y: 0}, {X: 0, Y: 1}}
	game.Food = Point{X: 1, Y: 1}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 3, 2)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 2, Y: 1}, ExpiresAt: now.Add(goldenByteTTL)}

	if !quest.EnsureFood(game) {
		t.Fatal("EnsureFood returned false on nearly full board")
	}
	if game.Food == quest.Pickup.Position || game.Food == quest.Golden.Position {
		t.Fatalf("food placed on quest object at %#v", game.Food)
	}
	if game.Food != (Point{X: 2, Y: 0}) {
		t.Fatalf("Food = %#v, want only reachable free cell 2,0", game.Food)
	}
}

func TestQuestResizeObjectsKeepsFoodDistinctFromQuestObjects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Food = Point{X: 3, Y: 2}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 5, 5)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: game.Food, ExpiresAt: now.Add(pickupTTL)}

	quest.ResizeObjects(game)

	if game.Food == quest.Pickup.Position {
		t.Fatalf("food overlaps pickup after resize handling at %#v", game.Food)
	}
}

func TestStepGameCollectsPickupWithoutBlockingMovement(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 0}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 4, Y: 3}, ExpiresAt: now.Add(pickupTTL)}

	result := stepGame(game, quest, HeatByLevel(1), now)

	if result != StepMoved || game.Snake[0] != (Point{X: 4, Y: 3}) || quest.Pickup.Active || quest.Shield.Charges != 1 {
		t.Fatalf("pickup collection result=%v head=%#v pickup=%#v shield=%#v", result, game.Snake[0], quest.Pickup, quest.Shield)
	}
}

func TestStepGameCollectsPickupWithoutNormalFoodPointsOnOverlap(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}}
	game.Dir = DirectionRight
	game.Food = Point{X: 4, Y: 3}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 4, Y: 3}, ExpiresAt: now.Add(pickupTTL)}

	result := stepGame(game, quest, HeatByLevel(1), now)

	if result != StepMoved {
		t.Fatalf("stepGame result = %v, want pickup-only movement", result)
	}
	if game.Score != 0 || quest.BaseScore != 0 || quest.NormalFood != 0 {
		t.Fatalf("pickup overlap awarded normal food: game score=%d base=%d normal=%d", game.Score, quest.BaseScore, quest.NormalFood)
	}
	if quest.Pickup.Active || quest.Shield.Charges != 1 {
		t.Fatalf("pickup was not collected correctly: pickup=%#v shield=%#v", quest.Pickup, quest.Shield)
	}
	if game.Food == game.Snake[0] {
		t.Fatalf("food remains under snake head at %#v", game.Food)
	}
}

func TestStepGameCollectsNormalFoodWithoutConsumingQuestObjects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}}
	game.Dir = DirectionRight
	game.Food = Point{X: 4, Y: 3}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 6, Y: 6}, ExpiresAt: now.Add(pickupTTL)}
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 7, Y: 7}, ExpiresAt: now.Add(goldenByteTTL)}

	result := stepGame(game, quest, HeatByLevel(1), now)

	if result != StepAteFood {
		t.Fatalf("stepGame result = %v, want normal food", result)
	}
	if !quest.Pickup.Active || !quest.Golden.Active {
		t.Fatalf("normal food consumed quest object: pickup=%#v golden=%#v", quest.Pickup, quest.Golden)
	}
	if game.Score != quest.BaseScore || quest.NormalFood != 1 {
		t.Fatalf("score state mismatch after normal food: game=%d base=%d normal=%d", game.Score, quest.BaseScore, quest.NormalFood)
	}
	if game.Food == quest.Pickup.Position || game.Food == quest.Golden.Position {
		t.Fatalf("respawned food overlaps quest object at %#v", game.Food)
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
