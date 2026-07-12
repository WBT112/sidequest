package game

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/WBT112/sidequest/internal/session"
)

const DefaultPollInterval = 250 * time.Millisecond
const DefaultGameInterval = 0
const movementPulseInterval = 10 * time.Millisecond
const focusPollInterval = 100 * time.Millisecond

type StateReader func() (session.State, error)
type FocusReader func() (bool, error)

type PauseState struct {
	Manual bool
	Focus  bool
}

func (p PauseState) Active() bool {
	return p.Manual || p.Focus
}

type PlayClock struct {
	Accumulated time.Duration
	ActiveSince time.Time
	Running     bool
}

func (c *PlayClock) Start(now time.Time) bool {
	if c.Running {
		return false
	}
	c.ActiveSince = now
	c.Running = true
	return true
}

func (c *PlayClock) Stop(now time.Time) bool {
	if !c.Running {
		return false
	}
	if now.After(c.ActiveSince) {
		c.Accumulated += now.Sub(c.ActiveSince)
	}
	c.Running = false
	c.ActiveSince = time.Time{}
	return true
}

func (c PlayClock) Elapsed(now time.Time) time.Duration {
	elapsed := c.Accumulated
	if c.Running && now.After(c.ActiveSince) {
		elapsed += now.Sub(c.ActiveSince)
	}
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

type Shell struct {
	NewScreen      func() (tcell.Screen, error)
	ReadState      StateReader
	ReadFocus      FocusReader
	OnQuitActive   func() error
	OnQuitTerminal func() error
	Random         RandomSource
	StatsManager   StatsManager
	PollInterval   time.Duration
	GameInterval   time.Duration
	Now            func() time.Time
}

type viewState struct {
	State          session.State
	SessionState   string
	Pause          PauseState
	Frozen         bool
	Message        string
	Started        bool
	Game           *SnakeGame
	CommandHeat    HeatLevel
	Heat           HeatLevel
	MaxHeat        int
	HeatNotice     string
	NoticeUntil    time.Time
	RoundStarted   time.Time
	RoundHeat      int
	RoundCatchUp   bool
	Clock          PlayClock
	GameEpoch      time.Time
	GameTime       time.Time
	NextFocusCheck time.Time
	Quest          *QuestState
	FinalScore     ScoreBreakdown
	StatsMessage   string
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
	gameIntervalOverride := s.GameInterval

	events := make(chan tcell.Event, 8)
	done := make(chan struct{})
	go pollEvents(screen, events, done)
	defer func() {
		close(done)
		screen.PostEvent(tcell.NewEventInterrupt(nil))
	}()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	movementPulse := time.NewTicker(movementPulseInterval)
	defer movementPulse.Stop()

	state, err := s.ReadState()
	if err != nil {
		return err
	}
	now := s.now()
	gameEpoch := now
	game := newSnakeGameForScreen(screen)
	mode := gameMode(state.GameMode)
	boardWidth, boardHeight := boardSize(screen)
	view := viewState{
		State:        state,
		SessionState: state.Status,
		Frozen:       terminalState(state.Status),
		Game:         game,
		RoundStarted: gameEpoch,
		RoundHeat:    1,
		GameEpoch:    gameEpoch,
		GameTime:     gameEpoch,
		Quest:        NewQuestState(mode, gameEpoch, s.Random, boardWidth, boardHeight),
	}
	s.syncFocus(&view, now, true)
	s.syncPlayClock(&view, now)
	updateViewGameTime(&view, now)
	updateViewHeat(&view, now)
	nextMove := now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
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
					now := s.now()
					wasPaused := view.Pause.Active()
					view.Pause.Manual = !view.Pause.Manual
					if view.Pause.Active() && !wasPaused {
						view.Clock.Stop(now)
					}
					if wasPaused && !view.Pause.Active() {
						view.Clock.Start(now)
						updateViewGameTime(&view, now)
						nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
					}
					updateViewGameTime(&view, now)
					updateViewHeat(&view, now)
					render(screen, view)
				case typed.Key() == tcell.KeyRune && (typed.Rune() == 'r' || typed.Rune() == 'R') && !view.Frozen && game.Over:
					now := s.now()
					updateViewGameTime(&view, now)
					game = newSnakeGameForScreen(screen)
					view.Game = game
					view.Started = false
					view.Pause.Manual = false
					view.Clock.Stop(now)
					view.RoundStarted = view.GameTime
					view.RoundHeat = RestartStartHeat(view.CommandHeat.Level)
					view.RoundCatchUp = view.RoundHeat < view.CommandHeat.Level
					if view.Quest.Enabled() {
						view.Quest.OnCrash()
					}
					updateViewHeat(&view, now)
					nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
					render(screen, view)
				default:
					if direction, ok := directionFromKey(typed); ok && !view.Frozen && !game.Over && !view.Pause.Focus {
						now := s.now()
						firstMove := !view.Started
						view.Started = true
						if firstMove {
							view.Clock.Start(now)
							updateViewGameTime(&view, now)
							nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
						}
						game.ChangeDirection(direction)
						render(screen, view)
					}
				}
			case *tcell.EventResize:
				screen.Sync()
				if !view.Frozen {
					game.Resize(boardSize(screen))
					view.Quest.ResizePickup(game)
				}
				render(screen, view)
			}
		case <-movementPulse.C:
			now := s.now()
			focusChanged, focusResumed := false, false
			if !now.Before(view.NextFocusCheck) {
				focusChanged, focusResumed = s.syncFocus(&view, now, true)
			}
			clockStarted, _ := s.syncPlayClock(&view, now)
			updateViewGameTime(&view, now)
			if focusResumed || clockStarted {
				nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
			}
			heatChanged := updateViewHeat(&view, now)
			if view.Quest.Enabled() {
				view.Quest.Tick(game, view.Heat, view.GameTime)
			}
			if view.Started && !view.Pause.Active() && !view.Frozen && !game.Over {
				if now.Before(nextMove) {
					if heatChanged || focusChanged {
						render(screen, view)
					}
					continue
				}
				stepFocusChanged, stepFocusResumed := s.syncFocus(&view, now, true)
				if stepFocusResumed {
					nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
				}
				if stepFocusChanged || view.Pause.Focus {
					render(screen, view)
					continue
				}
				game.FoodScore = view.Heat.ScoreAward(baseFoodScore)
				game.FoodHeat = view.Heat.Level
				stepGame(game, view.Quest, view.Heat, view.GameTime)
				if game.Over && view.Quest.Enabled() {
					view.Quest.OnCrash()
				}
				nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
				render(screen, view)
				continue
			}
			if heatChanged || focusChanged {
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
				now := s.now()
				view.Clock.Stop(now)
				updateViewGameTime(&view, now)
				freezeView(&view, view.GameTime, s.StatsManager)
			}
			updateViewHeat(&view, s.now())
			render(screen, view)
		}
	}
}

func stepGame(game *SnakeGame, quest *QuestState, heat HeatLevel, now time.Time) StepResult {
	if quest.Enabled() && quest.Pickup.Active && game.NextPoint() == quest.Pickup.Position {
		result := game.Step()
		if result == StepMoved {
			quest.OnPickupCollected(game, heat, now)
		}
		return result
	}
	if quest.Enabled() && quest.Golden.Active && game.NextPoint() == quest.Golden.Position {
		game.Food = quest.Golden.Position
		result := game.Step()
		if result == StepAteFood {
			quest.OnGoldenByte(game, heat, now)
		}
		return result
	}
	result := game.Step()
	if result == StepAteFood && quest.Enabled() {
		quest.OnNormalFood(game, heat, now)
	}
	if result == StepHitWall || result == StepHitSelf {
		return quest.TryCollisionEffects(game, result, now)
	}
	return result
}

func freezeView(view *viewState, now time.Time, statsManager StatsManager) {
	if view.Frozen {
		return
	}
	view.Frozen = true
	if view.Quest.Enabled() {
		view.FinalScore = view.Quest.Complete(view.Game, view.Heat, now)
		manager := statsManager
		if manager.BaseDir == "" {
			manager = DefaultStatsManager()
		}
		if _, err := manager.UpdateQuest(view.FinalScore, view.Quest); err != nil {
			view.StatsMessage = "Stats not saved: " + err.Error()
		}
	}
}

func gameMode(mode string) string {
	if mode == GameModeQuest {
		return GameModeQuest
	}
	return GameModeClassic
}

func (s Shell) syncFocus(view *viewState, now time.Time, force bool) (changed bool, resumed bool) {
	if !force && now.Before(view.NextFocusCheck) {
		return false, false
	}
	view.NextFocusCheck = now.Add(focusPollInterval)
	if s.ReadFocus == nil || view.Frozen {
		if view.Pause.Focus {
			view.Pause.Focus = false
			return true, true
		}
		return false, false
	}
	active, err := s.ReadFocus()
	nextFocusPause := err != nil || !active
	if view.Pause.Focus == nextFocusPause {
		return false, false
	}
	view.Pause.Focus = nextFocusPause
	if nextFocusPause && view.Game != nil {
		view.Game.ClearPendingDirections()
	}
	return true, !nextFocusPause
}

func (s Shell) syncPlayClock(view *viewState, now time.Time) (started bool, stopped bool) {
	shouldRun := view.Started &&
		!view.Pause.Active() &&
		!view.Frozen &&
		view.Game != nil &&
		!view.Game.Over &&
		!session.IsTerminalStatus(view.SessionState)
	if shouldRun {
		return view.Clock.Start(now), false
	}
	return false, view.Clock.Stop(now)
}

func updateViewGameTime(view *viewState, now time.Time) {
	if view.GameEpoch.IsZero() {
		view.GameEpoch = now
	}
	view.GameTime = view.GameEpoch.Add(view.Clock.Elapsed(now))
}

func (s Shell) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
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
	if view.Pause.Active() {
		controlLine = pauseLine(view.Pause)
	}
	if view.Frozen {
		controlLine = "Command finished. Q exit/cleanup  F10 detach/list"
	}
	if view.Game != nil && view.Game.Over && !view.Frozen {
		controlLine = "Round over. R restart  Q exit/cleanup  F10 detach/list"
	}
	if view.Quest.Enabled() {
		controlLine = questLine(view)
	}
	if view.HeatNotice != "" {
		controlLine = view.HeatNotice
	}

	lines := []renderLine{
		{0, "Sidequest Snake [" + gameModeLabel(view) + "]", titleStyle},
		{1, "Command state: " + displayState(view.SessionState), statusStyle},
		{2, heatScoreLine(view), scoreStyle},
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
	drawGoldenByte(screen, view.Quest, style)
	drawPickup(screen, view.Quest, style)
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
	arena := arenaForScreen(screen)
	return arena.Width, arena.Height
}

func boardBounds(screen tcell.Screen) (int, int, int, int) {
	arena := arenaForScreen(screen)
	return arena.X, arena.Y, arena.Width, arena.Height
}

func arenaForScreen(screen tcell.Screen) Arena {
	width, height := screen.Size()
	return ArenaForScreen(width, height)
}

func drawPlayfield(screen tcell.Screen, style tcell.Style) {
	width, height := screen.Size()
	arena := arenaForScreen(screen)
	topWallY := arena.Y - 1
	bottomWallY := arena.Y + arena.Height
	leftWallX := arena.X - 1
	rightWallX := arena.X + arena.RenderWidth()
	if width < 2 || height < 8 || topWallY <= 0 || bottomWallY >= height {
		return
	}

	boardStyle := style.Background(tcell.ColorDarkSlateGray)
	wallStyle := style.Foreground(tcell.ColorTeal).Background(tcell.ColorTeal).Bold(true)

	for y := arena.Y; y < arena.Y+arena.Height; y++ {
		for x := arena.X; x < arena.X+arena.RenderWidth(); x++ {
			screen.SetContent(x, y, ' ', nil, boardStyle)
		}
	}
	for x := leftWallX; x <= rightWallX; x++ {
		screen.SetContent(x, topWallY, tcell.RuneBlock, nil, wallStyle)
		screen.SetContent(x, bottomWallY, tcell.RuneBlock, nil, wallStyle)
	}
	for y := arena.Y; y < bottomWallY; y++ {
		screen.SetContent(leftWallX, y, tcell.RuneBlock, nil, wallStyle)
		screen.SetContent(rightWallX, y, tcell.RuneBlock, nil, wallStyle)
	}
}

func drawSnake(screen tcell.Screen, game *SnakeGame, baseStyle tcell.Style) {
	if game == nil {
		return
	}

	arena := arenaForScreen(screen)
	boardBackground := tcell.ColorDarkSlateGray
	foodStyle := baseStyle.Foreground(tcell.ColorYellow).Background(boardBackground).Bold(true)
	bodyStyle := baseStyle.Foreground(tcell.ColorLimeGreen).Background(boardBackground)
	headStyle := bodyStyle.Bold(true)

	if game.Food.X >= 0 && game.Food.X < arena.Width && game.Food.Y >= 0 && game.Food.Y < arena.Height {
		drawCell(screen, arena, game.Food, "()", foodStyle)
	}
	for index := len(game.Snake) - 1; index >= 0; index-- {
		point := game.Snake[index]
		if point.X < 0 || point.X >= arena.Width || point.Y < 0 || point.Y >= arena.Height {
			continue
		}
		cell := "▓▓"
		style := bodyStyle
		if index == 0 {
			cell = "██"
			style = headStyle
		}
		drawCell(screen, arena, point, cell, style)
	}
}

func drawGoldenByte(screen tcell.Screen, quest *QuestState, baseStyle tcell.Style) {
	if !quest.Enabled() || !quest.Golden.Active {
		return
	}
	arena := arenaForScreen(screen)
	point := quest.Golden.Position
	if point.X < 0 || point.X >= arena.Width || point.Y < 0 || point.Y >= arena.Height {
		return
	}
	style := baseStyle.Foreground(tcell.ColorOrange).Background(tcell.ColorDarkSlateGray).Bold(true)
	drawCell(screen, arena, point, "<>", style)
}

func drawPickup(screen tcell.Screen, quest *QuestState, baseStyle tcell.Style) {
	if !quest.Enabled() || !quest.Pickup.Active {
		return
	}
	arena := arenaForScreen(screen)
	point := quest.Pickup.Position
	if point.X < 0 || point.X >= arena.Width || point.Y < 0 || point.Y >= arena.Height {
		return
	}
	style := pickupStyle(baseStyle, quest.Pickup.Upgrade)
	drawCell(screen, arena, point, PickupSymbol(quest.Pickup.Upgrade), style)
}

func pickupStyle(baseStyle tcell.Style, upgrade Upgrade) tcell.Style {
	background := tcell.ColorDarkSlateGray
	switch upgrade {
	case UpgradeShield:
		return baseStyle.Foreground(tcell.ColorAqua).Background(background).Bold(true)
	case UpgradePhase:
		return baseStyle.Foreground(tcell.ColorFuchsia).Background(background).Bold(true)
	case UpgradeSlowClock:
		return baseStyle.Foreground(tcell.ColorBlue).Background(background).Bold(true)
	case UpgradeDoubleScore:
		return baseStyle.Foreground(tcell.ColorLimeGreen).Background(background).Bold(true)
	case UpgradePatch:
		return baseStyle.Foreground(tcell.ColorWhite).Background(background).Bold(true)
	case UpgradeComboKeeper:
		return baseStyle.Foreground(tcell.ColorYellow).Background(background).Bold(true)
	case UpgradeTurbo:
		return baseStyle.Foreground(tcell.ColorAqua).Background(background).Bold(true)
	case UpgradeWarp:
		return baseStyle.Foreground(tcell.ColorPurple).Background(background).Bold(true)
	default:
		return baseStyle.Foreground(tcell.ColorWhite).Background(background).Bold(true)
	}
}

func drawCell(screen tcell.Screen, arena Arena, point Point, text string, style tcell.Style) {
	x := arena.CellX(point.X)
	y := arena.CellY(point.Y)
	runes := []rune(text)
	for index := 0; index < arenaCellWidth; index++ {
		cell := ' '
		if index < len(runes) {
			cell = runes[index]
		}
		screen.SetContent(x+index, y, cell, nil, style)
	}
}

func drawResultPanel(screen tcell.Screen, view viewState, baseStyle tcell.Style) {
	if view.Game == nil || (!view.Game.Over && !view.Frozen) {
		return
	}

	arena := arenaForScreen(screen)
	if arena.RenderWidth() < 16 || arena.Height < 5 {
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
		"Max heat: " + fmt.Sprintf("%d", view.MaxHeat),
		action,
	}
	if view.Quest.Enabled() && view.FinalScore.FinalScore > 0 {
		lines = []string{
			title,
			fmt.Sprintf("Final score: %d", view.FinalScore.FinalScore),
			fmt.Sprintf("Mission %+d  Alive %+d", view.FinalScore.MissionBonus, view.FinalScore.SurvivalBonus),
			fmt.Sprintf("Combo %+d  Heat %+d", view.FinalScore.MaxComboBonus, view.FinalScore.MaxHeatBonus),
		}
	}

	panelWidth := maxTextWidth(lines) + 4
	if panelWidth < 24 {
		panelWidth = 24
	}
	if panelWidth > arena.RenderWidth() {
		panelWidth = arena.RenderWidth()
	}
	panelHeight := 8
	if arena.Height < panelHeight {
		panelHeight = 6
	}
	panelX := arena.X + (arena.RenderWidth()-panelWidth)/2
	panelY := arena.Y + (arena.Height-panelHeight)/2

	panelStyle := baseStyle.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	borderStyle := baseStyle.Foreground(accent).Background(tcell.ColorBlack).Bold(true)
	fillRect(screen, panelX, panelY, panelWidth, panelHeight, ' ', panelStyle)
	drawBox(screen, panelX, panelY, panelWidth, panelHeight, borderStyle)

	lineYs := []int{panelY + 1, panelY + 2, panelY + panelHeight - 3, panelY + panelHeight - 2}
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

func updateViewHeat(view *viewState, now time.Time) bool {
	previousCommandHeat := view.CommandHeat.Level
	previousHeat := view.Heat.Level
	previousNotice := view.HeatNotice

	elapsed := view.Clock.Elapsed(now)
	commandHeat := HeatForElapsed(elapsed)
	view.CommandHeat = commandHeat
	if view.MaxHeat < commandHeat.Level {
		view.MaxHeat = commandHeat.Level
	}

	activeLevel := commandHeat.Level
	if view.RoundCatchUp {
		activeLevel = RestartRampHeat(commandHeat.Level, view.RoundHeat, view.GameTime.Sub(view.RoundStarted))
	}
	view.Heat = HeatByLevel(activeLevel)

	view.HeatNotice = ""
	if !view.Frozen {
		if previousCommandHeat > 0 && commandHeat.Level > previousCommandHeat {
			view.HeatNotice = heatTransitionText(commandHeat)
			view.NoticeUntil = now.Add(3 * time.Second)
		}
		if view.HeatNotice == "" && now.Before(view.NoticeUntil) {
			view.HeatNotice = heatTransitionText(commandHeat)
		}
		if view.HeatNotice == "" {
			if next, remaining, ok := UpcomingHeat(elapsed); ok && remaining <= heatWarningWindow {
				view.HeatNotice = heatTransitionText(next)
			}
		}
	}

	return previousCommandHeat != view.CommandHeat.Level ||
		previousHeat != view.Heat.Level ||
		previousNotice != view.HeatNotice
}

func activeMoveInterval(view viewState, override time.Duration, now time.Time) time.Duration {
	if override > 0 {
		return override
	}
	if view.Heat.MovementInterval <= 0 {
		return HeatByLevel(1).MovementInterval
	}
	interval := view.Heat.MovementInterval
	if view.Quest.Enabled() {
		interval = view.Quest.EffectiveInterval(interval, now)
	}
	return interval
}

func pauseLine(pause PauseState) string {
	switch {
	case pause.Manual && pause.Focus:
		return "PAUSED - MANUAL + COMMAND FOCUS"
	case pause.Focus:
		return "PAUSED - COMMAND PANE ACTIVE"
	case pause.Manual:
		return "PAUSED - PRESS P TO RESUME"
	default:
		return ""
	}
}

func heatTransitionText(heat HeatLevel) string {
	return fmt.Sprintf("COMMAND HEAT RISING... SPEED %d  SCORE %s", heat.Level, heat.MultiplierText())
}

func heatScoreLine(view viewState) string {
	heat := view.Heat
	if heat.Level == 0 {
		heat = HeatByLevel(1)
	}
	if view.Quest.Enabled() {
		effects := view.Quest.effectHUDParts(view.GameTime)
		suffix := ""
		if len(effects) > 0 {
			suffix = "  " + joinParts(effects)
		}
		return fmt.Sprintf(
			"SCORE %s  COMBO x%d  HEAT %d %s%s",
			scoreText(view.Game),
			view.Quest.Combo,
			heat.Level,
			heat.MultiplierText(),
			suffix,
		)
	}
	return fmt.Sprintf(
		"MODE classic  Score: %s  Heat: %d/%d  Score %s",
		scoreText(view.Game),
		heat.Level,
		MaxHeatLevel(),
		heat.MultiplierText(),
	)
}

func gameModeLabel(view viewState) string {
	if view.Quest.Enabled() {
		return GameModeQuest
	}
	return GameModeClassic
}

func questLine(view viewState) string {
	if !view.Quest.Enabled() {
		return ""
	}
	quest := view.Quest
	if quest.Message != "" {
		return quest.Message
	}
	if quest.Mission.ID == "" {
		return "QUEST: none"
	}
	return fmt.Sprintf("QUEST: %s %d/%d", quest.Mission.Label, quest.MissionProgress, quest.Mission.Target)
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
