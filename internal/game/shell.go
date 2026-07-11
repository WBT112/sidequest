package game

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/WBT112/sidequest/internal/session"
)

const DefaultPollInterval = 250 * time.Millisecond
const DefaultGameInterval = 120 * time.Millisecond

type StateReader func() (session.State, error)

type Shell struct {
	NewScreen    func() (tcell.Screen, error)
	ReadState    StateReader
	PollInterval time.Duration
	GameInterval time.Duration
}

type viewState struct {
	SessionState string
	Paused       bool
	Frozen       bool
	Message      string
	Game         *SnakeGame
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
	gameInterval := s.GameInterval
	if gameInterval <= 0 {
		gameInterval = DefaultGameInterval
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
	gameTicker := time.NewTicker(gameInterval)
	defer gameTicker.Stop()

	state, err := s.ReadState()
	if err != nil {
		return err
	}
	game := newSnakeGameForScreen(screen)
	view := viewState{SessionState: state.Status, Frozen: terminalState(state.Status), Game: game}
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
				default:
					if direction, ok := directionFromKey(typed); ok && !view.Frozen && !game.Over {
						game.ChangeDirection(direction)
					}
				}
			case *tcell.EventResize:
				screen.Sync()
				if !view.Frozen {
					game.Resize(boardSize(screen))
				}
				render(screen, view)
			}
		case <-gameTicker.C:
			if !view.Paused && !view.Frozen && !game.Over {
				game.Step()
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
	scoreStyle := style.Foreground(tcell.ColorGreen)

	drawBox(screen, 0, 0, width, height, style)

	controlLine := "Arrows/WASD move  P pause/resume  Q leave"
	if view.Paused {
		controlLine = "Paused  P resume  Q leave"
	}
	if view.Frozen {
		controlLine = "Command finished. Game area frozen.  Q leave"
	}
	if view.Game != nil && view.Game.Over && !view.Frozen {
		controlLine = "Round over.  Q leave"
	}

	lines := []renderLine{
		{0, "Sidequest Snake", titleStyle},
		{1, "Command state: " + displayState(view.SessionState), statusStyle},
		{2, "Score: " + scoreText(view.Game), scoreStyle},
		{3, controlLine, secondaryStyle},
	}
	if view.Message != "" {
		lines = append(lines, renderLine{height - 2, view.Message, secondaryStyle})
	}

	for _, line := range lines {
		drawText(screen, 1, line.y, width-2, line.text, line.style)
	}

	drawSnake(screen, view.Game, style)

	screen.Show()
}

type renderLine struct {
	y     int
	text  string
	style tcell.Style
}

func newSnakeGameForScreen(screen tcell.Screen) *SnakeGame {
	width, height := boardSize(screen)
	return NewSnakeGame(width, height, nil)
}

func boardSize(screen tcell.Screen) (int, int) {
	_, _, width, height := boardBounds(screen)
	return width, height
}

func boardBounds(screen tcell.Screen) (int, int, int, int) {
	screenWidth, screenHeight := screen.Size()
	if screenWidth <= 0 || screenHeight <= 0 {
		return 0, 0, 1, 1
	}

	x := 1
	y := 4
	width := screenWidth - 2
	height := screenHeight - y - 1
	if screenHeight < 7 {
		y = 1
		height = screenHeight - 2
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return x, y, width, height
}

func drawSnake(screen tcell.Screen, game *SnakeGame, baseStyle tcell.Style) {
	if game == nil {
		return
	}

	boardX, boardY, boardWidth, boardHeight := boardBounds(screen)
	foodStyle := baseStyle.Foreground(tcell.ColorRed)
	bodyStyle := baseStyle.Foreground(tcell.ColorGreen)
	headStyle := bodyStyle.Bold(true)

	if game.Food.X >= 0 && game.Food.X < boardWidth && game.Food.Y >= 0 && game.Food.Y < boardHeight {
		screen.SetContent(boardX+game.Food.X, boardY+game.Food.Y, '*', nil, foodStyle)
	}
	for index := len(game.Snake) - 1; index >= 0; index-- {
		point := game.Snake[index]
		if point.X < 0 || point.X >= boardWidth || point.Y < 0 || point.Y >= boardHeight {
			continue
		}
		cell := 'o'
		style := bodyStyle
		if index == 0 {
			cell = '@'
			style = headStyle
		}
		screen.SetContent(boardX+point.X, boardY+point.Y, cell, nil, style)
	}
}

func directionFromKey(event *tcell.EventKey) (Direction, bool) {
	switch event.Key() {
	case tcell.KeyUp:
		return DirectionUp, true
	case tcell.KeyRight:
		return DirectionRight, true
	case tcell.KeyDown:
		return DirectionDown, true
	case tcell.KeyLeft:
		return DirectionLeft, true
	}
	if event.Key() != tcell.KeyRune {
		return DirectionRight, false
	}

	switch event.Rune() {
	case 'w', 'W':
		return DirectionUp, true
	case 'd', 'D':
		return DirectionRight, true
	case 's', 'S':
		return DirectionDown, true
	case 'a', 'A':
		return DirectionLeft, true
	default:
		return DirectionRight, false
	}
}

func scoreText(game *SnakeGame) string {
	if game == nil {
		return "0"
	}
	return fmt.Sprintf("%d", game.Score)
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
	_, height := screen.Size()
	if y < 0 || y >= height || maxWidth <= 0 {
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
