package game

import (
	"fmt"
	"sort"
	"time"
)

const (
	GameModeClassic = "classic"
	GameModeQuest   = "quest"

	comboWindow        = 8 * time.Second
	goldenByteTTL      = 12 * time.Second
	goldenByteBase     = 100
	missionBonus       = 500
	survivalBonus      = 500
	maxComboBonusUnit  = 100
	maxHeatBonusUnit   = 100
	slowClockDuration  = 20 * time.Second
	normalMinimumSpeed = 180 * time.Millisecond
)

type RandomSource interface {
	Intn(int) int
}

type Upgrade string

const (
	UpgradeShield     Upgrade = "Shield"
	UpgradeSlowClock  Upgrade = "Slow Clock"
	UpgradeDoubleByte Upgrade = "Double Byte"
)

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

type UpgradeChoice struct {
	Upgrade Upgrade
	Label   string
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

	Mission          Mission
	MissionProgress  int
	MissionCompleted bool
	MissionAwarded   bool
	MaxHeat          int

	PendingChoices []UpgradeChoice
	ShieldCharges  int
	SlowUntil      time.Time
	DoubleCharges  int

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
	q.refreshCombo(now)
	q.Combo++
	q.ComboExpiresAt = now.Add(comboWindow)
	if q.Combo > q.MaxCombo {
		q.MaxCombo = q.Combo
	}
	q.NormalFood++
	score := q.scoreFor(baseFoodScore, heat)
	if q.DoubleCharges > 0 {
		score *= 2
		q.DoubleCharges--
	}
	q.BaseScore += score
	game.Score = q.BaseScore
	q.updateMission(heat, now, false)
	if q.NormalFood%5 == 0 && len(q.PendingChoices) == 0 {
		q.PendingChoices = PickUpgradeChoices(q.Rand)
	}
	q.maybeSpawnGolden(game, now)
}

func (q *QuestState) OnGoldenByte(game *SnakeGame, heat HeatLevel, now time.Time) {
	if !q.Enabled() || q.Completed || !q.Golden.Active {
		return
	}
	q.refreshCombo(now)
	q.Combo++
	q.ComboExpiresAt = now.Add(comboWindow)
	if q.Combo > q.MaxCombo {
		q.MaxCombo = q.Combo
	}
	q.GoldenCollected++
	score := q.scoreFor(goldenByteBase, heat)
	if q.DoubleCharges > 0 {
		score *= 2
		q.DoubleCharges--
	}
	q.BaseScore += score
	game.Score = q.BaseScore
	q.Golden.Active = false
	q.updateMission(heat, now, false)
}

func (q *QuestState) Tick(game *SnakeGame, heat HeatLevel, now time.Time) {
	if !q.Enabled() || q.Completed {
		return
	}
	if heat.Level > q.MaxHeat {
		q.MaxHeat = heat.Level
	}
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
	q.PendingChoices = nil
	q.ShieldCharges = 0
	q.SlowUntil = time.Time{}
	q.DoubleCharges = 0
	q.Golden.Active = false
}

func (q *QuestState) TryShieldRecovery(game *SnakeGame) bool {
	if !q.Enabled() || q.ShieldCharges <= 0 {
		return false
	}
	q.ShieldCharges--
	game.Recover()
	return true
}

func (q *QuestState) ApplyUpgrade(index int, now time.Time) bool {
	if !q.Enabled() || index < 0 || index >= len(q.PendingChoices) {
		return false
	}
	switch q.PendingChoices[index].Upgrade {
	case UpgradeShield:
		q.ShieldCharges++
	case UpgradeSlowClock:
		q.SlowUntil = now.Add(slowClockDuration)
	case UpgradeDoubleByte:
		q.DoubleCharges += 3
	}
	q.PendingChoices = nil
	return true
}

func (q *QuestState) EffectiveInterval(base time.Duration, now time.Time) time.Duration {
	if !q.Enabled() || q.SlowUntil.IsZero() || !now.Before(q.SlowUntil) {
		return base
	}
	slowed := base + 60*time.Millisecond
	if slowed < normalMinimumSpeed {
		return normalMinimumSpeed
	}
	return slowed
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
	if q.ShieldCharges > 0 {
		parts = append(parts, fmt.Sprintf("SHIELD %d", q.ShieldCharges))
	}
	if q.DoubleCharges > 0 {
		parts = append(parts, fmt.Sprintf("DOUBLE BYTE %d", q.DoubleCharges))
	}
	return joinParts(parts)
}

func (q *QuestState) scoreFor(base int, heat HeatLevel) int {
	score := heat.ScoreAward(base)
	comboMultiplier := 1000 + (q.Combo-1)*250
	return score * comboMultiplier / 1000
}

func (q *QuestState) refreshCombo(now time.Time) {
	if q.Combo > 0 && !now.Before(q.ComboExpiresAt) {
		q.Combo = 0
		q.ComboExpiresAt = time.Time{}
	}
}

func (q *QuestState) maybeSpawnGolden(game *SnakeGame, now time.Time) {
	if q.Golden.Active || q.NormalFood == 0 || q.NormalFood%7 != 0 {
		return
	}
	point, ok := freePoint(game, q.Rand, []Point{game.Food})
	if !ok {
		return
	}
	q.Golden = GoldenByte{Position: point, ExpiresAt: now.Add(goldenByteTTL), Active: true}
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

func PickUpgradeChoices(random RandomSource) []UpgradeChoice {
	upgrades := []Upgrade{UpgradeShield, UpgradeSlowClock, UpgradeDoubleByte}
	for index := range upgrades {
		swap := index + randomIndex(random, len(upgrades)-index)
		upgrades[index], upgrades[swap] = upgrades[swap], upgrades[index]
	}
	choices := make([]UpgradeChoice, 0, 3)
	for _, upgrade := range upgrades[:3] {
		choices = append(choices, UpgradeChoice{Upgrade: upgrade, Label: string(upgrade)})
	}
	return choices
}

func freePoint(game *SnakeGame, random RandomSource, extraOccupied []Point) (Point, bool) {
	occupied := make(map[Point]bool, len(game.Snake)+len(extraOccupied))
	for _, point := range game.Snake {
		occupied[point] = true
	}
	for _, point := range extraOccupied {
		if point.X >= 0 && point.Y >= 0 {
			occupied[point] = true
		}
	}
	available := make([]Point, 0, game.Width*game.Height-len(occupied))
	for y := 0; y < game.Height; y++ {
		for x := 0; x < game.Width; x++ {
			point := Point{X: x, Y: y}
			if !occupied[point] {
				available = append(available, point)
			}
		}
	}
	if len(available) == 0 {
		return Point{}, false
	}
	sort.Slice(available, func(i, j int) bool {
		if available[i].Y == available[j].Y {
			return available[i].X < available[j].X
		}
		return available[i].Y < available[j].Y
	})
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
