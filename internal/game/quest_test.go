package game

import (
	"fmt"
	"reflect"
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

func TestQuestComboDurationScalesWithCombo(t *testing.T) {
	if got := comboDuration(1); got >= comboWindow {
		t.Fatalf("comboDuration(1) = %s, want faster than %s", got, comboWindow)
	}
	if got := comboDuration(4); got != comboWindow {
		t.Fatalf("comboDuration(4) = %s, want baseline %s", got, comboWindow)
	}
	if got := comboDuration(8); got <= comboWindow {
		t.Fatalf("comboDuration(8) = %s, want slower than %s", got, comboWindow)
	}
	if got := comboDuration(20); got != maxComboWindow {
		t.Fatalf("comboDuration(20) = %s, want cap %s", got, maxComboWindow)
	}
}

func TestQuestGoldenByteSpawnCollectAndFullBoard(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(4, 4, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 4, 4)
	for i := 0; i < goldenSpawnMin; i++ {
		quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Duration(i)*time.Second))
	}
	if !quest.Golden.Active {
		t.Fatal("golden byte did not spawn after minimum food target")
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
	for i := 0; i < goldenSpawnMin; i++ {
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
	for i := 0; i < goldenSpawnMin; i++ {
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
	random := &sequenceRandom{values: []int{0, 0, 0, 2}}
	quest := NewQuestState(GameModeQuest, now, random, 4, 4)
	quest.NextPickupFood = 100
	quest.NextGoldenFood = 7

	quest.NormalFood = 6
	quest.OnNormalFood(game, HeatByLevel(1), now)
	if !quest.Golden.Active {
		t.Fatal("first golden byte did not spawn")
	}
	first := quest.Golden.Position

	quest.Golden.Active = false
	quest.NextGoldenFood = 14
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

func TestQuestPickupSpawnsOnRandomFoodTarget(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}}
	game.Food = Point{X: 0, Y: 0}
	quest := NewQuestState(GameModeQuest, now, &sequenceRandom{values: []int{0, 0, 0}}, 8, 8)

	for index := 0; index < 3; index++ {
		quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Duration(index)*time.Second))
		if quest.Pickup.Active {
			t.Fatalf("pickup spawned after %d foods, want no pickup before fourth", index+1)
		}
	}
	quest.OnNormalFood(game, HeatByLevel(1), now.Add(4*time.Second))
	if !quest.Pickup.Active || quest.Pickup.Upgrade != UpgradeShield {
		t.Fatalf("pickup = %#v, want active shield", quest.Pickup)
	}
}

func TestQuestPickupAndGoldenSpawnTargetsUseRandomRanges(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(10, 10, func(int) int { return 0 })
	game.Food = Point{X: 0, Y: 0}
	quest := NewQuestState(GameModeQuest, now, &sequenceRandom{values: []int{0, pickupSpawnMax - pickupSpawnMin, goldenSpawnMax - goldenSpawnMin}}, 10, 10)

	quest.OnNormalFood(game, HeatByLevel(1), now)

	if quest.NextPickupFood != pickupSpawnMax {
		t.Fatalf("NextPickupFood = %d, want max target %d", quest.NextPickupFood, pickupSpawnMax)
	}
	if quest.NextGoldenFood != goldenSpawnMax {
		t.Fatalf("NextGoldenFood = %d, want max target %d", quest.NextGoldenFood, goldenSpawnMax)
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

func TestQuestDoubleScoreRefreshesTimeoutAndNormalFoodOnly(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.DoubleCharges = doubleScoreCap - 1
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeDoubleScore, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	quest.OnPickupCollected(game, HeatByLevel(1), now)
	if quest.DoubleCharges != doubleScoreCharges {
		t.Fatalf("DoubleCharges = %d, want refreshed charges %d", quest.DoubleCharges, doubleScoreCharges)
	}

	quest.OnNormalFood(game, HeatByLevel(1), now.Add(time.Second))
	if quest.DoubleCharges != doubleScoreCharges-1 || game.Score != 20 {
		t.Fatalf("after normal food charges=%d score=%d, want one doubled food", quest.DoubleCharges, game.Score)
	}
	quest.Golden = GoldenByte{Active: true}
	quest.OnGoldenByte(game, HeatByLevel(1), now.Add(2*time.Second))
	if quest.DoubleCharges != doubleScoreCharges-1 {
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
	if got := turboQuest.EffectiveInterval(180*time.Millisecond, now.Add(time.Second)); got != 117*time.Millisecond {
		t.Fatalf("turbo interval = %s, want noticeable speed boost", got)
	}
	if got := turboQuest.EffectiveInterval(80*time.Millisecond, now.Add(time.Second)); got != turboMinimumSpeed {
		t.Fatalf("turbo capped interval = %s, want %s", got, turboMinimumSpeed)
	}
	turboQuest.Tick(game, HeatByLevel(1), now.Add(turboDuration))
	if !turboQuest.TurboUntil.IsZero() {
		t.Fatal("turbo remained active after timeout")
	}
}

func TestQuestEffectHUDPartsSeparatesChargesAndDuration(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(29 * time.Second)}
	quest.Phase = TimedCharge{Charges: 1, ExpiresAt: now.Add(14 * time.Second)}
	quest.Warp = TimedCharge{Charges: 1, ExpiresAt: now.Add(22 * time.Second)}
	quest.DoubleCharges = 3
	quest.DoubleUntil = now.Add(18 * time.Second)

	got := quest.effectHUDParts(now)
	want := []string{
		"SHIELD x1 29s",
		"PHASE x1 14s",
		"WARP x1 22s",
		"DOUBLE x3 18s",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectHUDParts = %#v, want %#v", got, want)
	}
}

func TestQuestEffectHUDPartsKeepsDurationOnlyEffectsReadable(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.SlowUntil = now.Add(11 * time.Second)
	quest.ComboKeeperUntil = now.Add(17 * time.Second)
	quest.TurboUntil = now.Add(9 * time.Second)

	got := quest.effectHUDParts(now)
	want := []string{
		"SLOW 11s",
		"COMBO LOCK 17s",
		"TURBO 9s",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectHUDParts = %#v, want %#v", got, want)
	}
}

func TestQuestEffectHUDPartsCeilActiveCountdowns(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		remaining time.Duration
		want      string
	}{
		{name: "one and a half seconds", remaining: 1500 * time.Millisecond, want: "2s"},
		{name: "one second", remaining: time.Second, want: "1s"},
		{name: "half second", remaining: 500 * time.Millisecond, want: "1s"},
		{name: "one millisecond", remaining: time.Millisecond, want: "1s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
			deadline := now.Add(test.remaining)
			quest.Shield = TimedCharge{Charges: 1, ExpiresAt: deadline}
			quest.Phase = TimedCharge{Charges: 1, ExpiresAt: deadline}
			quest.Warp = TimedCharge{Charges: 1, ExpiresAt: deadline}
			quest.DoubleCharges = 2
			quest.DoubleUntil = deadline
			quest.SlowUntil = deadline
			quest.ComboKeeperUntil = deadline
			quest.TurboUntil = deadline

			got := quest.effectHUDParts(now)
			want := []string{
				fmt.Sprintf("SHIELD x1 %s", test.want),
				fmt.Sprintf("PHASE x1 %s", test.want),
				fmt.Sprintf("WARP x1 %s", test.want),
				fmt.Sprintf("DOUBLE x2 %s", test.want),
				fmt.Sprintf("SLOW %s", test.want),
				fmt.Sprintf("COMBO LOCK %s", test.want),
				fmt.Sprintf("TURBO %s", test.want),
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("effectHUDParts = %#v, want %#v", got, want)
			}
		})
	}
}

func TestQuestEffectHUDPartsHidesExpiredCountdowns(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now}
	quest.Phase = TimedCharge{Charges: 1, ExpiresAt: now}
	quest.Warp = TimedCharge{Charges: 1, ExpiresAt: now}
	quest.DoubleCharges = 2
	quest.DoubleUntil = now
	quest.SlowUntil = now
	quest.ComboKeeperUntil = now
	quest.TurboUntil = now

	if got := quest.effectHUDParts(now); len(got) != 0 {
		t.Fatalf("effectHUDParts = %#v, want no active countdowns", got)
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

func TestQuestShieldRecoveryPreservesSnakeLength(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(6, 6, func(int) int { return 0 })
	game.Snake = []Point{{X: 5, Y: 2}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}}
	game.Dir = DirectionRight
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 6, 6)
	quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(shieldDuration)}

	result := game.Step()
	result = quest.TryCollisionEffects(game, result, now)

	if result != StepMoved || game.Over || quest.Shield.Charges != 0 {
		t.Fatalf("shield result=%v over=%t charges=%d", result, game.Over, quest.Shield.Charges)
	}
	if len(game.Snake) != 4 {
		t.Fatalf("snake length after shield = %d, want preserved length 4", len(game.Snake))
	}
	assertSnakeValid(t, game.Snake, game.Width, game.Height)
}

func TestQuestShieldRecoveryReconcilesBoardObjects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	recoveredHead := Point{X: 3, Y: 3}

	tests := []struct {
		name  string
		setup func(*SnakeGame, *QuestState) time.Time
		check func(*testing.T, *SnakeGame, *QuestState, time.Time)
	}{
		{
			name: "normal food",
			setup: func(game *SnakeGame, quest *QuestState) time.Time {
				game.Food = recoveredHead
				return time.Time{}
			},
			check: func(t *testing.T, game *SnakeGame, quest *QuestState, _ time.Time) {
				t.Helper()
				if !game.FoodValid(quest.ActiveObjectPoints()) {
					t.Fatalf("food invalid after shield recovery: food=%#v snake=%#v", game.Food, game.Snake)
				}
			},
		},
		{
			name: "golden byte",
			setup: func(game *SnakeGame, quest *QuestState) time.Time {
				expiresAt := now.Add(goldenByteTTL)
				quest.Golden = GoldenByte{Active: true, Position: recoveredHead, ExpiresAt: expiresAt}
				return expiresAt
			},
			check: func(t *testing.T, game *SnakeGame, quest *QuestState, expiresAt time.Time) {
				t.Helper()
				if !quest.Golden.Active || quest.Golden.ExpiresAt != expiresAt || game.Occupies(quest.Golden.Position) {
					t.Fatalf("golden not safely reconciled: golden=%#v snake=%#v", quest.Golden, game.Snake)
				}
			},
		},
		{
			name: "pickup",
			setup: func(game *SnakeGame, quest *QuestState) time.Time {
				expiresAt := now.Add(pickupTTL)
				quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: recoveredHead, ExpiresAt: expiresAt}
				return expiresAt
			},
			check: func(t *testing.T, game *SnakeGame, quest *QuestState, expiresAt time.Time) {
				t.Helper()
				if !quest.Pickup.Active || quest.Pickup.ExpiresAt != expiresAt || game.Occupies(quest.Pickup.Position) {
					t.Fatalf("pickup not safely reconciled: pickup=%#v snake=%#v", quest.Pickup, game.Snake)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			game := NewSnakeGame(6, 6, func(int) int { return 0 })
			game.Snake = []Point{{X: 5, Y: 2}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}}
			game.Dir = DirectionRight
			quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 6, 6)
			quest.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(shieldDuration)}
			expiresAt := test.setup(game, quest)

			result := game.Step()
			result = quest.TryCollisionEffects(game, result, now)

			if result != StepMoved || game.Over || quest.Shield.Charges != 0 {
				t.Fatalf("shield result=%v over=%t charges=%d", result, game.Over, quest.Shield.Charges)
			}
			assertQuestObjectsValid(t, game, quest)
			test.check(t, game, quest, expiresAt)
		})
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

func TestQuestPhaseRecoveryReconcilesBoardObjects(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	phaseTarget := Point{X: 0, Y: 2}

	tests := []struct {
		name  string
		setup func(*SnakeGame, *QuestState) time.Time
		check func(*testing.T, *SnakeGame, *QuestState, time.Time)
	}{
		{
			name: "normal food",
			setup: func(game *SnakeGame, quest *QuestState) time.Time {
				game.Food = phaseTarget
				return time.Time{}
			},
			check: func(t *testing.T, game *SnakeGame, quest *QuestState, _ time.Time) {
				t.Helper()
				if game.Food == phaseTarget || !game.FoodValid(quest.ActiveObjectPoints()) {
					t.Fatalf("food not safely reconciled: food=%#v snake=%#v", game.Food, game.Snake)
				}
			},
		},
		{
			name: "golden byte",
			setup: func(game *SnakeGame, quest *QuestState) time.Time {
				expiresAt := now.Add(goldenByteTTL)
				quest.Golden = GoldenByte{Active: true, Position: phaseTarget, ExpiresAt: expiresAt}
				return expiresAt
			},
			check: func(t *testing.T, game *SnakeGame, quest *QuestState, expiresAt time.Time) {
				t.Helper()
				if !quest.Golden.Active || quest.Golden.ExpiresAt != expiresAt || quest.Golden.Position == phaseTarget {
					t.Fatalf("golden not safely reconciled: %#v", quest.Golden)
				}
			},
		},
		{
			name: "pickup",
			setup: func(game *SnakeGame, quest *QuestState) time.Time {
				expiresAt := now.Add(pickupTTL)
				quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: phaseTarget, ExpiresAt: expiresAt}
				return expiresAt
			},
			check: func(t *testing.T, game *SnakeGame, quest *QuestState, expiresAt time.Time) {
				t.Helper()
				if !quest.Pickup.Active || quest.Pickup.ExpiresAt != expiresAt || quest.Pickup.Position == phaseTarget {
					t.Fatalf("pickup not safely reconciled: %#v", quest.Pickup)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			game := NewSnakeGame(5, 5, func(int) int { return 0 })
			game.Snake = []Point{{X: 4, Y: 2}, {X: 3, Y: 2}}
			game.Dir = DirectionRight
			game.Food = Point{X: 1, Y: 1}
			quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 5, 5)
			quest.Phase = TimedCharge{Charges: 1, ExpiresAt: now.Add(phaseDuration)}
			expiresAt := test.setup(game, quest)

			result := game.Step()
			result = quest.TryCollisionEffects(game, result, now)

			if result != StepMoved || game.Over || quest.Phase.Charges != 0 || game.Snake[0] != phaseTarget {
				t.Fatalf("phase result=%v over=%t charges=%d head=%#v", result, game.Over, quest.Phase.Charges, game.Snake[0])
			}
			assertQuestObjectsValid(t, game, quest)
			test.check(t, game, quest, expiresAt)
		})
	}
}

func TestQuestObjectReconciliationClearsObjectsWithoutFreeCells(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(2, 1, func(int) int { return 0 })
	game.Snake = []Point{{X: 0, Y: 0}, {X: 1, Y: 0}}
	game.Food = Point{X: 0, Y: 0}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 2, 1)
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 0, Y: 0}, ExpiresAt: now.Add(goldenByteTTL)}
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 1, Y: 0}, ExpiresAt: now.Add(pickupTTL)}

	quest.ResizeObjects(game)

	if quest.Golden.Active || quest.Pickup.Active {
		t.Fatalf("quest objects remained without free cells: golden=%#v pickup=%#v", quest.Golden, quest.Pickup)
	}
	if game.Food.X >= 0 || game.Food.Y >= 0 {
		t.Fatalf("food remained without free cells: %#v", game.Food)
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
	if len(game.Snake) != 2 {
		t.Fatalf("snake length after warp = %d, want preserved length 2", len(game.Snake))
	}
	assertSnakeValid(t, game.Snake, game.Width, game.Height)
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

func TestStepGameRetriesMissingFoodAfterQuestMovement(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(4, 3, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 1}, {X: 0, Y: 1}}
	game.Dir = DirectionRight
	game.Food = Point{X: -1, Y: -1}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 4, 3)
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 2, Y: 2}, ExpiresAt: now.Add(pickupTTL)}
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 3, Y: 2}, ExpiresAt: now.Add(goldenByteTTL)}

	result := stepGame(game, quest, HeatByLevel(1), now)

	if result != StepMoved {
		t.Fatalf("stepGame result = %v, want movement", result)
	}
	if !game.FoodValid(quest.ActiveObjectPoints()) {
		t.Fatalf("Food = %#v, want valid food excluding quest objects pickup=%#v golden=%#v", game.Food, quest.Pickup.Position, quest.Golden.Position)
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

func TestStepGameCollectsGoldenBytePreservesNormalFood(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(8, 8, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}}
	game.Dir = DirectionRight
	game.Food = Point{X: 1, Y: 1}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 8, 8)
	quest.Mission = Mission{ID: MissionGolden2, Label: "Collect 2 Golden Bytes", Target: 2}
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 4, Y: 3}, ExpiresAt: now.Add(goldenByteTTL)}
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 6, Y: 6}, ExpiresAt: now.Add(pickupTTL)}
	quest.DoubleCharges = 3
	quest.DoubleUntil = now.Add(doubleScoreDuration)
	beforeLength := len(game.Snake)

	result := stepGame(game, quest, HeatByLevel(1), now)

	if result != StepAteFood {
		t.Fatalf("stepGame result = %v, want golden-byte growth", result)
	}
	if game.Snake[0] != (Point{X: 4, Y: 3}) || len(game.Snake) != beforeLength+goldenByteGrowth {
		t.Fatalf("snake = %#v, want grown by %d onto golden byte", game.Snake, goldenByteGrowth)
	}
	if game.Food != (Point{X: 1, Y: 1}) {
		t.Fatalf("food = %#v, want preserved normal food", game.Food)
	}
	if game.Score != goldenByteBase || quest.BaseScore != goldenByteBase {
		t.Fatalf("score state game=%d base=%d, want golden score %d", game.Score, quest.BaseScore, goldenByteBase)
	}
	if quest.NormalFood != 0 || quest.GoldenCollected != 1 || quest.MissionProgress != 1 || quest.Combo != 1 {
		t.Fatalf("quest counters normal=%d golden=%d mission=%d combo=%d", quest.NormalFood, quest.GoldenCollected, quest.MissionProgress, quest.Combo)
	}
	if quest.DoubleCharges != 3 || !quest.DoubleUntil.Equal(now.Add(doubleScoreDuration)) {
		t.Fatalf("double score changed after golden collection: charges=%d until=%s", quest.DoubleCharges, quest.DoubleUntil)
	}
	if quest.Golden.Active {
		t.Fatal("golden byte remained active after collection")
	}
	if !quest.Pickup.Active || quest.Pickup.Position != (Point{X: 6, Y: 6}) {
		t.Fatalf("pickup changed after golden collection: %#v", quest.Pickup)
	}
}

func TestStepGameCollectsGoldenByteOnNearlyFullBoard(t *testing.T) {
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	game := NewSnakeGame(3, 2, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 0}, {X: 0, Y: 0}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 1}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 3, 2)
	quest.Mission = Mission{ID: MissionGolden2, Label: "Collect 2 Golden Bytes", Target: 2}
	quest.Golden = GoldenByte{Active: true, Position: Point{X: 2, Y: 0}, ExpiresAt: now.Add(goldenByteTTL)}
	quest.Pickup = UpgradePickup{Active: true, Upgrade: UpgradeShield, Position: Point{X: 1, Y: 1}, ExpiresAt: now.Add(pickupTTL)}
	beforeLength := len(game.Snake)

	result := stepGame(game, quest, HeatByLevel(1), now)

	if result != StepAteFood {
		t.Fatalf("stepGame result = %v, want golden-byte growth", result)
	}
	if len(game.Snake) != beforeLength+1 {
		t.Fatalf("snake length = %d, want only safe movement growth to %d", len(game.Snake), beforeLength+1)
	}
	for _, point := range game.Snake {
		if point.X < 0 || point.X >= game.Width || point.Y < 0 || point.Y >= game.Height {
			t.Fatalf("snake segment outside board: %#v in %#v", point, game.Snake)
		}
		if point == quest.Pickup.Position || point == game.Food {
			t.Fatalf("snake segment overlaps occupied quest object or food: point=%#v pickup=%#v food=%#v snake=%#v", point, quest.Pickup.Position, game.Food, game.Snake)
		}
	}
	if game.Food != (Point{X: 0, Y: 1}) {
		t.Fatalf("food = %#v, want preserved valid food on nearly full board", game.Food)
	}
	if game.Food == quest.Pickup.Position || game.Occupies(game.Food) {
		t.Fatalf("food overlaps occupied cell after golden collection: food=%#v pickup=%#v snake=%#v", game.Food, quest.Pickup.Position, game.Snake)
	}
	if quest.Golden.Active || quest.GoldenCollected != 1 || quest.MissionProgress != 1 {
		t.Fatalf("golden state active=%t collected=%d mission=%d", quest.Golden.Active, quest.GoldenCollected, quest.MissionProgress)
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

func assertQuestObjectsValid(t *testing.T, game *SnakeGame, quest *QuestState) {
	t.Helper()
	if !game.FoodValid(quest.ActiveObjectPoints()) {
		t.Fatalf("food invalid: food=%#v snake=%#v golden=%#v pickup=%#v", game.Food, game.Snake, quest.Golden, quest.Pickup)
	}
	if quest.Golden.Active {
		extraOccupied := []Point{game.Food}
		if quest.Pickup.Active {
			extraOccupied = append(extraOccupied, quest.Pickup.Position)
		}
		if !objectPointValid(game, quest.Golden.Position, extraOccupied) {
			t.Fatalf("golden invalid: food=%#v snake=%#v golden=%#v pickup=%#v", game.Food, game.Snake, quest.Golden, quest.Pickup)
		}
	}
	if quest.Pickup.Active {
		extraOccupied := []Point{game.Food}
		if quest.Golden.Active {
			extraOccupied = append(extraOccupied, quest.Golden.Position)
		}
		if !objectPointValid(game, quest.Pickup.Position, extraOccupied) {
			t.Fatalf("pickup invalid: food=%#v snake=%#v golden=%#v pickup=%#v", game.Food, game.Snake, quest.Golden, quest.Pickup)
		}
	}
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
