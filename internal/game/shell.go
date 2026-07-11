package game

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/WBT112/sidequest/internal/session"
)

const DefaultPollInterval = 250 * time.Millisecond
const DefaultGameInterval = 250 * time.Millisecond

type StateReader func() (session.State, error)

type Shell struct {
	NewScreen      func() (tcell.Screen, error)
	ReadState      StateReader
	OnQuitActive   func() error
	OnQuitTerminal func() error
	PollInterval   time.Duration
	GameInterval   time.Duration
}

type viewState struct {
	State        session.State
	SessionState string
	Paused       bool
	Frozen       bool
	Message      string
	Started      bool
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
	view := viewState{State: state, SessionState: state.Status, Frozen: terminalState(state.Status), Game: game}
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
					if session.IsTerminalStatus(view.SessionState) && s.OnQuitTerminal != nil {
						return s.OnQuitTerminal()
					}
					if !session.IsTerminalStatus(view.SessionState) && s.OnQuitActive != nil {
						return s.OnQuitActive()
					}
					return nil
				case typed.Key() == tcell.KeyRune && (typed.Rune() == 'p' || typed.Rune() == 'P'):
					view.Paused = !view.Paused
					render(screen, view)
				case typed.Key() == tcell.KeyRune && (typed.Rune() == 'r' || typed.Rune() == 'R') && !view.Frozen && game.Over:
					game = newSnakeGameForScreen(screen)
					view.Game = game
					view.Started = false
					view.Paused = false
					render(screen, view)
				default:
					if direction, ok := directionFromKey(typed); ok && !view.Frozen && !game.Over {
						view.Started = true
						game.ChangeDirection(direction)
						render(screen, view)
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
			if view.Started && !view.Paused && !view.Frozen && !game.Over {
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

			view.State = state
			view.SessionState = state.Status
			if session.IsTerminalStatus(state.Status) {
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
	drawPlayfield(screen, style)

	controlLine := "Arrows/WASD start. F12 command. F10 detach/list."
	if view.Started {
		controlLine = "Arrows/WASD move  P pause/resume  Q exit/cleanup  F10 detach/list"
	}
	if view.Paused {
		controlLine = "Paused  P resume  Q exit/cleanup  F10 detach/list"
	}
	if view.Frozen {
		controlLine = "Command finished. Q exit/cleanup  F10 detach/list"
	}
	if view.Game != nil && view.Game.Over && !view.Frozen {
		controlLine = "Round over. R restart  Q exit/cleanup  F10 detach/list"
	}

	lines := []renderLine{
		{0, "Sidequest Snake", titleStyle},
		{1, "Command state: " + displayState(view.SessionState), statusStyle},
		{2, "Score: " + scoreText(view.Game), scoreStyle},
		{3, controlLine, secondaryStyle},
	}
	if session.IsTerminalStatus(view.SessionState) {
		lines = append(lines, renderLine{height - 2, resultSummary(view.State), secondaryStyle})
	}
	if view.Message != "" {
		y := height - 2
		if session.IsTerminalStatus(view.SessionState) {
			y = height - 3
		}
		lines = append(lines, renderLine{y, view.Message, secondaryStyle})
	}

	drawSnake(screen, view.Game, style)
	drawResultPanel(screen, view, style)

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
	y := 5
	width := screenWidth - 2
	height := screenHeight - y - 1
	if screenHeight < 8 {
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

func drawPlayfield(screen tcell.Screen, style tcell.Style) {
	width, height := screen.Size()
	boardX, boardY, boardWidth, boardHeight := boardBounds(screen)
	topWallY := boardY - 1
	bottomWallY := boardY + boardHeight
	if width < 2 || height < 8 || topWallY <= 0 || bottomWallY >= height {
		return
	}

	boardStyle := style.Background(tcell.ColorDarkSlateGray)
	wallStyle := style.Foreground(tcell.ColorTeal).Background(tcell.ColorTeal).Bold(true)

	for y := boardY; y < boardY+boardHeight; y++ {
		for x := boardX; x < boardX+boardWidth; x++ {
			screen.SetContent(x, y, ' ', nil, boardStyle)
		}
	}
	for x := 0; x < width; x++ {
		screen.SetContent(x, topWallY, tcell.RuneBlock, nil, wallStyle)
		screen.SetContent(x, bottomWallY, tcell.RuneBlock, nil, wallStyle)
	}
	for y := boardY; y < bottomWallY; y++ {
		screen.SetContent(0, y, tcell.RuneBlock, nil, wallStyle)
		screen.SetContent(width-1, y, tcell.RuneBlock, nil, wallStyle)
	}
}

func drawSnake(screen tcell.Screen, game *SnakeGame, baseStyle tcell.Style) {
	if game == nil {
		return
	}

	boardX, boardY, boardWidth, boardHeight := boardBounds(screen)
	boardBackground := tcell.ColorDarkSlateGray
	foodStyle := baseStyle.Foreground(tcell.ColorYellow).Background(boardBackground).Bold(true)
	bodyStyle := baseStyle.Foreground(tcell.ColorLimeGreen).Background(boardBackground)
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

func drawResultPanel(screen tcell.Screen, view viewState, baseStyle tcell.Style) {
	if view.Game == nil || (!view.Game.Over && !view.Frozen) {
		return
	}

	boardX, boardY, boardWidth, boardHeight := boardBounds(screen)
	if boardWidth < 16 || boardHeight < 5 {
		return
	}

	title := "GAME OVER"
	action := "R restart  Q exit"
	accent := tcell.ColorOrange
	if view.Frozen {
		title = "RUN FINISHED"
		action = "Q exit/cleanup"
		accent = tcell.ColorAqua
	}
	lines := []string{
		title,
		"Final score: " + scoreText(view.Game),
		action,
	}

	panelWidth := maxTextWidth(lines) + 4
	if panelWidth < 24 {
		panelWidth = 24
	}
	if panelWidth > boardWidth {
		panelWidth = boardWidth
	}
	panelHeight := 7
	if boardHeight < panelHeight {
		panelHeight = 5
	}
	panelX := boardX + (boardWidth-panelWidth)/2
	panelY := boardY + (boardHeight-panelHeight)/2

	panelStyle := baseStyle.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	borderStyle := baseStyle.Foreground(accent).Background(tcell.ColorBlack).Bold(true)
	fillRect(screen, panelX, panelY, panelWidth, panelHeight, ' ', panelStyle)
	drawBox(screen, panelX, panelY, panelWidth, panelHeight, borderStyle)

	lineYs := []int{panelY + 1, panelY + panelHeight/2, panelY + panelHeight - 2}
	for index, line := range lines {
		lineStyle := panelStyle
		if index == 0 {
			lineStyle = borderStyle
		}
		drawCenteredText(screen, panelX+1, lineYs[index], panelWidth-2, line, lineStyle)
	}
}

func fillRect(screen tcell.Screen, x int, y int, width int, height int, char rune, style tcell.Style) {
	for row := y; row < y+height; row++ {
		for col := x; col < x+width; col++ {
			screen.SetContent(col, row, char, nil, style)
		}
	}
}

func drawCenteredText(screen tcell.Screen, x int, y int, width int, text string, style tcell.Style) {
	textWidth := textDisplayWidth(text)
	if textWidth > width {
		drawText(screen, x, y, width, text, style)
		return
	}
	drawText(screen, x+(width-textWidth)/2, y, textWidth, text, style)
}

func maxTextWidth(lines []string) int {
	maximum := 0
	for _, line := range lines {
		if width := textDisplayWidth(line); width > maximum {
			maximum = width
		}
	}
	return maximum
}

func textDisplayWidth(text string) int {
	return len([]rune(text))
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

func resultSummary(state session.State) string {
	parts := []string{"Runtime: " + durationText(state.DurationMillis)}
	if state.ExitCode != nil {
		parts = append([]string{fmt.Sprintf("Exit code: %d", *state.ExitCode)}, parts...)
	}
	if state.ExitSignal != "" {
		parts = append([]string{"Signal: " + state.ExitSignal}, parts...)
	}
	if state.StartError != "" {
		parts = append([]string{"Start error: " + state.StartError}, parts...)
	}
	return joinParts(parts)
}

func durationText(durationMillis *int64) string {
	if durationMillis == nil {
		return "-"
	}
	duration := time.Duration(*durationMillis) * time.Millisecond
	if duration < 0 {
		duration = 0
	}
	total := int64(duration.Round(time.Second).Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += "  " + part
	}
	return result
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
	return session.IsTerminalStatus(state)
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
