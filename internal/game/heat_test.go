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

func TestRestartHeatRampCatchesUpWithoutExceedingCommandHeat(t *testing.T) {
	if got := RestartStartHeat(1); got != 1 {
		t.Fatalf("RestartStartHeat(1) = %d, want 1", got)
	}
	if got := RestartStartHeat(2); got != 2 {
		t.Fatalf("RestartStartHeat(2) = %d, want 2", got)
	}
	if got := RestartStartHeat(6); got != 4 {
		t.Fatalf("RestartStartHeat(6) = %d, want 4", got)
	}

	start := RestartStartHeat(6)
	tests := []struct {
		elapsed time.Duration
		want    int
	}{
		{0, 4},
		{19*time.Second + 999*time.Millisecond, 4},
		{20 * time.Second, 5},
		{39*time.Second + 999*time.Millisecond, 5},
		{40 * time.Second, 6},
		{10 * time.Minute, 6},
	}
	for _, test := range tests {
		if got := RestartRampHeat(6, start, test.elapsed); got != test.want {
			t.Fatalf("RestartRampHeat elapsed %s = %d, want %d", test.elapsed, got, test.want)
		}
	}
}

func TestUpcomingHeatWarnsBeforeNextThreshold(t *testing.T) {
	next, remaining, ok := UpcomingHeat(25 * time.Second)
	if !ok || next.Level != 2 || remaining != 5*time.Second {
		t.Fatalf("UpcomingHeat = level %d remaining %s ok %t", next.Level, remaining, ok)
	}
}
