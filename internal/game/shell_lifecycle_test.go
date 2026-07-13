package game

import (
	"testing"
	"time"

	"github.com/WBT112/sidequest/internal/session"
)

func TestTerminalState(t *testing.T) {
	for _, state := range []string{session.StatusCompleted, session.StatusFailed, session.StatusInterrupted, session.StatusStartFailed} {
		if !terminalState(state) {
			t.Fatalf("terminalState(%q) = false, want true", state)
		}
	}
	if terminalState(session.StatusRunning) {
		t.Fatal("terminalState(running) = true, want false")
	}
}

func TestPauseStateActive(t *testing.T) {
	tests := []struct {
		pause PauseState
		want  bool
	}{
		{pause: PauseState{}, want: false},
		{pause: PauseState{Manual: true}, want: true},
		{pause: PauseState{Focus: true}, want: true},
		{pause: PauseState{Resize: true}, want: true},
		{pause: PauseState{Manual: true, Focus: true}, want: true},
	}
	for _, test := range tests {
		if got := test.pause.Active(); got != test.want {
			t.Fatalf("PauseState(%#v).Active() = %t, want %t", test.pause, got, test.want)
		}
	}
}

func TestPlayClockAccumulatesOnlyWhileRunning(t *testing.T) {
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	var clock PlayClock

	clock.Start(base)
	if got := clock.Elapsed(base.Add(10 * time.Second)); got != 10*time.Second {
		t.Fatalf("elapsed while running = %s, want 10s", got)
	}
	clock.Stop(base.Add(10 * time.Second))
	if got := clock.Elapsed(base.Add(time.Hour)); got != 10*time.Second {
		t.Fatalf("elapsed while stopped = %s, want 10s", got)
	}
	clock.Start(base.Add(time.Hour))
	if got := clock.Elapsed(base.Add(time.Hour + 5*time.Second)); got != 15*time.Second {
		t.Fatalf("elapsed after restart = %s, want 15s", got)
	}
}

func TestUpdateViewHeatUsesActivePlayClock(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	view := viewState{
		Clock:        PlayClock{Accumulated: 60 * time.Second},
		GameEpoch:    now,
		GameTime:     now.Add(60 * time.Second),
		RoundStarted: now,
	}

	updateViewHeat(&view, now)

	if view.Heat.Level != 3 {
		t.Fatalf("Heat level = %d, want active-play level 3", view.Heat.Level)
	}
}

func TestResizePauseStopsPlayClock(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	view := viewState{
		Started:      true,
		Pause:        PauseState{Resize: true},
		Game:         NewSnakeGame(5, 5, nil),
		SessionState: session.StatusRunning,
		Clock:        PlayClock{Accumulated: 10 * time.Second, ActiveSince: now.Add(-5 * time.Second), Running: true},
	}

	started, stopped := (Shell{}).syncPlayClock(&view, now)

	if started || !stopped || view.Clock.Running {
		t.Fatalf("syncPlayClock while resize-paused started=%t stopped=%t running=%t, want stopped", started, stopped, view.Clock.Running)
	}
	if got := view.Clock.Elapsed(now.Add(time.Hour)); got != 15*time.Second {
		t.Fatalf("elapsed while resize-paused = %s, want 15s", got)
	}
}

func TestUpdateViewHeatIgnoresPausedClockTime(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	view := viewState{
		Clock:        PlayClock{Accumulated: 29 * time.Second, ActiveSince: now.Add(-10 * time.Minute), Running: false},
		GameEpoch:    now,
		GameTime:     now.Add(10 * time.Minute),
		RoundStarted: now,
	}

	updateViewHeat(&view, now.Add(10*time.Minute))

	if view.Heat.Level != 1 {
		t.Fatalf("Heat level = %d, want paused time ignored at level 1", view.Heat.Level)
	}
}
