//go:build !race

package game

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/WBT112/sidequest/internal/session"
)

func TestRunInitializesAndFinalizesScreenOnQuit(t *testing.T) {
	screen := &finalizingScreen{SimulationScreen: tcell.NewSimulationScreen("")}
	screen.SetSize(60, 12)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Command state: running")
	cancelShell(t, cancel, errc)
	if !screen.finiCalled {
		t.Fatal("screen Fini was not called")
	}
}

func TestRunIgnoresActiveQForRunningCommand(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(60, 12)
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)
	waitForRenderedText(t, screen, "F9 hide")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))
	select {
	case err := <-errc:
		t.Fatalf("Run returned after active Q: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancelShell(t, cancel, errc)
}

func TestRunTogglesPause(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(60, 12)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Command state: running")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone))
	waitForRenderedText(t, screen, "PAUSED - PRESS P TO RESUME")
	cancelShell(t, cancel, errc)
}

func TestRunDoesNotIncreaseHeatBeforeActivePlay(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	now := time.Date(2026, 7, 11, 18, 1, 0, 0, time.UTC)
	started := now.Add(-60 * time.Second)
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning, StartedAt: &started}, nil
		},
		PollInterval: time.Hour,
		Now:          func() time.Time { return now },
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Heat: 1/6")
	waitForRenderedText(t, screen, "Score x1.0")
	cancelShell(t, cancel, errc)
}

func TestRunDisplaysQuestModeHUD(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(80, 12)
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning, GameMode: GameModeQuest}, nil
		},
		PollInterval: time.Hour,
		Now:          func() time.Time { return now },
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Sidequest Snake [quest]")
	waitForRenderedText(t, screen, "COMBO x0")
	waitForRenderedText(t, screen, "QUEST:")
	cancelShell(t, cancel, errc)
}

func TestRenderCentersQuestHUDLines(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	defer screen.Fini()

	width, height := 100, 20
	screen.SetSize(width, height)

	view := viewState{
		SessionState: session.StatusRunning,
		Heat:         HeatByLevel(1),
		Quest: &QuestState{
			Mode:            GameModeQuest,
			Mission:         Mission{ID: MissionGolden2, Label: "Collect 2 Golden Bytes", Target: 2},
			MissionProgress: 0,
		},
	}

	render(screen, view)

	cases := []struct {
		y    int
		text string
	}{
		{y: 1, text: "Command state: running"},
		{y: 2, text: "SCORE 0  COMBO x0  HEAT 1 x1.0"},
		{y: 3, text: "QUEST: Collect 2 Golden Bytes 0/2"},
	}

	for _, test := range cases {
		got := rowTextIndex(screen, test.y, test.text)
		if got < 0 {
			t.Fatalf("row %d missing %q:\n%s", test.y, test.text, screenText(screen))
		}
		want := 1 + ((width - 2 - textDisplayWidth(test.text)) / 2)
		if got != want {
			t.Fatalf("row %d start column = %d, want %d for %q", test.y, got, want, test.text)
		}
	}
}

func TestStatusLineKeepsCompletionChoiceAboveQuestAndHeatMessages(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	quest := &QuestState{
		Mode:            GameModeQuest,
		Mission:         Mission{ID: MissionGolden2, Label: "Collect 2 Golden Bytes", Target: 2},
		MissionProgress: 1,
		Message:         "PICKUP: SHIELD - NEXT COLLISION BLOCKED",
		MessageUntil:    now.Add(time.Second),
	}

	tests := []struct {
		name string
		view viewState
	}{
		{
			name: "mission",
			view: viewState{
				Completion: CompletionUndecided,
				Quest: &QuestState{
					Mode:            GameModeQuest,
					Mission:         Mission{ID: MissionGolden2, Label: "Collect 2 Golden Bytes", Target: 2},
					MissionProgress: 1,
				},
			},
		},
		{
			name: "pickup",
			view: viewState{Completion: CompletionUndecided, Quest: quest},
		},
		{
			name: "heat",
			view: viewState{Completion: CompletionUndecided, Quest: quest, HeatNotice: "COMMAND HEAT RISING... SPEED 3  SCORE x1.4"},
		},
	}

	for _, test := range tests {
		got := statusLine(test.view)
		if !strings.Contains(got, "Command finished  C continue  Q quit") {
			t.Fatalf("%s statusLine = %q, want completion controls", test.name, got)
		}
		for _, unwanted := range []string{"QUEST:", "PICKUP:", "COMMAND HEAT"} {
			if strings.Contains(got, unwanted) {
				t.Fatalf("%s statusLine = %q, should not contain %q", test.name, got, unwanted)
			}
		}
	}
}

func TestStatusLineReturnsQuestMessageAfterCompletionContinue(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	view := viewState{
		Completion: CompletionContinue,
		Quest: &QuestState{
			Mode:         GameModeQuest,
			Message:      "PICKUP: TURBO",
			MessageUntil: now.Add(time.Second),
		},
	}

	if got := statusLine(view); got != "PICKUP: TURBO" {
		t.Fatalf("statusLine = %q, want Quest message after continue", got)
	}
}

func TestRenderCompactStatusLineKeepsCompletionPriority(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(36, 12)

	render(screen, viewState{
		SessionState: session.StatusCompleted,
		Completion:   CompletionUndecided,
		Heat:         HeatByLevel(3),
		HeatNotice:   "COMMAND HEAT RISING... SPEED 3  SCORE x1.4",
		Quest: &QuestState{
			Mode:    GameModeQuest,
			Message: "PICKUP: DOUBLE SCORE",
		},
	})

	text := screenText(screen)
	if !strings.Contains(text, "Command finished") {
		t.Fatalf("compact screen missing completion status:\n%s", text)
	}
	for _, unwanted := range []string{"PICKUP:", "COMMAND HEAT"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("compact screen contains %q despite completion priority:\n%s", unwanted, text)
		}
	}
}

func TestRunUpdatesQuestStatsWhenCommandFinishIsQuit(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(80, 12)
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	started := now.Add(-time.Minute)
	var finished atomic.Bool
	statsDir := filepath.Join(t.TempDir(), "sidequest")
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if finished.Load() {
				return session.State{Status: session.StatusCompleted, StartedAt: &started, DurationMillis: int64Ptr(60_000), GameMode: GameModeQuest}, nil
			}
			return session.State{Status: session.StatusRunning, StartedAt: &started, GameMode: GameModeQuest}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: time.Second,
		StatsManager: StatsManager{
			BaseDir: statsDir,
		},
		Now: func() time.Time { return now },
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "QUEST:")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone))
	finished.Store(true)
	waitForRenderedText(t, screen, "C Continue")
	if _, err := os.Stat(filepath.Join(statsDir, statsFileName)); !os.IsNotExist(err) {
		t.Fatalf("stats file exists before completion choice: %v", err)
	}
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "COMMAND FINISHED")
	if _, err := os.Stat(filepath.Join(statsDir, statsFileName)); err != nil {
		t.Fatalf("stats file was not written: %v", err)
	}
	cancelShell(t, cancel, errc)
}

func TestRunDoesNotWarnBeforeHeatRisesWithoutActivePlay(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	now := time.Date(2026, 7, 11, 18, 0, 25, 0, time.UTC)
	started := now.Add(-25 * time.Second)
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning, StartedAt: &started}, nil
		},
		PollInterval: time.Hour,
		Now:          func() time.Time { return now },
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Heat: 1/6")
	if strings.Contains(screenText(screen), "COMMAND HEAT RISING") {
		t.Fatalf("heat warning appeared before active play:\n%s", screenText(screen))
	}
	cancelShell(t, cancel, errc)
}

func TestRunFreezesCommandHeatWhenCommandAlreadyFinished(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	now := time.Date(2026, 7, 11, 19, 0, 0, 0, time.UTC)
	durationMillis := int64(29_999)
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusCompleted, DurationMillis: &durationMillis}, nil
		},
		PollInterval: time.Hour,
		Now:          func() time.Time { return now },
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Heat: 1/6")
	if strings.Contains(screenText(screen), "Heat: 2/6") {
		t.Fatalf("finished command heat kept progressing:\n%s", screenText(screen))
	}
	cancelShell(t, cancel, errc)
}

func TestRunCallsTerminalQuitHookForFinishedCommand(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(40, 10)
	terminalCalled := false
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusCompleted}, nil
		},
		OnQuitTerminal: func() error {
			terminalCalled = true
			return nil
		},
		PollInterval: time.Hour,
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Command finished  Q quit  F9 hide")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !terminalCalled {
		t.Fatal("OnQuitTerminal was not called")
	}
}

func TestRunWaitsForFirstMoveBeforeStartingSnake(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(5, 7)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 10 * time.Millisecond,
		StatsManager: StatsManager{
			BaseDir: filepath.Join(t.TempDir(), "sidequest"),
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	time.Sleep(80 * time.Millisecond)
	if strings.Contains(screenText(screen), "Round over") {
		t.Fatalf("snake started before first move:\n%s", screenText(screen))
	}

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	cancelShell(t, cancel, errc)
}

func TestRunRestartsSnakeAfterRoundOver(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(5, 7)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 10 * time.Millisecond,
		StatsManager: StatsManager{
			BaseDir: filepath.Join(t.TempDir(), "sidequest"),
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "R Restart")

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone))
	waitForRenderedText(t, screen, "Arrows/WASD start")
	waitForMissingRenderedText(t, screen, "Round over")
	cancelShell(t, cancel, errc)
}

func TestRunSavesQuestStatsBeforeRestartAfterRoundOver(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(5, 7)
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning, GameMode: GameModeQuest}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 10 * time.Millisecond,
		StatsManager: manager,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "QUEST:")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "R Restart")

	stats := manager.load()
	if stats.GamesPlayed != 1 {
		t.Fatalf("GamesPlayed before restart = %d, want 1", stats.GamesPlayed)
	}

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone))
	waitForRenderedText(t, screen, "QUEST:")
	stats = manager.load()
	if stats.GamesPlayed != 1 {
		t.Fatalf("GamesPlayed after restart = %d, want still 1", stats.GamesPlayed)
	}
	cancelShell(t, cancel, errc)
}

func TestRunDoesNotDuplicateQuestStatsWhenCommandCompletesAfterRoundOver(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(5, 7)
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	var finished atomic.Bool

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if finished.Load() {
				return session.State{Status: session.StatusCompleted, GameMode: GameModeQuest}, nil
			}
			return session.State{Status: session.StatusRunning, GameMode: GameModeQuest}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: 10 * time.Millisecond,
		StatsManager: manager,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "QUEST:")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "R Restart")
	if stats := manager.load(); stats.GamesPlayed != 1 {
		t.Fatalf("GamesPlayed after round over = %d, want 1", stats.GamesPlayed)
	}

	finished.Store(true)
	waitForRenderedText(t, screen, "Command state: completed")
	if stats := manager.load(); stats.GamesPlayed != 1 {
		t.Fatalf("GamesPlayed after command completion = %d, want still 1", stats.GamesPlayed)
	}
	cancelShell(t, cancel, errc)
}

func TestRunDirectionInputDoesNotPostponeNextMove(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(5, 7)
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	var nowNanos atomic.Int64
	nowNanos.Store(base.UnixNano())

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 100 * time.Millisecond,
		Now: func() time.Time {
			return time.Unix(0, nowNanos.Load())
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	arena := arenaForScreen(screen)
	nextHead := Point{X: arena.Width / 2, Y: arena.Height/2 + 1}
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone))
	waitForRenderedText(t, screen, "Arrows/WASD move")

	nowNanos.Store(base.Add(90 * time.Millisecond).UnixNano())
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone))
	time.Sleep(20 * time.Millisecond)
	nowNanos.Store(base.Add(105 * time.Millisecond).UnixNano())

	waitForRenderedCell(t, screen, nextHead, tcell.RuneBlock)
	cancelShell(t, cancel, errc)
}

func TestRunFocusPauseStopsMovementAndResumesWithFullInterval(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(5, 7)
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	var nowNanos atomic.Int64
	var focusActive atomic.Bool
	nowNanos.Store(base.UnixNano())
	focusActive.Store(true)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		ReadFocus: func() (bool, error) {
			return focusActive.Load(), nil
		},
		PollInterval: time.Hour,
		GameInterval: 100 * time.Millisecond,
		Now: func() time.Time {
			return time.Unix(0, nowNanos.Load())
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	arena := arenaForScreen(screen)
	nextHeads := []Point{
		{X: arena.Width/2 + 1, Y: arena.Height / 2},
		{X: arena.Width/2 + 2, Y: arena.Height / 2},
	}
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone))
	waitForRenderedText(t, screen, "Arrows/WASD move")

	focusActive.Store(false)
	nowNanos.Store(base.Add(150 * time.Millisecond).UnixNano())
	waitForRenderedText(t, screen, "PAUSED - COMMAND PANE ACTIVE")
	if strings.Contains(screenText(screen), "Round over") {
		t.Fatalf("snake moved while focus-paused:\n%s", screenText(screen))
	}

	focusActive.Store(true)
	nowNanos.Store(base.Add(260 * time.Millisecond).UnixNano())
	waitForMissingRenderedText(t, screen, "PAUSED - COMMAND PANE ACTIVE")
	if strings.Contains(screenText(screen), "Round over") {
		t.Fatalf("snake moved immediately after focus return:\n%s", screenText(screen))
	}

	nowNanos.Store(base.Add(370 * time.Millisecond).UnixNano())
	waitForAnyRenderedCell(t, screen, nextHeads, tcell.RuneBlock)
	cancelShell(t, cancel, errc)
}

func TestRunContinueAfterCommandFinishResumesSameRoundWithFullInterval(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(40, 12)
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	var nowNanos atomic.Int64
	var completed atomic.Bool
	nowNanos.Store(base.UnixNano())

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if completed.Load() {
				exitCode := 0
				return session.State{Status: session.StatusCompleted, ExitCode: &exitCode}, nil
			}
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: 100 * time.Millisecond,
		Now: func() time.Time {
			return time.Unix(0, nowNanos.Load())
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	arena := arenaForScreen(screen)
	nextHeads := []Point{
		{X: arena.Width/2 + 1, Y: arena.Height / 2},
		{X: arena.Width/2 + 2, Y: arena.Height / 2},
	}
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone))
	waitForRenderedText(t, screen, "Arrows/WASD move")

	completed.Store(true)
	waitForRenderedText(t, screen, "C Continue")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))
	waitForMissingRenderedText(t, screen, "C Continue")
	waitForMissingRenderedText(t, screen, "FINAL SCORE")

	nowNanos.Store(base.Add(90 * time.Millisecond).UnixNano())
	time.Sleep(30 * time.Millisecond)
	for _, point := range nextHeads {
		main, _, _, _ := screen.GetContent(arena.CellX(point.X), arena.CellY(point.Y))
		if main == tcell.RuneBlock {
			t.Fatalf("snake moved before full post-command interval:\n%s", screenText(screen))
		}
	}

	nowNanos.Store(base.Add(120 * time.Millisecond).UnixNano())
	waitForAnyRenderedCell(t, screen, nextHeads, tcell.RuneBlock)
	cancelShell(t, cancel, errc)
}

func TestRunContinueAfterCommandFinishFreezesCommandHeat(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	var nowNanos atomic.Int64
	var completed atomic.Bool
	nowNanos.Store(base.UnixNano())

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if completed.Load() {
				return session.State{Status: session.StatusCompleted}, nil
			}
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: time.Hour,
		Now: func() time.Time {
			return time.Unix(0, nowNanos.Load())
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone))
	waitForRenderedText(t, screen, "Arrows/WASD move")
	nowNanos.Store(base.Add(35 * time.Second).UnixNano())
	waitForRenderedText(t, screen, "Heat: 2/6")

	completed.Store(true)
	waitForRenderedText(t, screen, "C Continue")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))
	nowNanos.Store(base.Add(70 * time.Second).UnixNano())
	time.Sleep(30 * time.Millisecond)
	if strings.Contains(screenText(screen), "Heat: 3/6") {
		t.Fatalf("command heat increased after post-command continue:\n%s", screenText(screen))
	}
	waitForRenderedText(t, screen, "Heat: 2/6")
	cancelShell(t, cancel, errc)
}

func TestRunManualPauseSurvivesFocusRoundTrip(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(60, 12)
	var focusActive atomic.Bool
	focusActive.Store(true)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		ReadFocus: func() (bool, error) {
			return focusActive.Load(), nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Command state: running")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone))
	waitForRenderedText(t, screen, "PAUSED - PRESS P TO RESUME")
	focusActive.Store(false)
	waitForRenderedText(t, screen, "PAUSED - MANUAL + COMMAND FOCUS")
	focusActive.Store(true)
	waitForRenderedText(t, screen, "PAUSED - PRESS P TO RESUME")
	cancelShell(t, cancel, errc)
}

func TestRunShowsCenteredResultPanelAfterRoundOver(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(32, 12)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 10 * time.Millisecond,
		StatsManager: StatsManager{
			BaseDir: filepath.Join(t.TempDir(), "sidequest"),
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "GAME OVER")
	waitForRenderedText(t, screen, "FINAL SCORE  0")
	waitForRenderedText(t, screen, "TOP 5")
	waitForRenderedText(t, screen, "R Restart")
	cancelShell(t, cancel, errc)
}

func TestRunHighscoreNameEntryReplacesDefaultAndPersists(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(32, 12)
	statsDir := filepath.Join(t.TempDir(), "sidequest")

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 10 * time.Millisecond,
		StatsManager: StatsManager{
			BaseDir: statsDir,
		},
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'A', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'L', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'X', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "ALX")
	cancelShell(t, cancel, errc)
	entries := (StatsManager{BaseDir: statsDir}).Leaderboard(GameModeClassic)
	if len(entries) != 1 || entries[0].PlayerName != "ALX" {
		t.Fatalf("leaderboard = %#v, want ALX entry", entries)
	}
}

func TestRunNonQualifyingScoreSkipsNameEntry(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(32, 12)
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	for _, score := range []int{500, 400, 300, 200, 100} {
		if _, _, err := manager.AddLeaderboardScore(GameModeClassic, score, "seed"); err != nil {
			t.Fatalf("AddLeaderboardScore returned error: %v", err)
		}
	}

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
		GameInterval: 10 * time.Millisecond,
		StatsManager: manager,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "GAME OVER")
	waitForMissingRenderedText(t, screen, "NEW HIGH SCORE")
	cancelShell(t, cancel, errc)
}

func TestRunCommandCompletionDuringNameEntryKeepsPendingScore(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(40, 14)
	var completed atomic.Bool
	statsDir := filepath.Join(t.TempDir(), "sidequest")

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if completed.Load() {
				return session.State{Status: session.StatusCompleted}, nil
			}
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: 10 * time.Millisecond,
		StatsManager: StatsManager{
			BaseDir: statsDir,
		},
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "NEW HIGH SCORE")
	completed.Store(true)
	waitForRenderedText(t, screen, "Command state: completed")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'Z', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitForRenderedText(t, screen, "COMMAND FINISHED")
	waitForRenderedText(t, screen, "Q Quit")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone))
	waitForMissingRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	entries := (StatsManager{BaseDir: statsDir}).Leaderboard(GameModeClassic)
	if len(entries) != 1 || entries[0].PlayerName != "Z" {
		t.Fatalf("leaderboard = %#v, want saved pending score", entries)
	}
}

func TestRunShowsCompletionChoiceWithinPollIntervalWhenCommandFinishes(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	exitCode := 0
	durationMillis := int64(2500)
	states := make(chan session.State, 4)
	states <- session.State{Status: session.StatusRunning}
	states <- session.State{Status: session.StatusCompleted, ExitCode: &exitCode, DurationMillis: &durationMillis}

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			select {
			case state := <-states:
				return state, nil
			default:
				return session.State{Status: session.StatusCompleted}, nil
			}
		},
		PollInterval: 20 * time.Millisecond,
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Command finished  C continue  Q quit")
	waitForRenderedText(t, screen, "COMMAND FINISHED")
	waitForRenderedText(t, screen, "C Continue")
	waitForMissingRenderedText(t, screen, "FINAL SCORE")
	waitForRenderedText(t, screen, "Exit code: 0")
	waitForRenderedText(t, screen, "Runtime: 00:00:03")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunDoesNotPersistZeroScoreWhenCommandFailsImmediately(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	statsDir := filepath.Join(t.TempDir(), "sidequest")
	var failed atomic.Bool

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if failed.Load() {
				return session.State{Status: session.StatusFailed}, nil
			}
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: time.Second,
		StatsManager: StatsManager{
			BaseDir: statsDir,
		},
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone))
	failed.Store(true)
	waitForRenderedText(t, screen, "Command state: failed")
	waitForRenderedText(t, screen, "COMMAND FINISHED")
	waitForMissingRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if entries := (StatsManager{BaseDir: statsDir}).Leaderboard(GameModeClassic); len(entries) != 0 {
		t.Fatalf("classic leaderboard = %#v, want empty", entries)
	}
}

func TestRunDoesNotUpdateQuestStatsWhenCommandFailsImmediately(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	statsDir := filepath.Join(t.TempDir(), "sidequest")
	var failed atomic.Bool

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			if failed.Load() {
				return session.State{Status: session.StatusFailed, GameMode: GameModeQuest}, nil
			}
			return session.State{Status: session.StatusRunning, GameMode: GameModeQuest}, nil
		},
		PollInterval: 20 * time.Millisecond,
		GameInterval: time.Second,
		StatsManager: StatsManager{
			BaseDir: statsDir,
		},
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "QUEST:")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone))
	failed.Store(true)
	waitForRenderedText(t, screen, "Command state: failed")
	waitForMissingRenderedText(t, screen, "NEW HIGH SCORE")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(statsDir, statsFileName)); !os.IsNotExist(err) {
		t.Fatalf("stats file exists after immediate command failure: %v", err)
	}
}

func TestRunHandlesResize(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(40, 10)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Sidequest")
	screen.SetSize(80, 20)
	screen.PostEvent(tcell.NewEventResize(80, 20))
	waitForRenderedText(t, screen, "Command state: running")
	cancelShell(t, cancel, errc)
}

func TestRunDrawsColoredPlayfieldWithThickWalls(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(40, 12)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "Arrows/WASD start")
	topWall, _, topStyle, _ := screen.GetContent(20, 4)
	sideWall, _, _, _ := screen.GetContent(0, 6)
	inside, _, insideStyle, _ := screen.GetContent(20, 6)
	_, wallBackground, _ := topStyle.Decompose()
	_, insideBackground, _ := insideStyle.Decompose()
	if topWall != tcell.RuneBlock || sideWall != tcell.RuneBlock {
		t.Fatalf("wall runes = %q %q, want block walls", topWall, sideWall)
	}
	if wallBackground != tcell.ColorTeal {
		t.Fatalf("wall background = %v, want teal", wallBackground)
	}
	if inside != ' ' || insideBackground != tcell.ColorDarkSlateGray {
		t.Fatalf("inside cell = %q background=%v, want colored playfield", inside, insideBackground)
	}

	cancelShell(t, cancel, errc)
}

func TestRunDrawsMonochromeClassicWithoutColors(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(40, 12)

	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning, NoColor: true}, nil
		},
		PollInterval: time.Hour,
	}

	cancel, errc := runShellCancellable(shell)

	waitForRenderedText(t, screen, "MODE classic")
	topWall, _, topStyle, _ := screen.GetContent(20, 4)
	sideWall, _, _, _ := screen.GetContent(0, 6)
	inside, _, insideStyle, _ := screen.GetContent(20, 6)
	_, wallBackground, _ := topStyle.Decompose()
	_, insideBackground, _ := insideStyle.Decompose()
	if topWall != tcell.RuneBlock || sideWall != tcell.RuneBlock {
		t.Fatalf("wall runes = %q %q, want block walls", topWall, sideWall)
	}
	if wallBackground != tcell.ColorDefault {
		t.Fatalf("wall background = %v, want default", wallBackground)
	}
	if inside != ' ' || insideBackground != tcell.ColorDefault {
		t.Fatalf("inside cell = %q background=%v, want monochrome playfield", inside, insideBackground)
	}
	assertScreenHasNoColors(t, screen)

	cancelShell(t, cancel, errc)
}

func TestRenderDrawsMonochromeQuestObjectsAndResultPanel(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init returned error: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	game := NewSnakeGame(20, 12, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}}
	game.Food = Point{X: 8, Y: 5}
	quest := NewQuestState(GameModeQuest, now, fixedRandom(0), 20, 12)
	quest.Mission = Mission{ID: MissionFood15, Label: "Collect 15 food", Target: 15}
	quest.MissionProgress = 4
	quest.Golden = GoldenByte{Position: Point{X: 10, Y: 5}, Active: true}
	quest.Pickup = UpgradePickup{Upgrade: UpgradeDoubleScore, Position: Point{X: 12, Y: 5}, Active: true}
	view := viewState{
		State:        session.State{Status: session.StatusRunning, NoColor: true},
		SessionState: session.StatusRunning,
		NoColor:      true,
		Game:         game,
		Quest:        quest,
		Heat:         HeatByLevel(1),
		GameTime:     now,
	}

	render(screen, view)

	text := screenText(screen)
	for _, want := range []string{"QUEST: Collect 15 food 4/15", "()", "<>", "x2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("monochrome quest render missing %q:\n%s", want, text)
		}
	}
	assertScreenHasNoColors(t, screen)

	view.Completion = CompletionUndecided
	render(screen, view)

	text = screenText(screen)
	for _, want := range []string{"COMMAND FINISHED", "C Continue", "Q Quit"} {
		if !strings.Contains(text, want) {
			t.Fatalf("monochrome completion render missing %q:\n%s", want, text)
		}
	}
	assertScreenHasNoColors(t, screen)

	game.Over = true
	view.Completion = CompletionNone
	view.ResultScore = 920
	view.RoundFinalized = true
	view.Leaderboard = []LeaderboardEntry{{Score: 920, PlayerName: "YOU"}}
	view.CurrentRank = 1
	render(screen, view)

	text = screenText(screen)
	for _, want := range []string{"GAME OVER", "FINAL SCORE  920", "TOP 5", "<- YOU"} {
		if !strings.Contains(text, want) {
			t.Fatalf("monochrome result render missing %q:\n%s", want, text)
		}
	}
	assertScreenHasNoColors(t, screen)
}

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
		RoundHeat:    1,
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

func runShellCancellable(shell Shell) (context.CancelFunc, chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(ctx)
	}()
	return cancel, errc
}

func cancelShell(t *testing.T, cancel context.CancelFunc, errc <-chan error) {
	t.Helper()
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned error %v, want context canceled", err)
	}
}

func waitForRenderedText(t *testing.T, screen tcell.SimulationScreen, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(screenText(screen), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("screen did not contain %q:\n%s", want, screenText(screen))
}

func waitForMissingRenderedText(t *testing.T, screen tcell.SimulationScreen, unwanted string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(screenText(screen), unwanted) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("screen still contained %q:\n%s", unwanted, screenText(screen))
}

func waitForRenderedCell(t *testing.T, screen tcell.SimulationScreen, point Point, want rune) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		arena := arenaForScreen(screen)
		main, _, _, _ := screen.GetContent(arena.CellX(point.X), arena.CellY(point.Y))
		if main == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("cell %#v did not contain %q:\n%s", point, want, screenText(screen))
}

func waitForAnyRenderedCell(t *testing.T, screen tcell.SimulationScreen, points []Point, want rune) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		arena := arenaForScreen(screen)
		for _, point := range points {
			main, _, _, _ := screen.GetContent(arena.CellX(point.X), arena.CellY(point.Y))
			if main == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("none of cells %#v contained %q:\n%s", points, want, screenText(screen))
}

func screenText(screen tcell.SimulationScreen) string {
	width, height := screen.Size()
	var builder strings.Builder
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			main, _, _, _ := screen.GetContent(x, y)
			if main == 0 {
				builder.WriteRune(' ')
				continue
			}
			builder.WriteRune(main)
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}

func assertScreenHasNoColors(t *testing.T, screen tcell.SimulationScreen) {
	t.Helper()
	width, height := screen.Size()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			main, _, style, _ := screen.GetContent(x, y)
			foreground, background, _ := style.Decompose()
			if foreground != tcell.ColorDefault || background != tcell.ColorDefault {
				t.Fatalf("cell (%d,%d) %q has foreground=%v background=%v, want defaults", x, y, main, foreground, background)
			}
		}
	}
}

func rowTextIndex(screen tcell.SimulationScreen, row int, text string) int {
	haystack := []rune(screenRowText(screen, row))
	needle := []rune(text)
	if len(needle) == 0 {
		return 0
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		matched := true
		for offset := range needle {
			if haystack[start+offset] != needle[offset] {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func screenRowText(screen tcell.SimulationScreen, row int) string {
	width, height := screen.Size()
	if row < 0 || row >= height {
		return ""
	}
	var builder strings.Builder
	for x := 0; x < width; x++ {
		main, _, _, _ := screen.GetContent(x, row)
		if main == 0 {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(main)
	}
	return builder.String()
}

func int64Ptr(value int64) *int64 {
	return &value
}

type finalizingScreen struct {
	tcell.SimulationScreen
	finiCalled bool
}

func (s *finalizingScreen) Fini() {
	s.finiCalled = true
	s.SimulationScreen.Fini()
}
