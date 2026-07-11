package game

import (
	"context"
	"strings"
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

func TestRunFreezesWithinPollIntervalWhenCommandFinishes(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	screen.SetSize(70, 12)
	states := make(chan session.State, 4)
	states <- session.State{Status: session.StatusRunning}
	states <- session.State{Status: session.StatusCompleted}

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

	waitForRenderedText(t, screen, "Command finished. Game area frozen.")
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

type finalizingScreen struct {
	tcell.SimulationScreen
	finiCalled bool
}

func (s *finalizingScreen) Fini() {
	s.finiCalled = true
	s.SimulationScreen.Fini()
}
