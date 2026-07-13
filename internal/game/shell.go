package game

import (
	"context"
	"fmt"
	"strings"
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

type CommandCompletionChoice int

const (
	CompletionNone CommandCompletionChoice = iota
	CompletionUndecided
	CompletionContinue
	CompletionQuit
)

type PauseState struct {
	Manual     bool
	Focus      bool
	Resize     bool
	Completion bool
}

func (p PauseState) Active() bool {
	return p.Manual || p.Focus || p.Resize || p.Completion
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
	OnQuitTerminal func() error
	Random         RandomSource
	StatsManager   StatsManager
	PollInterval   time.Duration
	GameInterval   time.Duration
	Now            func() time.Time
}

type viewState struct {
	State           session.State
	SessionState    string
	Pause           PauseState
	Frozen          bool
	Message         string
	Started         bool
	NoColor         bool
	Game            *SnakeGame
	CommandHeat     HeatLevel
	FrozenHeat      HeatLevel
	HeatFrozen      bool
	Heat            HeatLevel
	MaxHeat         int
	HeatNotice      string
	NoticeUntil     time.Time
	RoundStarted    time.Time
	RoundHeat       int
	RoundCatchUp    bool
	Clock           PlayClock
	GameEpoch       time.Time
	GameTime        time.Time
	NextFocusCheck  time.Time
	Quest           *QuestState
	FinalScore      ScoreBreakdown
	RoundFinalized  bool
	QuestStatsSaved bool
	ResultScore     int
	Leaderboard     []LeaderboardEntry
	CurrentRank     int
	PendingScore    *PendingHighscore
	StatsMessage    string
	Completion      CommandCompletionChoice
}

type PendingHighscore struct {
	Score         int
	Mode          string
	Input         string
	DefaultInput  string
	ReplaceOnType bool
	CurrentRank   int
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
		NoColor:      state.NoColor,
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
				if view.PendingScore != nil {
					quit, handled := s.handlePendingHighscoreKey(&view, typed)
					render(screen, view)
					if quit {
						return s.quit(view)
					}
					if handled {
						continue
					}
				}
				if view.Completion == CompletionUndecided {
					now := s.now()
					switch {
					case typed.Key() == tcell.KeyRune && (typed.Rune() == 'c' || typed.Rune() == 'C'):
						view.Completion = CompletionContinue
						view.Pause.Completion = false
						game.ClearPendingDirections()
						updateViewGameTime(&view, now)
						updateViewHeat(&view, now)
						if view.Started && !view.Pause.Active() && !game.Over {
							view.Clock.Start(now)
						}
						nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
						render(screen, view)
						continue
					case typed.Key() == tcell.KeyRune && (typed.Rune() == 'q' || typed.Rune() == 'Q'):
						view.Completion = CompletionQuit
						view.Pause.Completion = false
						finalizeCompletionQuit(&view, s.statsManager())
						render(screen, view)
						if view.PendingScore == nil {
							return s.quit(view)
						}
						continue
					default:
						render(screen, view)
						continue
					}
				}
				switch {
				case typed.Key() == tcell.KeyRune && (typed.Rune() == 'q' || typed.Rune() == 'Q') && session.IsTerminalStatus(view.SessionState):
					return s.quit(view)
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
					if view.RoundFinalized && !view.QuestStatsSaved {
						updateQuestStats(&view, s.statsManager())
					}
					statsMessage := ""
					if !view.QuestStatsSaved {
						statsMessage = view.StatsMessage
					}
					game = newSnakeGameForScreen(screen)
					view.Game = game
					view.Started = false
					view.Pause.Manual = false
					view.Clock.Stop(now)
					view.RoundStarted = view.GameTime
					view.RoundHeat = RestartStartHeat(view.CommandHeat.Level)
					view.RoundCatchUp = view.RoundHeat < view.CommandHeat.Level
					view.RoundFinalized = false
					view.QuestStatsSaved = false
					view.ResultScore = 0
					view.CurrentRank = 0
					view.PendingScore = nil
					view.Leaderboard = nil
					view.StatsMessage = ""
					view.Message = statsMessage
					boardWidth, boardHeight := boardSize(screen)
					view.Quest = NewQuestState(mode, view.GameTime, s.Random, boardWidth, boardHeight)
					updateViewHeat(&view, now)
					nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
					render(screen, view)
				default:
					if direction, ok := directionFromKey(typed); ok && !view.Frozen && !game.Over && !view.Pause.Active() {
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
				now := s.now()
				wasPaused := view.Pause.Active()
				result := game.Resize(boardSize(screen))
				if result == ResizeTooSmall {
					view.Pause.Resize = true
					view.Clock.Stop(now)
					updateViewGameTime(&view, now)
					updateViewHeat(&view, now)
				} else {
					view.Pause.Resize = false
					view.Quest.ResizeObjects(game)
					updateViewGameTime(&view, now)
					updateViewHeat(&view, now)
					if wasPaused && !view.Pause.Active() && canRunPlayClock(view) {
						view.Clock.Start(now)
					}
					nextMove = now.Add(activeMoveInterval(view, gameIntervalOverride, view.GameTime))
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
				if game.Over {
					finalizeRound(&view, s.statsManager())
					updateQuestStats(&view, s.statsManager())
					if session.IsTerminalStatus(view.SessionState) {
						view.Frozen = true
					}
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

			state = mergeTerminalState(view.State, state)
			view.State = state
			view.SessionState = state.Status
			view.NoColor = state.NoColor
			if session.IsTerminalStatus(state.Status) {
				now := s.now()
				if view.Completion != CompletionContinue {
					view.Clock.Stop(now)
				}
				updateViewGameTime(&view, now)
				updateViewHeat(&view, now)
				freezeCommandHeat(&view)
				if shouldOfferCompletionDecision(&view) {
					view.Completion = CompletionUndecided
					view.Pause.Completion = true
					if view.Game != nil {
						view.Game.ClearPendingDirections()
					}
					render(screen, view)
					continue
				}
				if view.Completion == CompletionNone {
					freezeView(&view, view.GameTime, s.StatsManager)
				}
			}
			updateViewHeat(&view, s.now())
			render(screen, view)
		}
	}
}

func stepGame(game *SnakeGame, quest *QuestState, heat HeatLevel, now time.Time) StepResult {
	if quest.Enabled() && quest.Pickup.Active && game.NextPoint() == quest.Pickup.Position {
		overlappedFood := game.Food == quest.Pickup.Position
		originalFood := game.Food
		if overlappedFood {
			game.Food = Point{X: -1, Y: -1}
		}
		result := game.Step()
		if overlappedFood {
			game.Food = originalFood
		}
		if result == StepMoved {
			quest.OnPickupCollected(game, heat, now)
			quest.EnsureFood(game)
		}
		return result
	}
	if quest.Enabled() && quest.Golden.Active && game.NextPoint() == quest.Golden.Position {
		result := game.StepGrow()
		if result == StepAteFood {
			quest.OnGoldenByte(game, heat, now)
			quest.EnsureFood(game)
		}
		return result
	}
	result := game.Step()
	if result == StepMoved && quest.Enabled() {
		quest.EnsureFood(game)
	}
	if result == StepAteFood && quest.Enabled() {
		quest.OnNormalFood(game, heat, now)
		quest.EnsureFood(game)
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
	if shouldFinalizeTerminalRound(view) {
		finalizeRound(view, statsManagerOrDefault(statsManager))
	} else if !view.RoundFinalized {
		refreshLeaderboard(view, statsManagerOrDefault(statsManager))
	}
	updateQuestStats(view, statsManagerOrDefault(statsManager))
}

func mergeTerminalState(previous session.State, next session.State) session.State {
	if !session.IsTerminalStatus(previous.Status) || !session.IsTerminalStatus(next.Status) {
		return next
	}
	if next.DurationMillis == nil {
		next.DurationMillis = previous.DurationMillis
	}
	if next.ExitCode == nil {
		next.ExitCode = previous.ExitCode
	}
	if next.ExitSignal == "" {
		next.ExitSignal = previous.ExitSignal
	}
	if next.StartError == "" {
		next.StartError = previous.StartError
	}
	if next.StartedAt == nil {
		next.StartedAt = previous.StartedAt
	}
	if next.FinishedAt == nil {
		next.FinishedAt = previous.FinishedAt
	}
	return next
}

func shouldFinalizeTerminalRound(view *viewState) bool {
	if !view.Started || view.Game == nil || view.Game.Over {
		return false
	}
	if view.SessionState == session.StatusCompleted {
		return true
	}
	return currentRoundScore(view) > 0
}

func shouldOfferCompletionDecision(view *viewState) bool {
	return view != nil &&
		session.IsTerminalStatus(view.SessionState) &&
		view.Completion == CompletionNone &&
		!view.Frozen &&
		!view.RoundFinalized &&
		view.PendingScore == nil &&
		view.Game != nil &&
		!view.Game.Over
}

func finalizeCompletionQuit(view *viewState, manager StatsManager) {
	view.Frozen = true
	if shouldFinalizeCompletionQuit(view) {
		finalizeRound(view, manager)
	} else if !view.RoundFinalized {
		refreshLeaderboard(view, manager)
	}
	updateQuestStats(view, manager)
}

func shouldFinalizeCompletionQuit(view *viewState) bool {
	if view == nil || !view.Started || view.Game == nil || view.Game.Over {
		return false
	}
	if view.SessionState == session.StatusCompleted {
		return true
	}
	return currentRoundScore(view) > 0
}

func updateQuestStats(view *viewState, manager StatsManager) {
	if view.Quest.Enabled() && view.RoundFinalized && !view.QuestStatsSaved {
		if _, err := manager.UpdateQuest(view.FinalScore, view.Quest); err != nil {
			view.StatsMessage = "Stats not saved: " + err.Error()
			return
		}
		view.QuestStatsSaved = true
	}
}

func freezeCommandHeat(view *viewState) {
	if view == nil || view.HeatFrozen {
		return
	}
	if view.CommandHeat.Level == 0 {
		view.CommandHeat = HeatByLevel(1)
	}
	view.FrozenHeat = view.CommandHeat
	view.HeatFrozen = true
}

func (s Shell) statsManager() StatsManager {
	return statsManagerOrDefault(s.StatsManager)
}

func statsManagerOrDefault(manager StatsManager) StatsManager {
	if manager.BaseDir == "" {
		manager.BaseDir = DefaultStatsManager().BaseDir
	}
	return manager
}

func (s Shell) quit(view viewState) error {
	if session.IsTerminalStatus(view.SessionState) && s.OnQuitTerminal != nil {
		return s.OnQuitTerminal()
	}
	return nil
}

func finalizeRound(view *viewState, manager StatsManager) {
	if view.RoundFinalized || view.Game == nil || !view.Started {
		refreshLeaderboard(view, manager)
		return
	}
	score := finalScore(view)
	view.ResultScore = score
	view.RoundFinalized = true
	refreshLeaderboard(view, manager)
	rank := manager.QualifyingRank(gameModeLabel(*view), score)
	view.CurrentRank = rank
	if rank == 0 {
		return
	}
	defaultName := manager.DefaultPlayerName()
	view.PendingScore = &PendingHighscore{
		Score:         score,
		Mode:          gameModeLabel(*view),
		Input:         defaultName,
		DefaultInput:  defaultName,
		ReplaceOnType: true,
		CurrentRank:   rank,
	}
}

func finalScore(view *viewState) int {
	if view.Quest.Enabled() {
		if !view.Quest.Completed {
			view.FinalScore = view.Quest.Complete(view.Game, view.Heat, view.GameTime)
		} else {
			view.FinalScore = view.Quest.Final
		}
		return view.FinalScore.FinalScore
	}
	if view.Game == nil {
		return 0
	}
	return view.Game.Score
}

func currentRoundScore(view *viewState) int {
	if view == nil || view.Game == nil {
		return 0
	}
	return view.Game.Score
}

func refreshLeaderboard(view *viewState, manager StatsManager) {
	view.Leaderboard = manager.Leaderboard(gameModeLabel(*view))
}

func (s Shell) handlePendingHighscoreKey(view *viewState, event *tcell.EventKey) (bool, bool) {
	pending := view.PendingScore
	if pending == nil {
		return false, false
	}
	switch event.Key() {
	case tcell.KeyEnter:
		confirmPendingHighscore(view, s.statsManager())
		return false, true
	case tcell.KeyEscape:
		if session.IsTerminalStatus(view.SessionState) {
			confirmPendingHighscore(view, s.statsManager())
			return true, true
		}
		return false, true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if pending.ReplaceOnType {
			pending.Input = ""
			pending.ReplaceOnType = false
			return false, true
		}
		runes := []rune(pending.Input)
		if len(runes) > 0 {
			pending.Input = string(runes[:len(runes)-1])
		}
		return false, true
	case tcell.KeyDelete:
		pending.Input = ""
		pending.ReplaceOnType = false
		return false, true
	case tcell.KeyCtrlU:
		pending.Input = ""
		pending.ReplaceOnType = false
		return false, true
	case tcell.KeyRune:
		r := event.Rune()
		if r == 'q' || r == 'Q' {
			if session.IsTerminalStatus(view.SessionState) {
				confirmPendingHighscore(view, s.statsManager())
				return true, true
			}
			return false, true
		}
		if r == 'r' || r == 'R' || r == 'p' || r == 'P' {
			return false, true
		}
		if r < ' ' || r == '\u007f' {
			return false, true
		}
		if pending.ReplaceOnType {
			pending.Input = ""
			pending.ReplaceOnType = false
		}
		pending.Input = NormalizePlayerName(pending.Input + string(r))
		return false, true
	default:
		return false, true
	}
}

func confirmPendingHighscore(view *viewState, manager StatsManager) {
	pending := view.PendingScore
	if pending == nil {
		return
	}
	stats, rank, err := manager.AddLeaderboardScore(pending.Mode, pending.Score, pending.Input)
	if err != nil {
		view.StatsMessage = "Highscore not saved: " + err.Error()
		view.Leaderboard = manager.Leaderboard(pending.Mode)
		view.CurrentRank = 0
		view.PendingScore = nil
		return
	}
	view.Leaderboard = top5ForMode(stats, pending.Mode)
	view.CurrentRank = rank
	view.PendingScore = nil
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
	if canRunPlayClock(*view) {
		return view.Clock.Start(now), false
	}
	return false, view.Clock.Stop(now)
}

func canRunPlayClock(view viewState) bool {
	return view.Started &&
		!view.Pause.Active() &&
		!view.Frozen &&
		view.Game != nil &&
		!view.Game.Over &&
		(!session.IsTerminalStatus(view.SessionState) || view.Completion == CompletionContinue)
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

	style := baseRenderStyle(view.NoColor)
	titleStyle := colorStyle(style.Bold(true), view.NoColor, tcell.ColorAqua, tcell.ColorDefault)
	statusStyle := colorStyle(style, view.NoColor, statusColor(view.SessionState), tcell.ColorDefault)
	secondaryStyle := colorStyle(style, view.NoColor, tcell.ColorGray, tcell.ColorDefault)
	scoreStyle := colorStyle(style, view.NoColor, tcell.ColorGreen, tcell.ColorDefault)

	drawBox(screen, 0, 0, width, height, style)
	drawPlayfield(screen, style, view.NoColor)

	lines := []renderLine{
		{y: 0, text: "Sidequest Snake [" + gameModeLabel(view) + "]", style: titleStyle, centered: true},
		{y: 1, text: "Command state: " + displayState(view.SessionState), style: statusStyle, centered: true},
		{y: 2, text: heatScoreLine(view), style: scoreStyle, centered: true},
		{y: 3, text: statusLine(view), style: secondaryStyle, centered: true},
	}
	if session.IsTerminalStatus(view.SessionState) {
		lines = append(lines, renderLine{y: height - 2, text: resultSummary(view.State), style: secondaryStyle})
	}
	if view.Message != "" {
		y := height - 2
		if session.IsTerminalStatus(view.SessionState) {
			y = height - 3
		}
		lines = append(lines, renderLine{y: y, text: view.Message, style: secondaryStyle})
	}

	drawSnake(screen, view.Game, style, view.NoColor)
	drawGoldenByte(screen, view.Quest, style, view.NoColor)
	drawPickup(screen, view.Quest, style, view.NoColor)
	drawResultPanel(screen, view, style)
	drawCompletionDecisionPanel(screen, view, style)
	drawResizePausePanel(screen, view, style)

	for _, line := range lines {
		if line.centered {
			drawCenteredText(screen, 1, line.y, width-2, line.text, line.style)
			continue
		}
		drawText(screen, 1, line.y, width-2, line.text, line.style)
	}

	screen.Show()
}

type renderLine struct {
	y        int
	text     string
	style    tcell.Style
	centered bool
}

func baseRenderStyle(noColor bool) tcell.Style {
	if noColor {
		return tcell.StyleDefault
	}
	return tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
}

func colorStyle(style tcell.Style, noColor bool, foreground tcell.Color, background tcell.Color) tcell.Style {
	if noColor {
		return style
	}
	if foreground != tcell.ColorDefault {
		style = style.Foreground(foreground)
	}
	if background != tcell.ColorDefault {
		style = style.Background(background)
	}
	return style
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

func drawPlayfield(screen tcell.Screen, style tcell.Style, noColor bool) {
	width, height := screen.Size()
	arena := arenaForScreen(screen)
	topWallY := arena.Y - 1
	bottomWallY := arena.Y + arena.Height
	leftWallX := arena.X - 1
	rightWallX := arena.X + arena.RenderWidth()
	if width < 2 || height < 8 || topWallY <= 0 || bottomWallY >= height {
		return
	}

	boardStyle := colorStyle(style, noColor, tcell.ColorDefault, tcell.ColorDarkSlateGray)
	wallStyle := colorStyle(style.Bold(true), noColor, tcell.ColorTeal, tcell.ColorTeal)

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

func drawSnake(screen tcell.Screen, game *SnakeGame, baseStyle tcell.Style, noColor bool) {
	if game == nil {
		return
	}

	arena := arenaForScreen(screen)
	boardBackground := tcell.ColorDarkSlateGray
	foodStyle := colorStyle(baseStyle.Bold(true), noColor, tcell.ColorYellow, boardBackground)
	bodyStyle := colorStyle(baseStyle.Bold(true), noColor, tcell.ColorLimeGreen, boardBackground)
	tailStyle := colorStyle(baseStyle, noColor, tcell.ColorGreen, boardBackground)
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
		} else if index == len(game.Snake)-1 {
			cell = "▒▒"
			style = tailStyle
		}
		drawCell(screen, arena, point, cell, style)
	}
}

func drawGoldenByte(screen tcell.Screen, quest *QuestState, baseStyle tcell.Style, noColor bool) {
	if !quest.Enabled() || !quest.Golden.Active {
		return
	}
	arena := arenaForScreen(screen)
	point := quest.Golden.Position
	if point.X < 0 || point.X >= arena.Width || point.Y < 0 || point.Y >= arena.Height {
		return
	}
	style := colorStyle(baseStyle.Bold(true), noColor, tcell.ColorOrange, tcell.ColorDarkSlateGray)
	drawCell(screen, arena, point, "<>", style)
}

func drawPickup(screen tcell.Screen, quest *QuestState, baseStyle tcell.Style, noColor bool) {
	if !quest.Enabled() || !quest.Pickup.Active {
		return
	}
	arena := arenaForScreen(screen)
	point := quest.Pickup.Position
	if point.X < 0 || point.X >= arena.Width || point.Y < 0 || point.Y >= arena.Height {
		return
	}
	style := pickupStyle(baseStyle, quest.Pickup.Upgrade, noColor)
	drawCell(screen, arena, point, PickupSymbol(quest.Pickup.Upgrade), style)
}

func pickupStyle(baseStyle tcell.Style, upgrade Upgrade, noColor bool) tcell.Style {
	if noColor {
		return baseStyle.Bold(true)
	}
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

	lines := resultPanelLines(view, arena.RenderWidth())
	if len(lines) == 0 {
		return
	}

	accent := tcell.ColorOrange
	if view.Frozen {
		accent = tcell.ColorAqua
	}
	if view.PendingScore != nil {
		accent = tcell.ColorYellow
	}

	panelWidth := maxTextWidth(lines) + 4
	if panelWidth < 30 {
		panelWidth = 30
	}
	if panelWidth > arena.RenderWidth() {
		panelWidth = arena.RenderWidth()
	}
	panelHeight := len(lines) + 2
	if panelHeight > arena.Height {
		panelHeight = arena.Height
	}
	if panelHeight < 5 {
		panelHeight = 5
	}
	panelX := arena.X + (arena.RenderWidth()-panelWidth)/2
	panelY := arena.Y + (arena.Height-panelHeight)/2

	panelStyle := colorStyle(baseStyle, view.NoColor, tcell.ColorWhite, tcell.ColorBlack)
	borderStyle := colorStyle(baseStyle.Bold(true), view.NoColor, accent, tcell.ColorBlack)
	fillRect(screen, panelX, panelY, panelWidth, panelHeight, ' ', panelStyle)
	drawBox(screen, panelX, panelY, panelWidth, panelHeight, borderStyle)

	for index, line := range lines {
		if index >= panelHeight-2 {
			break
		}
		lineStyle := panelStyle
		if index == 0 {
			lineStyle = borderStyle
		}
		drawCenteredText(screen, panelX+1, panelY+1+index, panelWidth-2, line, lineStyle)
	}
}

func drawResizePausePanel(screen tcell.Screen, view viewState, baseStyle tcell.Style) {
	if !view.Pause.Resize {
		return
	}

	arena := arenaForScreen(screen)
	if arena.RenderWidth() < 16 || arena.Height < 4 {
		return
	}

	lines := []string{"Terminal too small", "Resize to continue"}
	if arena.RenderWidth() >= 36 {
		lines[0] = "Terminal too small for current Snake"
	}
	panelWidth := maxTextWidth(lines) + 4
	if panelWidth > arena.RenderWidth() {
		panelWidth = arena.RenderWidth()
	}
	panelHeight := len(lines) + 2
	panelX := arena.X + (arena.RenderWidth()-panelWidth)/2
	panelY := arena.Y + (arena.Height-panelHeight)/2

	panelStyle := colorStyle(baseStyle, view.NoColor, tcell.ColorWhite, tcell.ColorBlack)
	borderStyle := colorStyle(baseStyle.Bold(true), view.NoColor, tcell.ColorYellow, tcell.ColorBlack)
	fillRect(screen, panelX, panelY, panelWidth, panelHeight, ' ', panelStyle)
	drawBox(screen, panelX, panelY, panelWidth, panelHeight, borderStyle)
	for index, line := range lines {
		drawCenteredText(screen, panelX+1, panelY+1+index, panelWidth-2, line, panelStyle)
	}
}

func drawCompletionDecisionPanel(screen tcell.Screen, view viewState, baseStyle tcell.Style) {
	if view.Completion != CompletionUndecided {
		return
	}

	arena := arenaForScreen(screen)
	if arena.RenderWidth() < 16 || arena.Height < 5 {
		return
	}

	score := currentRoundScore(&view)
	lines := []string{
		"COMMAND FINISHED",
		resultSummary(view.State),
		fmt.Sprintf("Current Score  %d", score),
		"",
		"C Continue     Q Quit",
	}
	if arena.RenderWidth() < 34 {
		lines = []string{
			fmt.Sprintf("COMMAND FINISHED - Score %d", score),
			"C Continue  Q Quit",
		}
	}

	panelWidth := maxTextWidth(lines) + 4
	if panelWidth < 30 {
		panelWidth = 30
	}
	if panelWidth > arena.RenderWidth() {
		panelWidth = arena.RenderWidth()
	}
	panelHeight := len(lines) + 2
	if panelHeight > arena.Height {
		panelHeight = arena.Height
	}
	panelX := arena.X + (arena.RenderWidth()-panelWidth)/2
	panelY := arena.Y + (arena.Height-panelHeight)/2

	panelStyle := colorStyle(baseStyle, view.NoColor, tcell.ColorWhite, tcell.ColorBlack)
	borderStyle := colorStyle(baseStyle.Bold(true), view.NoColor, tcell.ColorAqua, tcell.ColorBlack)
	actionStyle := colorStyle(baseStyle.Bold(true), view.NoColor, tcell.ColorYellow, tcell.ColorBlack)
	fillRect(screen, panelX, panelY, panelWidth, panelHeight, ' ', panelStyle)
	drawBox(screen, panelX, panelY, panelWidth, panelHeight, borderStyle)
	for index, line := range lines {
		if index >= panelHeight-2 {
			break
		}
		lineStyle := panelStyle
		if index == 0 {
			lineStyle = borderStyle
		}
		if strings.Contains(line, "Continue") {
			lineStyle = actionStyle
		}
		drawCenteredText(screen, panelX+1, panelY+1+index, panelWidth-2, line, lineStyle)
	}
}

func resultPanelLines(view viewState, maxWidth int) []string {
	score := view.ResultScore
	if score == 0 && view.PendingScore == nil && !view.RoundFinalized {
		if view.Quest.Enabled() && view.FinalScore.FinalScore > 0 {
			score = view.FinalScore.FinalScore
		} else if view.Game != nil {
			score = view.Game.Score
		}
	}
	if view.PendingScore != nil {
		if maxWidth < 34 {
			return []string{
				fmt.Sprintf("NEW HIGH SCORE %d", view.PendingScore.Score),
				"NAME [" + truncateDisplay(view.PendingScore.Input, maxWidth-8) + "]",
				"Enter confirm",
			}
		}
		return []string{
			"NEW HIGH SCORE",
			"",
			fmt.Sprintf("FINAL SCORE  %d", view.PendingScore.Score),
			"",
			"ENTER YOUR NAME",
			"[ " + truncateDisplay(view.PendingScore.Input, maxWidth-8) + " ]",
			"",
			"Type name - Enter confirm",
		}
	}

	title := "GAME OVER"
	action := "R Restart     F9 Hide"
	if view.Frozen {
		title = "COMMAND FINISHED"
		action = "Q Quit"
	}
	if maxWidth < 34 {
		lines := []string{
			title,
			fmt.Sprintf("%d", score),
		}
		lines = append(lines, leaderboardLines(view.Leaderboard, view.CurrentRank, maxWidth-2)...)
		lines = append(lines, action)
		if view.StatsMessage != "" {
			lines = append(lines, truncateDisplay(view.StatsMessage, maxWidth-2))
		}
		return lines
	}

	lines := []string{
		title,
		"",
		fmt.Sprintf("FINAL SCORE  %d", score),
		"",
		"TOP 5",
	}
	lines = append(lines, leaderboardLines(view.Leaderboard, view.CurrentRank, maxWidth-2)...)
	for len(lines) < 10 {
		lines = append(lines, "")
	}
	lines = append(lines, action)
	if view.StatsMessage != "" {
		lines = append(lines, truncateDisplay(view.StatsMessage, maxWidth-2))
	}
	return lines
}

func leaderboardLines(entries []LeaderboardEntry, currentRank int, maxWidth int) []string {
	if len(entries) == 0 {
		return []string{"No scores yet"}
	}
	lines := make([]string, 0, len(entries))
	for index, entry := range entries {
		marker := ""
		if currentRank == index+1 {
			marker = " <- YOU"
		}
		prefix := fmt.Sprintf("%d. %5d  ", index+1, entry.Score)
		nameWidth := maxWidth - textDisplayWidth(prefix) - textDisplayWidth(marker)
		if nameWidth < 3 {
			nameWidth = 3
		}
		lines = append(lines, prefix+truncateDisplay(entry.PlayerName, nameWidth)+marker)
	}
	return lines
}

func truncateDisplay(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if textDisplayWidth(text) <= maxWidth {
		return text
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}
	limit := maxWidth - 3
	var builder strings.Builder
	width := 0
	for _, r := range text {
		rw := runeDisplayWidth(r)
		if width+rw > limit {
			break
		}
		builder.WriteRune(r)
		width += rw
	}
	builder.WriteString("...")
	return builder.String()
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
	width := 0
	for _, r := range text {
		width += runeDisplayWidth(r)
	}
	return width
}

func updateViewHeat(view *viewState, now time.Time) bool {
	previousCommandHeat := view.CommandHeat.Level
	previousHeat := view.Heat.Level
	previousNotice := view.HeatNotice

	elapsed := view.Clock.Elapsed(now)
	commandHeat := HeatForElapsed(elapsed)
	if view.HeatFrozen {
		commandHeat = view.FrozenHeat
		if commandHeat.Level == 0 {
			commandHeat = HeatByLevel(1)
		}
	}
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
	if !view.Frozen && !view.HeatFrozen {
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
	case pause.Completion:
		return "PAUSED - COMMAND FINISHED"
	case pause.Resize:
		return "PAUSED - TERMINAL TOO SMALL"
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

func statusLine(view viewState) string {
	if view.PendingScore != nil {
		if session.IsTerminalStatus(view.SessionState) {
			return "New high score. Type name  Enter confirm  Q save+exit"
		}
		return "New high score. Type name  Enter confirm  F9 hide"
	}
	if view.Completion == CompletionUndecided {
		return "Command finished  C continue  Q quit  F9 hide  F10 detach"
	}
	if view.Frozen {
		return "Command finished  Q quit  F9 hide  F10 detach"
	}
	if view.Game != nil && view.Game.Over {
		return "Round over  R restart  F9 hide  F10 detach"
	}
	if view.Pause.Active() {
		return pauseLine(view.Pause)
	}
	if view.HeatNotice != "" {
		return view.HeatNotice
	}
	if view.Quest.Enabled() {
		return questLine(view)
	}
	if view.Started {
		return "Arrows/WASD move  P pause  F9 hide  F10 detach  F12 command"
	}
	return "Arrows/WASD start  F9 hide  F12 command  F10 detach"
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
