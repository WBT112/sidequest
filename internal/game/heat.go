package game

import (
	"fmt"
	"time"
)

const (
	baseFoodScore       = 10
	heatWarningWindow   = 5 * time.Second
	restartCatchUpEvery = 20 * time.Second
)

type HeatLevel struct {
	Level              int
	RuntimeThreshold   time.Duration
	MovementInterval   time.Duration
	MultiplierPermille int
}

var defaultHeatCurve = []HeatLevel{
	{Level: 1, RuntimeThreshold: 0, MovementInterval: 180 * time.Millisecond, MultiplierPermille: 1000},
	{Level: 2, RuntimeThreshold: 30 * time.Second, MovementInterval: 160 * time.Millisecond, MultiplierPermille: 1200},
	{Level: 3, RuntimeThreshold: 60 * time.Second, MovementInterval: 140 * time.Millisecond, MultiplierPermille: 1400},
	{Level: 4, RuntimeThreshold: 90 * time.Second, MovementInterval: 120 * time.Millisecond, MultiplierPermille: 1700},
	{Level: 5, RuntimeThreshold: 135 * time.Second, MovementInterval: 100 * time.Millisecond, MultiplierPermille: 2000},
	{Level: 6, RuntimeThreshold: 180 * time.Second, MovementInterval: 85 * time.Millisecond, MultiplierPermille: 2500},
}

func HeatForElapsed(elapsed time.Duration) HeatLevel {
	if elapsed < 0 {
		elapsed = 0
	}
	current := defaultHeatCurve[0]
	for _, level := range defaultHeatCurve {
		if elapsed < level.RuntimeThreshold {
			break
		}
		current = level
	}
	return current
}

func HeatByLevel(level int) HeatLevel {
	if level <= defaultHeatCurve[0].Level {
		return defaultHeatCurve[0]
	}
	for _, heat := range defaultHeatCurve {
		if heat.Level == level {
			return heat
		}
	}
	return defaultHeatCurve[len(defaultHeatCurve)-1]
}

func UpcomingHeat(elapsed time.Duration) (HeatLevel, time.Duration, bool) {
	if elapsed < 0 {
		elapsed = 0
	}
	for _, level := range defaultHeatCurve {
		if elapsed < level.RuntimeThreshold {
			return level, level.RuntimeThreshold - elapsed, true
		}
	}
	return defaultHeatCurve[len(defaultHeatCurve)-1], 0, false
}

func RestartStartHeat(commandHeatLevel int) int {
	if commandHeatLevel <= 2 {
		return commandHeatLevel
	}
	start := commandHeatLevel - 2
	if start < 1 {
		return 1
	}
	return start
}

func RestartRampHeat(commandHeatLevel int, restartStartLevel int, roundElapsed time.Duration) int {
	if commandHeatLevel < 1 {
		commandHeatLevel = 1
	}
	if restartStartLevel < 1 {
		restartStartLevel = 1
	}
	if roundElapsed < 0 {
		roundElapsed = 0
	}
	level := restartStartLevel + int(roundElapsed/restartCatchUpEvery)
	if level > commandHeatLevel {
		return commandHeatLevel
	}
	return level
}

func (h HeatLevel) ScoreAward(base int) int {
	if base < 0 {
		base = 0
	}
	return base * h.MultiplierPermille / 1000
}

func (h HeatLevel) MultiplierText() string {
	whole := h.MultiplierPermille / 1000
	fraction := h.MultiplierPermille % 1000
	if fraction == 0 {
		return fmt.Sprintf("x%d.0", whole)
	}
	if fraction%100 == 0 {
		return fmt.Sprintf("x%d.%d", whole, fraction/100)
	}
	return fmt.Sprintf("x%d.%03d", whole, fraction)
}

func MaxHeatLevel() int {
	return defaultHeatCurve[len(defaultHeatCurve)-1].Level
}
