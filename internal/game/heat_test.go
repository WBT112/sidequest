package game

import (
	"testing"
	"time"
)

func TestHeatForElapsedUsesDeterministicThresholds(t *testing.T) {
	tests := []struct {
		elapsed time.Duration
		level   int
	}{
		{29*time.Second + 999*time.Millisecond, 1},
		{30 * time.Second, 2},
		{59*time.Second + 999*time.Millisecond, 2},
		{60 * time.Second, 3},
		{90 * time.Second, 4},
		{135 * time.Second, 5},
		{180 * time.Second, 6},
		{24 * time.Hour, 6},
	}

	for _, test := range tests {
		if got := HeatForElapsed(test.elapsed).Level; got != test.level {
			t.Fatalf("HeatForElapsed(%s) = %d, want %d", test.elapsed, got, test.level)
		}
	}
}

func TestHeatCurveCapsDifficultyAndSpeedsUp(t *testing.T) {
	lastInterval := time.Hour
	for level := 1; level <= MaxHeatLevel(); level++ {
		heat := HeatByLevel(level)
		if heat.Level != level {
			t.Fatalf("HeatByLevel(%d) = %d", level, heat.Level)
		}
		if heat.MovementInterval >= lastInterval {
			t.Fatalf("level %d interval = %s, previous %s", level, heat.MovementInterval, lastInterval)
		}
		lastInterval = heat.MovementInterval
	}

	if got := HeatForElapsed(48 * time.Hour).Level; got != MaxHeatLevel() {
		t.Fatalf("far future heat = %d, want max", got)
	}
}

func TestHeatScoreAwardUsesFixedPointMultiplier(t *testing.T) {
	tests := []struct {
		level int
		want  int
	}{
		{1, 10},
		{2, 12},
		{3, 14},
		{4, 17},
		{5, 20},
		{6, 25},
	}

	for _, test := range tests {
		if got := HeatByLevel(test.level).ScoreAward(baseFoodScore); got != test.want {
			t.Fatalf("level %d score = %d, want %d", test.level, got, test.want)
		}
	}
}

func TestUpcomingHeatWarnsBeforeNextThreshold(t *testing.T) {
	next, remaining, ok := UpcomingHeat(25 * time.Second)
	if !ok || next.Level != 2 || remaining != 5*time.Second {
		t.Fatalf("UpcomingHeat = level %d remaining %s ok %t", next.Level, remaining, ok)
	}
}
