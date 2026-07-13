package game

import (
	"fmt"
	"time"
)

const (
	GameModeClassic = "classic"
	GameModeQuest   = "quest"

	comboWindow         = 8 * time.Second
	minComboWindow      = 5 * time.Second
	maxComboWindow      = 12 * time.Second
	goldenByteTTL       = 20 * time.Second
	goldenByteBase      = 100
	pickupTTL           = 20 * time.Second
	pickupSpawnMin      = 4
	pickupSpawnMax      = 8
	goldenSpawnMin      = 6
	goldenSpawnMax      = 9
	shieldDuration      = 30 * time.Second
	phaseDuration       = 20 * time.Second
	slowClockDuration   = 15 * time.Second
	doubleScoreDuration = 30 * time.Second
	comboKeeperDuration = 20 * time.Second
	turboDuration       = 10 * time.Second
	warpDuration        = 25 * time.Second
	missionBonus        = 500
	survivalBonus       = 500
	maxComboBonusUnit   = 100
	maxHeatBonusUnit    = 100
	normalMinimumSpeed  = 180 * time.Millisecond
	turboMinimumSpeed   = 55 * time.Millisecond
	turboSpeedPermille  = 650
	turboScorePermille  = 1250
	doubleScoreCharges  = 3
	doubleScoreCap      = 6
	patchRemoveLimit    = 3
	minimumSnakeLength  = 1
)

type RandomSource interface {
	Intn(int) int
}

type Upgrade string

const (
	UpgradeShield      Upgrade = "Shield"
	UpgradePhase       Upgrade = "Phase"
	UpgradeSlowClock   Upgrade = "Slow Clock"
	UpgradeDoubleScore Upgrade = "Double Score"
	UpgradePatch       Upgrade = "Patch"
	UpgradeComboKeeper Upgrade = "Combo Keeper"
	UpgradeTurbo       Upgrade = "Turbo"
	UpgradeWarp        Upgrade = "Warp"
)

var pickupPool = []Upgrade{
	UpgradeShield,
	UpgradePhase,
	UpgradeSlowClock,
	UpgradeDoubleScore,
	UpgradePatch,
	UpgradeComboKeeper,
	UpgradeTurbo,
	UpgradeWarp,
}

type pickupDefinition struct {
	Name   string
	Symbol string
}

var pickupDefinitions = map[Upgrade]pickupDefinition{
	UpgradeShield:      {Name: "Shield", Symbol: "[]"},
	UpgradePhase:       {Name: "Phase", Symbol: "##"},
	UpgradeSlowClock:   {Name: "Slow Clock", Symbol: "~~"},
	UpgradeDoubleScore: {Name: "Double Score", Symbol: "x2"},
	UpgradePatch:       {Name: "Patch", Symbol: "+-"},
	UpgradeComboKeeper: {Name: "Combo Keeper", Symbol: "CK"},
	UpgradeTurbo:       {Name: "Turbo", Symbol: ">>"},
	UpgradeWarp:        {Name: "Warp", Symbol: "@@"},
}

type MissionID string

const (
	MissionCombo5     MissionID = "combo_5"
	MissionFood15     MissionID = "food_15"
	MissionSurvive60  MissionID = "survive_60"
	MissionGolden2    MissionID = "golden_2"
	MissionReachHeat4 MissionID = "heat_4"
)

type Mission struct {
	ID     MissionID
	Label  string
	Target int
}

var missionPool = []Mission{
	{ID: MissionCombo5, Label: "Reach combo x5", Target: 5},
	{ID: MissionFood15, Label: "Collect 15 food", Target: 15},
	{ID: MissionSurvive60, Label: "Survive 60s", Target: 60},
	{ID: MissionGolden2, Label: "Collect 2 Golden Bytes", Target: 2},
	{ID: MissionReachHeat4, Label: "Reach Heat 4 alive", Target: 4},
}

type GoldenByte struct {
	Position  Point
	ExpiresAt time.Time
	Active    bool
}

type UpgradePickup struct {
	Upgrade   Upgrade
	Position  Point
	ExpiresAt time.Time
	Active    bool
}

type TimedCharge struct {
	Charges   int
	ExpiresAt time.Time
}

type ScoreBreakdown struct {
	BaseScore       int
	MissionBonus    int
	SurvivalBonus   int
	MaxComboBonus   int
	MaxHeatBonus    int
	FinalScore      int
	SurvivalAwarded bool
	MissionAwarded  bool
}

type QuestState struct {
	Mode string

	StartedAt time.Time
	Rand      RandomSource

	BaseScore       int
	Combo           int
	MaxCombo        int
	ComboExpiresAt  time.Time
	NormalFood      int
	GoldenCollected int
	Golden          GoldenByte
	NextGoldenFood  int

	Mission          Mission
	MissionProgress  int
	MissionCompleted bool
	MissionAwarded   bool
	MaxHeat          int

	Pickup           UpgradePickup
	NextPickupFood   int
	Shield           TimedCharge
	Phase            TimedCharge
	Warp             TimedCharge
	SlowUntil        time.Time
	DoubleCharges    int
	DoubleUntil      time.Time
	ComboKeeperUntil time.Time
	TurboUntil       time.Time
	Message          string
	MessageUntil     time.Time

	Completed       bool
	SurvivalAwarded bool
	Final           ScoreBreakdown
}

func NewQuestState(mode string, now time.Time, random RandomSource, boardWidth int, boardHeight int) *QuestState {
	if mode == "" {
		mode = GameModeClassic
	}
	state := &QuestState{Mode: mode, StartedAt: now, Rand: random}
	if mode != GameModeQuest {
		return state
	}
	state.Mission = selectMission(random, boardWidth, boardHeight)
	return state
}

func (q *QuestState) Enabled() bool {
	return q != nil && q.Mode == GameModeQuest
}

func (q *QuestState) OnNormalFood(game *SnakeGame, heat HeatLevel, now time.Time) {
	if !q.Enabled() || q.Completed {
		return
	}
	if q.NextPickupFood == 0 {
		q.scheduleNextPickup(q.NormalFood)
	}
	if q.NextGoldenFood == 0 {
		q.scheduleNextGolden(q.NormalFood)
	}
	q.expireEffects(now)
	q.refreshCombo(now)
	q.Combo++
	q.ComboExpiresAt = now.Add(comboDuration(q.Combo))
	if q.Combo > q.MaxCombo {
		q.MaxCombo = q.Combo
	}
	q.NormalFood++
	score := q.scoreFor(baseFoodScore, heat)
	if q.DoubleCharges > 0 && now.Before(q.DoubleUntil) {
		score *= 2
		q.DoubleCharges--
		if q.DoubleCharges == 0 {
			q.DoubleUntil = time.Time{}
		}
	}
	q.BaseScore += score
	game.Score = q.BaseScore
	q.updateMission(heat, now, false)
	if q.NormalFood >= q.NextPickupFood && !q.Pickup.Active {
		if q.spawnPickup(game, now) {
			q.scheduleNextPickup(q.NormalFood)
		}
	}
	q.maybeSpawnGolden(game, now)
}

func (q *QuestState) OnGoldenByte(game *SnakeGame, heat HeatLevel, now time.Time) {
	if !q.Enabled() || q.Completed || !q.Golden.Active {
		return
	}
	q.expireEffects(now)
	q.refreshCombo(now)
	q.Combo++
	q.ComboExpiresAt = now.Add(comboDuration(q.Combo))
	if q.Combo > q.MaxCombo {
		q.MaxCombo = q.Combo
	}
	q.GoldenCollected++
	score := q.scoreFor(goldenByteBase, heat)
	q.BaseScore += score
	game.Score = q.BaseScore
	q.Golden.Active = false
	q.updateMission(heat, now, false)
}

func (q *QuestState) OnPickupCollected(game *SnakeGame, heat HeatLevel, now time.Time) bool {
	if !q.Enabled() || q.Completed || !q.Pickup.Active {
		return false
	}
	pickup := q.Pickup
	q.Pickup = UpgradePickup{}
	switch pickup.Upgrade {
	case UpgradeShield:
		q.Shield = TimedCharge{Charges: 1, ExpiresAt: now.Add(shieldDuration)}
		q.notice("PICKUP: SHIELD - NEXT COLLISION BLOCKED", now)
	case UpgradePhase:
		q.Phase = TimedCharge{Charges: 1, ExpiresAt: now.Add(phaseDuration)}
		q.notice("PICKUP: PHASE - NEXT COLLISION PASSES THROUGH", now)
	case UpgradeSlowClock:
		q.SlowUntil = now.Add(slowClockDuration)
		q.notice("PICKUP: SLOW CLOCK", now)
	case UpgradeDoubleScore:
		q.DoubleCharges = doubleScoreCharges
		q.DoubleUntil = now.Add(doubleScoreDuration)
		q.notice("PICKUP: DOUBLE SCORE", now)
	case UpgradePatch:
		removed := game.TrimTail(patchRemoveLimit, minimumSnakeLength)
		q.notice(fmt.Sprintf("PICKUP: PATCH - LENGTH -%d", removed), now)
	case UpgradeComboKeeper:
		q.ComboKeeperUntil = now.Add(comboKeeperDuration)
		q.notice("PICKUP: COMBO KEEPER", now)
	case UpgradeTurbo:
		q.TurboUntil = now.Add(turboDuration)
		q.notice("PICKUP: TURBO", now)
	case UpgradeWarp:
		q.Warp = TimedCharge{Charges: 1, ExpiresAt: now.Add(warpDuration)}
		q.notice("PICKUP: WARP - EMERGENCY TELEPORT READY", now)
	default:
		return false
	}
	q.updateMission(heat, now, false)
	return true
}

func (q *QuestState) Tick(game *SnakeGame, heat HeatLevel, now time.Time) {
	if !q.Enabled() || q.Completed {
		return
	}
	if heat.Level > q.MaxHeat {
		q.MaxHeat = heat.Level
	}
	q.expireEffects(now)
	q.refreshCombo(now)
	if q.Golden.Active && !now.Before(q.Golden.ExpiresAt) {
		q.Golden.Active = false
	}
	q.updateMission(heat, now, false)
}

func (q *QuestState) OnCrash() {
	if !q.Enabled() {
		return
	}
	q.Combo = 0
	q.ComboExpiresAt = time.Time{}
	q.clearRoundEffects()
	q.Golden.Active = false
}

func (q *QuestState) TryCollisionEffects(game *SnakeGame, result StepResult, now time.Time) StepResult {
	if !q.Enabled() || (result != StepHitWall && result != StepHitSelf) {
		return result
	}
	q.expireEffects(now)
	if q.Phase.Charges > 0 && now.Before(q.Phase.ExpiresAt) && game.PhaseForward() {
		q.Phase = TimedCharge{}
		q.ResizeObjects(game)
		q.notice("PHASE USED", now)
		return StepMoved
	}
	if q.Shield.Charges > 0 && now.Before(q.Shield.ExpiresAt) {
		q.Shield = TimedCharge{}
		game.Recover()
		q.ResizeObjects(game)
		q.notice("SHIELD USED", now)
		return StepMoved
	}
	if q.Warp.Charges > 0 && now.Before(q.Warp.ExpiresAt) && game.WarpToFreePoint(q.Rand, append([]Point{game.Food}, q.ActiveObjectPoints()...)) {
		q.Warp = TimedCharge{}
		q.ResizeObjects(game)
		q.notice("WARP USED", now)
		return StepMoved
	}
	return result
}

func (q *QuestState) TryShieldRecovery(game *SnakeGame) bool {
	if !q.Enabled() || q.Shield.Charges <= 0 {
		return false
	}
	q.Shield = TimedCharge{}
	game.Recover()
	q.ResizeObjects(game)
	return true
}

func (q *QuestState) EffectiveInterval(base time.Duration, now time.Time) time.Duration {
	if !q.Enabled() {
		return base
	}
	interval := base
	if !q.SlowUntil.IsZero() && now.Before(q.SlowUntil) {
		interval += 60 * time.Millisecond
		if interval < normalMinimumSpeed {
			interval = normalMinimumSpeed
		}
	}
	if !q.TurboUntil.IsZero() && now.Before(q.TurboUntil) {
		interval = interval * turboSpeedPermille / 1000
		if interval < turboMinimumSpeed {
			interval = turboMinimumSpeed
		}
	}
	return interval
}

func (q *QuestState) Complete(game *SnakeGame, heat HeatLevel, now time.Time) ScoreBreakdown {
	if q == nil {
		return ScoreBreakdown{BaseScore: game.Score, FinalScore: game.Score}
	}
	if q.Completed {
		return q.Final
	}
	q.Completed = true
	if q.Enabled() {
		q.updateMission(heat, now, !game.Over)
		q.clearRoundEffects()
	}
	final := ScoreBreakdown{BaseScore: game.Score}
	if q.Enabled() {
		final.BaseScore = q.BaseScore
		if q.MissionCompleted && !q.MissionAwarded {
			final.MissionBonus = missionBonus
			final.MissionAwarded = true
			q.MissionAwarded = true
		}
		if !game.Over && !q.SurvivalAwarded {
			final.SurvivalBonus = survivalBonus
			final.SurvivalAwarded = true
			q.SurvivalAwarded = true
		}
		final.MaxComboBonus = q.MaxCombo * maxComboBonusUnit
		final.MaxHeatBonus = heat.Level * maxHeatBonusUnit
	}
	final.FinalScore = final.BaseScore + final.MissionBonus + final.SurvivalBonus + final.MaxComboBonus + final.MaxHeatBonus
	q.Final = final
	game.Score = final.FinalScore
	return final
}

func (q *QuestState) HUD() string {
	if !q.Enabled() {
		return "MODE classic"
	}
	parts := []string{"MODE quest"}
	parts = append(parts, fmt.Sprintf("COMBO x%d", q.Combo))
	if q.Mission.ID != "" {
		parts = append(parts, fmt.Sprintf("QUEST: %s %d/%d", q.Mission.Label, q.MissionProgress, q.Mission.Target))
	}
	parts = append(parts, q.effectHUDParts(time.Now())...)
	return joinParts(parts)
}

func (q *QuestState) scoreFor(base int, heat HeatLevel) int {
	score := heat.ScoreAward(base)
	comboMultiplier := 1000 + (q.Combo-1)*250
	if !q.TurboUntil.IsZero() {
		score = score * turboScorePermille / 1000
	}
	return score * comboMultiplier / 1000
}

func (q *QuestState) refreshCombo(now time.Time) {
	if q.Combo > 0 && !q.ComboKeeperUntil.IsZero() {
		if now.Before(q.ComboKeeperUntil) {
			return
		}
		q.ComboKeeperUntil = time.Time{}
		if !now.Before(q.ComboExpiresAt) {
			q.ComboExpiresAt = now.Add(comboDuration(q.Combo))
			return
		}
	}
	if q.Combo > 0 && !now.Before(q.ComboExpiresAt) {
		q.Combo = 0
		q.ComboExpiresAt = time.Time{}
	}
}

func comboDuration(combo int) time.Duration {
	if combo <= 0 {
		return minComboWindow
	}
	duration := time.Duration(4+combo) * time.Second
	if duration < minComboWindow {
		return minComboWindow
	}
	if duration > maxComboWindow {
		return maxComboWindow
	}
	return duration
}

func (q *QuestState) maybeSpawnGolden(game *SnakeGame, now time.Time) {
	if q.Golden.Active || q.NormalFood < q.NextGoldenFood {
		return
	}
	extraOccupied := []Point{game.Food}
	if q.Pickup.Active {
		extraOccupied = append(extraOccupied, q.Pickup.Position)
	}
	point, ok := freePoint(game, q.Rand, extraOccupied)
	if !ok {
		return
	}
	q.Golden = GoldenByte{Position: point, ExpiresAt: now.Add(goldenByteTTL), Active: true}
	q.scheduleNextGolden(q.NormalFood)
}

func (q *QuestState) scheduleNextPickup(normalFood int) {
	q.NextPickupFood = normalFood + randomRange(q.Rand, pickupSpawnMin, pickupSpawnMax)
}

func (q *QuestState) scheduleNextGolden(normalFood int) {
	q.NextGoldenFood = normalFood + randomRange(q.Rand, goldenSpawnMin, goldenSpawnMax)
}

func (q *QuestState) spawnPickup(game *SnakeGame, now time.Time) bool {
	upgrade, ok := selectPickup(q.Rand, availablePickups(game))
	if !ok {
		return false
	}
	extraOccupied := []Point{game.Food}
	if q.Golden.Active {
		extraOccupied = append(extraOccupied, q.Golden.Position)
	}
	point, ok := freePoint(game, q.Rand, extraOccupied)
	if !ok {
		return false
	}
	q.Pickup = UpgradePickup{Upgrade: upgrade, Position: point, ExpiresAt: now.Add(pickupTTL), Active: true}
	return true
}

func (q *QuestState) ResizePickup(game *SnakeGame) {
	q.ResizeObjects(game)
}

func (q *QuestState) ResizeObjects(game *SnakeGame) {
	if !q.Enabled() {
		return
	}
	q.resizeGolden(game)
	q.resizePickup(game)
	q.EnsureFood(game)
}

func (q *QuestState) EnsureFood(game *SnakeGame) bool {
	if !q.Enabled() {
		return game.FoodValid(nil)
	}
	extraOccupied := q.ActiveObjectPoints()
	if game.FoodValid(extraOccupied) {
		return true
	}
	return game.PlaceFoodExcluding(extraOccupied)
}

func (q *QuestState) ActiveObjectPoints() []Point {
	if !q.Enabled() {
		return nil
	}
	points := make([]Point, 0, 2)
	if q.Golden.Active {
		points = append(points, q.Golden.Position)
	}
	if q.Pickup.Active {
		points = append(points, q.Pickup.Position)
	}
	return points
}

func (q *QuestState) resizeGolden(game *SnakeGame) {
	if !q.Enabled() || !q.Golden.Active {
		return
	}
	point := q.Golden.Position
	extraOccupied := []Point{game.Food}
	if q.Pickup.Active {
		extraOccupied = append(extraOccupied, q.Pickup.Position)
	}
	if objectPointValid(game, point, extraOccupied) {
		return
	}
	next, ok := freePoint(game, q.Rand, extraOccupied)
	if !ok {
		q.Golden.Active = false
		return
	}
	q.Golden.Position = next
}

func (q *QuestState) resizePickup(game *SnakeGame) {
	if !q.Enabled() || !q.Pickup.Active {
		return
	}
	point := q.Pickup.Position
	extraOccupied := []Point{game.Food}
	if q.Golden.Active {
		extraOccupied = append(extraOccupied, q.Golden.Position)
	}
	if objectPointValid(game, point, extraOccupied) {
		return
	}
	next, ok := freePoint(game, q.Rand, extraOccupied)
	if !ok {
		q.Pickup = UpgradePickup{}
		return
	}
	q.Pickup.Position = next
}

func objectPointValid(game *SnakeGame, point Point, extraOccupied []Point) bool {
	if point.X < 0 || point.X >= game.Width || point.Y < 0 || point.Y >= game.Height || game.Occupies(point) {
		return false
	}
	for _, occupied := range extraOccupied {
		if point == occupied {
			return false
		}
	}
	return true
}

func (q *QuestState) expireEffects(now time.Time) {
	if q.Pickup.Active && !now.Before(q.Pickup.ExpiresAt) {
		q.Pickup = UpgradePickup{}
		q.notice("PICKUP EXPIRED", now)
	}
	if q.Golden.Active && !now.Before(q.Golden.ExpiresAt) {
		q.Golden.Active = false
	}
	if q.Shield.Charges > 0 && !now.Before(q.Shield.ExpiresAt) {
		q.Shield = TimedCharge{}
		q.notice("SHIELD EXPIRED", now)
	}
	if q.Phase.Charges > 0 && !now.Before(q.Phase.ExpiresAt) {
		q.Phase = TimedCharge{}
		q.notice("PHASE EXPIRED", now)
	}
	if q.Warp.Charges > 0 && !now.Before(q.Warp.ExpiresAt) {
		q.Warp = TimedCharge{}
		q.notice("WARP EXPIRED", now)
	}
	if q.DoubleCharges > 0 && !now.Before(q.DoubleUntil) {
		q.DoubleCharges = 0
		q.DoubleUntil = time.Time{}
		q.notice("DOUBLE EXPIRED", now)
	}
	if !q.SlowUntil.IsZero() && !now.Before(q.SlowUntil) {
		q.SlowUntil = time.Time{}
		q.notice("SLOW EXPIRED", now)
	}
	if !q.TurboUntil.IsZero() && !now.Before(q.TurboUntil) {
		q.TurboUntil = time.Time{}
		q.notice("TURBO EXPIRED", now)
	}
	if q.Message != "" && !now.Before(q.MessageUntil) {
		q.Message = ""
		q.MessageUntil = time.Time{}
	}
}

func (q *QuestState) clearRoundEffects() {
	q.Pickup = UpgradePickup{}
	q.Shield = TimedCharge{}
	q.Phase = TimedCharge{}
	q.Warp = TimedCharge{}
	q.SlowUntil = time.Time{}
	q.DoubleCharges = 0
	q.DoubleUntil = time.Time{}
	q.ComboKeeperUntil = time.Time{}
	q.TurboUntil = time.Time{}
	q.Message = ""
	q.MessageUntil = time.Time{}
}

func (q *QuestState) notice(message string, now time.Time) {
	q.Message = message
	q.MessageUntil = now.Add(2 * time.Second)
}

func (q *QuestState) effectHUDParts(now time.Time) []string {
	parts := []string{}
	if q.Shield.Charges > 0 && now.Before(q.Shield.ExpiresAt) {
		parts = append(parts, chargedEffectHUDPart("SHIELD", q.Shield.Charges, q.Shield.ExpiresAt, now))
	}
	if q.Phase.Charges > 0 && now.Before(q.Phase.ExpiresAt) {
		parts = append(parts, chargedEffectHUDPart("PHASE", q.Phase.Charges, q.Phase.ExpiresAt, now))
	}
	if q.Warp.Charges > 0 && now.Before(q.Warp.ExpiresAt) {
		parts = append(parts, chargedEffectHUDPart("WARP", q.Warp.Charges, q.Warp.ExpiresAt, now))
	}
	if q.DoubleCharges > 0 && now.Before(q.DoubleUntil) {
		parts = append(parts, chargedEffectHUDPart("DOUBLE", q.DoubleCharges, q.DoubleUntil, now))
	}
	if !q.SlowUntil.IsZero() && now.Before(q.SlowUntil) {
		parts = append(parts, fmt.Sprintf("SLOW %ds", secondsLeft(q.SlowUntil, now)))
	}
	if !q.ComboKeeperUntil.IsZero() && now.Before(q.ComboKeeperUntil) {
		parts = append(parts, fmt.Sprintf("COMBO LOCK %ds", secondsLeft(q.ComboKeeperUntil, now)))
	}
	if !q.TurboUntil.IsZero() && now.Before(q.TurboUntil) {
		parts = append(parts, fmt.Sprintf("TURBO %ds", secondsLeft(q.TurboUntil, now)))
	}
	return parts
}

func chargedEffectHUDPart(label string, charges int, deadline time.Time, now time.Time) string {
	return fmt.Sprintf("%s x%d %ds", label, charges, secondsLeft(deadline, now))
}

func (q *QuestState) updateMission(heat HeatLevel, now time.Time, alive bool) {
	if heat.Level > q.MaxHeat {
		q.MaxHeat = heat.Level
	}
	progress := 0
	switch q.Mission.ID {
	case MissionCombo5:
		progress = q.Combo
	case MissionFood15:
		progress = q.NormalFood
	case MissionSurvive60:
		progress = int(now.Sub(q.StartedAt).Seconds())
	case MissionGolden2:
		progress = q.GoldenCollected
	case MissionReachHeat4:
		if alive || heat.Level >= q.Mission.Target {
			progress = heat.Level
		}
	}
	if progress > q.Mission.Target {
		progress = q.Mission.Target
	}
	q.MissionProgress = progress
	if q.Mission.ID != "" && q.MissionProgress >= q.Mission.Target {
		q.MissionCompleted = true
	}
}

func selectMission(random RandomSource, boardWidth int, boardHeight int) Mission {
	pool := append([]Mission(nil), missionPool...)
	if boardWidth*boardHeight < 8 {
		filtered := pool[:0]
		for _, mission := range pool {
			if mission.ID != MissionGolden2 {
				filtered = append(filtered, mission)
			}
		}
		pool = filtered
	}
	if len(pool) == 0 {
		return Mission{}
	}
	return pool[randomIndex(random, len(pool))]
}

func availablePickups(game *SnakeGame) []Upgrade {
	upgrades := make([]Upgrade, 0, len(pickupPool))
	for _, upgrade := range pickupPool {
		if upgrade == UpgradePatch && len(game.Snake) <= minimumSnakeLength {
			continue
		}
		upgrades = append(upgrades, upgrade)
	}
	return upgrades
}

func selectPickup(random RandomSource, pool []Upgrade) (Upgrade, bool) {
	if len(pool) == 0 {
		return "", false
	}
	return pool[randomIndex(random, len(pool))], true
}

func PickupName(upgrade Upgrade) string {
	if definition, ok := pickupDefinitions[upgrade]; ok {
		return definition.Name
	}
	return string(upgrade)
}

func PickupSymbol(upgrade Upgrade) string {
	if definition, ok := pickupDefinitions[upgrade]; ok {
		return definition.Symbol
	}
	return "??"
}

func secondsLeft(deadline time.Time, now time.Time) int {
	remaining := deadline.Sub(now)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Round(time.Second).Seconds())
}

func freePoint(game *SnakeGame, random RandomSource, extraOccupied []Point) (Point, bool) {
	occupied := occupiedCells(game.Width, game.Height, game.Snake, extraOccupied)
	distances := reachableDistances(game.Width, game.Height, game.Snake[0], occupied)
	preferred := FoodRangeForHeat(game.FoodHeat)
	available := make([]Point, 0, len(distances))
	fallback := make([]Point, 0, len(distances))
	for point, distance := range distances {
		if point == game.Snake[0] || occupied[point] {
			continue
		}
		fallback = append(fallback, point)
		if distance >= preferred.Min && distance <= preferred.Max {
			available = append(available, point)
		}
	}
	if len(available) == 0 {
		available = fallback
	}
	if len(available) == 0 {
		return Point{}, false
	}
	sortPoints(available)
	return available[randomIndex(random, len(available))], true
}

func randomIndex(random RandomSource, max int) int {
	if max <= 0 {
		return 0
	}
	if random == nil {
		return 0
	}
	value := random.Intn(max)
	if value < 0 {
		return 0
	}
	if value >= max {
		return max - 1
	}
	return value
}

func randomRange(random RandomSource, min int, max int) int {
	if max < min {
		return min
	}
	return min + randomIndex(random, max-min+1)
}
