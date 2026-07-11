package game

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/WBT112/sidequest/internal/session"
)

const DefaultPollInterval = 250 * time.Millisecond

type StateReader func() (session.State, error)

type Shell struct {
	NewScreen    func() (tcell.Screen, error)
	ReadState    StateReader
	PollInterval time.Duration
}

type viewState struct {
	SessionState string
	Paused       bool
	Frozen       bool
	Message      string
}

func (s Shell) Run(ctx context.Context) error {
	if s.ReadState == nil {
		return fmt.Errorf("missing session state reader")
	}

	newScreen := s.NewScreen
	if newScreen == nil {
		newScreen = tcell.NewScreen
	}
	screen, err := newScreen()
	if err != nil {
		return fmt.Errorf("create tcell screen: %w", err)
	}
	if err := screen.Init(); err != nil {
		return fmt.Errorf("initialize tcell screen: %w", err)
	}
	defer screen.Fini()

	pollInterval := s.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}

	events := make(chan tcell.Event, 8)
	done := make(chan struct{})
	go pollEvents(screen, events, done)
	defer func() {
		close(done)
		screen.PostEvent(tcell.NewEventInterrupt(nil))
	}()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	state, err := s.ReadState()
	if err != nil {
		return err
	}
	view := viewState{SessionState: state.Status, Frozen: terminalState(state.Status)}
	render(screen, view)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-events:
			switch typed := event.(type) {
			case *tcell.EventKey:
				switch {
				case typed.Key() == tcell.KeyRune && (typed.Rune() == 'q' || typed.Rune() == 'Q'):
					return nil
				case typed.Key() == tcell.KeyRune && (typed.Rune() == 'p' || typed.Rune() == 'P'):
					view.Paused = !view.Paused
					render(screen, view)
				}
			case *tcell.EventResize:
				screen.Sync()
				render(screen, view)
			}
		case <-ticker.C:
			state, err := s.ReadState()
			if err != nil {
				view.Message = err.Error()
				render(screen, view)
				continue
			}

			view.SessionState = state.Status
			if terminalState(state.Status) {
				view.Frozen = true
			}
			render(screen, view)
		}
	}
}

func pollEvents(screen tcell.Screen, events chan<- tcell.Event, done <-chan struct{}) {
	for {
		event := screen.PollEvent()
		select {
		case <-done:
			return
		default:
		}
		if event == nil {
			continue
		}
		select {
		case events <- event:
		case <-done:
			return
		}
	}
}

func render(screen tcell.Screen, view viewState) {
	screen.Clear()
	width, height := screen.Size()
	if width <= 0 || height <= 0 {
		screen.Show()
		return
	}

	style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	titleStyle := style.Bold(true).Foreground(tcell.ColorAqua)
	statusStyle := style.Foreground(statusColor(view.SessionState))
	secondaryStyle := style.Foreground(tcell.ColorGray)

	drawBox(screen, 0, 0, width, height, style)

	lines := []renderLine{
		{0, "Sidequest", titleStyle},
		{2, "Command state: " + displayState(view.SessionState), statusStyle},
		{3, "P pause/resume    Q leave game pane", secondaryStyle},
	}
	if view.Paused {
		lines = append(lines, renderLine{5, "Paused", secondaryStyle})
	}
	if view.Frozen {
		lines = append(lines, renderLine{6, "Command finished. Game area frozen.", secondaryStyle})
	}
	if view.Message != "" {
		lines = append(lines, renderLine{8, view.Message, secondaryStyle})
	}

	for _, line := range lines {
		drawText(screen, 1, line.y, width-2, line.text, line.style)
	}

	screen.Show()
}

type renderLine struct {
	y     int
	text  string
	style tcell.Style
}

func drawBox(screen tcell.Screen, x int, y int, width int, height int, style tcell.Style) {
	if width < 2 || height < 2 {
		return
	}

	right := x + width - 1
	bottom := y + height - 1
	for col := x; col <= right; col++ {
		screen.SetContent(col, y, tcell.RuneHLine, nil, style)
		screen.SetContent(col, bottom, tcell.RuneHLine, nil, style)
	}
	for row := y; row <= bottom; row++ {
		screen.SetContent(x, row, tcell.RuneVLine, nil, style)
		screen.SetContent(right, row, tcell.RuneVLine, nil, style)
	}
	screen.SetContent(x, y, tcell.RuneULCorner, nil, style)
	screen.SetContent(right, y, tcell.RuneURCorner, nil, style)
	screen.SetContent(x, bottom, tcell.RuneLLCorner, nil, style)
	screen.SetContent(right, bottom, tcell.RuneLRCorner, nil, style)
}

func drawText(screen tcell.Screen, x int, y int, maxWidth int, text string, style tcell.Style) {
	if y < 0 || maxWidth <= 0 {
		return
	}
	for index, r := range text {
		if index >= maxWidth {
			return
		}
		screen.SetContent(x+index, y, r, nil, style)
	}
}

func displayState(state string) string {
	if state == "" {
		return session.StatusCreated
	}
	return state
}

func terminalState(state string) bool {
	switch state {
	case session.StatusCompleted, session.StatusFailed, session.StatusInterrupted, session.StatusStartFailed:
		return true
	default:
		return false
	}
}

func statusColor(state string) tcell.Color {
	switch state {
	case session.StatusCompleted:
		return tcell.ColorGreen
	case session.StatusFailed, session.StatusStartFailed:
		return tcell.ColorRed
	case session.StatusInterrupted:
		return tcell.ColorYellow
	default:
		return tcell.ColorWhite
	}
}
