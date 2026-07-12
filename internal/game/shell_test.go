//go:build !race

package game

import (
	"context"
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Command state: running")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !screen.finiCalled {
		t.Fatal("screen Fini was not called")
	}
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Command state: running")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone))
	waitForRenderedText(t, screen, "Paused")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunDisplaysCommandHeatInHUD(t *testing.T) {
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Heat: 3/6")
	waitForRenderedText(t, screen, "Score x1.4")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Sidequest Snake [quest]")
	waitForRenderedText(t, screen, "COMBO x0")
	waitForRenderedText(t, screen, "QUEST:")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunUpdatesQuestStatsWhenCommandFinishes(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(80, 12)
	now := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	started := now.Add(-time.Minute)
	states := make(chan session.State, 4)
	states <- session.State{Status: session.StatusRunning, StartedAt: &started, GameMode: GameModeQuest}
	states <- session.State{Status: session.StatusCompleted, StartedAt: &started, DurationMillis: int64Ptr(60_000), GameMode: GameModeQuest}
	statsDir := filepath.Join(t.TempDir(), "sidequest")
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			select {
			case state := <-states:
				return state, nil
			default:
				return session.State{Status: session.StatusCompleted, StartedAt: &started, DurationMillis: int64Ptr(60_000), GameMode: GameModeQuest}, nil
			}
		},
		PollInterval: 20 * time.Millisecond,
		StatsManager: StatsManager{
			BaseDir: statsDir,
		},
		Now: func() time.Time { return now },
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "RUN FINISHED")
	if _, err := os.Stat(filepath.Join(statsDir, statsFileName)); err != nil {
		t.Fatalf("stats file was not written: %v", err)
	}
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunWarnsBeforeCommandHeatRises(t *testing.T) {
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "COMMAND HEAT RISING")
	waitForRenderedText(t, screen, "SPEED 2")
	waitForRenderedText(t, screen, "SCORE x1.2")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Heat: 1/6")
	if strings.Contains(screenText(screen), "Heat: 2/6") {
		t.Fatalf("finished command heat kept progressing:\n%s", screenText(screen))
	}
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunCallsActiveQuitHookForRunningCommand(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(40, 10)
	called := false
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusRunning}, nil
		},
		OnQuitActive: func() error {
			called = true
			return nil
		},
		PollInterval: time.Hour,
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "F10 detach/list")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !called {
		t.Fatal("OnQuitActive was not called")
	}
}

func TestRunCallsTerminalQuitHookForFinishedCommand(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(40, 10)
	activeCalled := false
	terminalCalled := false
	shell := Shell{
		NewScreen: func() (tcell.Screen, error) { return screen, nil },
		ReadState: func() (session.State, error) {
			return session.State{Status: session.StatusCompleted}, nil
		},
		OnQuitActive: func() error {
			activeCalled = true
			return nil
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

	waitForRenderedText(t, screen, "Q exit/cleanup")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !terminalCalled {
		t.Fatal("OnQuitTerminal was not called")
	}
	if activeCalled {
		t.Fatal("OnQuitActive was called for terminal state")
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
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Arrows/WASD start")
	time.Sleep(80 * time.Millisecond)
	if strings.Contains(screenText(screen), "Round over") {
		t.Fatalf("snake started before first move:\n%s", screenText(screen))
	}

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "Round over")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "R restart")

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone))
	waitForRenderedText(t, screen, "Arrows/WASD start")
	waitForMissingRenderedText(t, screen, "Round over")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

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
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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
	}

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Arrows/WASD start")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone))
	waitForRenderedText(t, screen, "GAME OVER")
	waitForRenderedText(t, screen, "Final score: 0")
	waitForRenderedText(t, screen, "R restart")

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunFreezesWithinPollIntervalWhenCommandFinishes(t *testing.T) {
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

	waitForRenderedText(t, screen, "Command finished. Q exit/cleanup")
	waitForRenderedText(t, screen, "RUN FINISHED")
	waitForRenderedText(t, screen, "Final score: 0")
	waitForRenderedText(t, screen, "Exit code: 0")
	waitForRenderedText(t, screen, "Runtime: 00:00:03")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

	waitForRenderedText(t, screen, "Sidequest")
	screen.SetSize(80, 20)
	screen.PostEvent(tcell.NewEventResize(80, 20))
	waitForRenderedText(t, screen, "Command state: running")
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))

	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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

	errc := make(chan error, 1)
	go func() {
		errc <- shell.Run(context.Background())
	}()

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

	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone))
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
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
