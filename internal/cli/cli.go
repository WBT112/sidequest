package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/WBT112/sidequest/internal/commandexec"
	"github.com/WBT112/sidequest/internal/game"
	"github.com/WBT112/sidequest/internal/preflight"
	"github.com/WBT112/sidequest/internal/runhistory"
	"github.com/WBT112/sidequest/internal/session"
	"github.com/WBT112/sidequest/internal/tmux"
)

const commandRunnerMode = "__sidequest-command-runner"
const gameRunnerMode = "__sidequest-game"

const usage = `Usage:
  sidequest list
  sidequest attach <session-id>
  sidequest runs
  sidequest show last|<run-id>
  sidequest output last|<run-id>
  sidequest purge <run-id>
  sidequest [options] -- <command> [arguments...]

Options:
  -h, --help       Show this help text.
  -v, --version    Show the sidequest version.
  --mode <mode>    Select game mode: classic or quest.
  --graphics <mode>
                   Select graphics: auto, ascii, or rich.
  --no-history     Do not persist command-pane output after the run.
  --no-color       Disable Sidequest game/UI colors.
  --aug            Show augmented live command context in the game pane.

The -- separator marks the end of Sidequest options and the start of the
command to run. Command arguments are preserved exactly and are not passed
through a shell.
`

var (
	ErrMissingSeparator = errors.New("missing command separator --")
	ErrMissingCommand   = errors.New("missing command after --")
)

type Config struct {
	Executable string
	Arguments  []string
	Mode       string
	Graphics   string
	NoHistory  bool
	NoColor    bool
	Augmented  bool
}

type Result struct {
	Config      Config
	ShowHelp    bool
	ShowVersion bool
}

type App struct {
	Out             io.Writer
	Err             io.Writer
	Version         string
	Preflight       func() error
	CreateSession   func() (session.Session, error)
	ListSessions    func() ([]session.Record, error)
	AttachSession   func(string) error
	TmuxHasSession  func(tmux.Info) bool
	CloseTmux       func(tmux.Info) error
	DetachClients   func(tmux.Info) error
	CapturePane     func(tmux.Info) (string, bool, error)
	CleanupSession  func(session.Session) error
	StoreRun        func(session.Record, string, bool) (runhistory.Run, error)
	ListRuns        func() ([]runhistory.Run, error)
	FindRun         func(string) (runhistory.Run, error)
	OutputRun       func(string) error
	PurgeRun        func(string) error
	RunLayout       func(session.Session, session.Command) error
	StartLayout     func(session.Session, []string, []string) (tmux.Info, error)
	AttachLayout    func(tmux.Info) error
	ServeCommand    func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error)
	UpdateState     func(session.Session, time.Time, func(*session.State)) error
	ReadState       func(session.Session) (session.State, error)
	ReceiveCommand  func(context.Context, string) (session.Command, error)
	ReceiveExchange func(context.Context, string) (session.Command, *session.CommandExchange, error)
	ExecCommand     func(session.Session, session.Command) error
	RunGameShell    func(string) error
	RunShell        func(game.Shell) error
	Now             func() time.Time
}

func (a App) Run(args []string) int {
	if len(args) == 2 && args[0] == gameRunnerMode {
		if err := a.runGameShell(args[1]); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest game: %v\n", err)
			return 2
		}
		return 0
	}
	if len(args) == 2 && args[0] == commandRunnerMode {
		if err := a.runCommandRunner(args[1]); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest command runner: %v\n", err)
			return exitCodeForError(err)
		}
		return 0
	}
	if len(args) > 0 {
		switch args[0] {
		case "list":
			if len(args) != 1 {
				fmt.Fprintf(a.errorWriter(), "sidequest: list does not accept arguments\n\n%s", usage)
				return 2
			}
			if err := a.runList(); err != nil {
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
			return 0
		case "attach":
			if len(args) != 2 {
				fmt.Fprintf(a.errorWriter(), "sidequest: attach requires a session id\n\n%s", usage)
				return 2
			}
			if err := a.runAttach(args[1]); err != nil {
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
			return 0
		case "runs":
			if len(args) != 1 {
				fmt.Fprintf(a.errorWriter(), "sidequest: runs does not accept arguments\n\n%s", usage)
				return 2
			}
			if err := a.runRuns(); err != nil {
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
			return 0
		case "show":
			if len(args) != 2 {
				fmt.Fprintf(a.errorWriter(), "sidequest: show requires last or a run id\n\n%s", usage)
				return 2
			}
			if err := a.runShow(args[1]); err != nil {
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
			return 0
		case "output":
			if len(args) != 2 {
				fmt.Fprintf(a.errorWriter(), "sidequest: output requires last or a run id\n\n%s", usage)
				return 2
			}
			if err := a.runOutput(args[1]); err != nil {
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
			return 0
		case "purge":
			if len(args) != 2 {
				fmt.Fprintf(a.errorWriter(), "sidequest: purge requires a run id\n\n%s", usage)
				return 2
			}
			if err := a.runPurge(args[1]); err != nil {
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
			return 0
		}
	}

	result, err := Parse(args)
	if err != nil {
		fmt.Fprintf(a.errorWriter(), "sidequest: %v\n\n%s", err, usage)
		return 2
	}

	switch {
	case result.ShowHelp:
		fmt.Fprint(a.outputWriter(), usage)
		return 0
	case result.ShowVersion:
		fmt.Fprintf(a.outputWriter(), "sidequest %s\n", a.version())
		return 0
	default:
		command := session.Command{
			Executable: result.Config.Executable,
			Arguments:  result.Config.Arguments,
		}
		if err := validateCommandExecutable(command.Executable); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return exitCodeForError(err)
		}
		if err := a.runPreflight(); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return 2
		}
		runtimeSession, err := a.createSession()
		if err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return 2
		}
		if runtimeSession.StatePath != "" {
			if err := a.updateState(runtimeSession, a.now(), func(state *session.State) {
				state.GameMode = result.Config.Mode
				state.GraphicsMode = result.Config.Graphics
				state.NoHistory = result.Config.NoHistory
				state.NoColor = result.Config.NoColor
				state.Augmented = result.Config.Augmented
			}); err != nil {
				err = a.cleanupStartupFailure(err, runtimeSession, tmux.Info{}, false)
				fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
				return 2
			}
		}

		if err := a.runLayout(runtimeSession, command); err != nil {
			if a.RunLayout != nil {
				err = a.cleanupStartupFailure(err, runtimeSession, tmux.Info{}, false)
			}
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return exitCodeForError(err)
		}

		return 0
	}
}

func Parse(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{}, ErrMissingSeparator
	}

	mode := game.GameModeClassic
	graphics := game.GraphicsModeAuto
	noHistory := false
	noColor := os.Getenv("NO_COLOR") != ""
	augmented := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "-h", "--help":
			return Result{ShowHelp: true}, nil
		case "-v", "--version":
			return Result{ShowVersion: true}, nil
		case "--mode":
			if index+1 >= len(args) {
				return Result{}, fmt.Errorf("--mode requires classic or quest")
			}
			selectedMode, err := parseMode(args[index+1])
			if err != nil {
				return Result{}, err
			}
			mode = selectedMode
			index++
		case "--graphics":
			if index+1 >= len(args) {
				return Result{}, fmt.Errorf("--graphics requires auto, ascii, or rich")
			}
			selectedGraphics, err := game.ParseGraphicsMode(args[index+1])
			if err != nil {
				return Result{}, err
			}
			graphics = selectedGraphics
			index++
		case "--no-history":
			noHistory = true
		case "--no-color":
			noColor = true
		case "--aug":
			augmented = true
		case "--":
			return parseCommand(args[index+1:], mode, graphics, noHistory, noColor, augmented)
		default:
			if strings.HasPrefix(arg, "--mode=") {
				selectedMode, err := parseMode(strings.TrimPrefix(arg, "--mode="))
				if err != nil {
					return Result{}, err
				}
				mode = selectedMode
				continue
			}
			if strings.HasPrefix(arg, "--graphics=") {
				selectedGraphics, err := game.ParseGraphicsMode(strings.TrimPrefix(arg, "--graphics="))
				if err != nil {
					return Result{}, err
				}
				graphics = selectedGraphics
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return Result{}, fmt.Errorf("unknown option %q", arg)
			}
			if containsSeparator(args[index+1:]) {
				return Result{}, fmt.Errorf("unexpected argument %q before --", arg)
			}
		}
	}

	return Result{}, ErrMissingSeparator
}

func containsSeparator(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return true
		}
	}
	return false
}

func parseMode(mode string) (string, error) {
	switch mode {
	case game.GameModeClassic, game.GameModeQuest:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown mode %q", mode)
	}
}

func Usage() string {
	return usage
}

func parseCommand(args []string, mode string, graphics string, noHistory bool, noColor bool, augmented bool) (Result, error) {
	if len(args) == 0 || args[0] == "" {
		return Result{}, ErrMissingCommand
	}

	return Result{
		Config: Config{
			Executable: args[0],
			Arguments:  append([]string(nil), args[1:]...),
			Mode:       mode,
			Graphics:   graphics,
			NoHistory:  noHistory,
			NoColor:    noColor,
			Augmented:  augmented,
		},
	}, nil
}

func (a App) outputWriter() io.Writer {
	if a.Out != nil {
		return a.Out
	}
	return io.Discard
}

func (a App) errorWriter() io.Writer {
	if a.Err != nil {
		return a.Err
	}
	return io.Discard
}

func (a App) version() string {
	if a.Version != "" {
		return a.Version
	}
	return "dev"
}

func (a App) runPreflight() error {
	if a.Preflight != nil {
		return a.Preflight()
	}
	return preflight.Validate(preflight.DefaultEnvironment())
}

func (a App) createSession() (session.Session, error) {
	if a.CreateSession != nil {
		return a.CreateSession()
	}
	manager := session.DefaultManager()
	if err := a.cleanupStaleFinishedSessions(manager); err != nil {
		return session.Session{}, err
	}
	return manager.Create()
}

func (a App) listSessions() ([]session.Record, error) {
	if a.ListSessions != nil {
		return a.ListSessions()
	}
	manager := session.DefaultManager()
	if err := a.cleanupStaleFinishedSessions(manager); err != nil {
		return nil, err
	}
	return manager.List()
}

func (a App) runList() error {
	records, err := a.listSessions()
	if err != nil {
		return err
	}

	fmt.Fprintln(a.outputWriter(), "ID             STATE        STARTED      ELAPSED")
	for _, record := range records {
		state := record.State
		displayState := state.Status
		if state.TmuxSocket != "" {
			info, owned := ownedInfoFromRecord(record)
			if !owned || !a.tmuxHasSession(info) {
				displayState = "stale"
			}
		}

		startedAt := displayStartTime(state)
		fmt.Fprintf(
			a.outputWriter(),
			"%-14s %-12s %-12s %s\n",
			record.Session.ID,
			displayState,
			startedAt,
			formatElapsed(state, a.now()),
		)
	}

	return nil
}

func (a App) runRuns() error {
	runs, err := a.listRuns()
	if err != nil {
		return err
	}

	fmt.Fprintln(a.outputWriter(), "ID             FINISHED             EXIT   DURATION")
	for _, run := range runs {
		fmt.Fprintf(
			a.outputWriter(),
			"%-14s %-20s %-6s %s\n",
			run.Result.ID,
			run.Result.FinishedAt.Local().Format("2006-01-02 15:04:05"),
			runhistory.FormatExitCode(run.Result.ExitCode),
			runhistory.FormatDuration(run.Result.DurationMillis),
		)
	}
	return nil
}

func (a App) runShow(id string) error {
	run, err := a.findRun(id)
	if err != nil {
		return err
	}
	exitCode := runhistory.FormatExitCode(run.Result.ExitCode)
	fmt.Fprintf(a.outputWriter(), "ID: %s\n", run.Result.ID)
	fmt.Fprintf(a.outputWriter(), "Started: %s\n", run.Result.StartedAt.Local().Format(time.RFC3339))
	fmt.Fprintf(a.outputWriter(), "Finished: %s\n", run.Result.FinishedAt.Local().Format(time.RFC3339))
	fmt.Fprintf(a.outputWriter(), "Duration: %s\n", runhistory.FormatDuration(run.Result.DurationMillis))
	fmt.Fprintf(a.outputWriter(), "Exit code: %s\n", exitCode)
	fmt.Fprintf(a.outputWriter(), "Termination: %s\n", run.Result.Termination)
	fmt.Fprintf(a.outputWriter(), "Output truncated: %t\n", run.Result.OutputTruncated)
	fmt.Fprintf(a.outputWriter(), "Output: %s\n", run.OutputPath)
	return nil
}

func (a App) runOutput(id string) error {
	if a.OutputRun != nil {
		return a.OutputRun(id)
	}
	manager := runhistory.DefaultManager()
	manager.Out = a.outputWriter()
	return manager.Output(id)
}

func (a App) runPurge(id string) error {
	if a.PurgeRun != nil {
		return a.PurgeRun(id)
	}
	return runhistory.DefaultManager().Purge(id)
}

func (a App) runAttach(id string) error {
	if err := a.runPreflight(); err != nil {
		return err
	}
	if a.AttachSession != nil {
		return a.AttachSession(id)
	}

	record, err := session.DefaultManager().Find(id)
	if err != nil {
		return err
	}
	if record.State.TmuxSocket == "" {
		return fmt.Errorf("session %q has no tmux socket metadata; it cannot be attached", id)
	}

	info, owned := ownedInfoFromRecord(record)
	if !owned {
		return fmt.Errorf("session %q has invalid Sidequest tmux metadata", id)
	}
	layout := tmux.Layout{}
	if !a.tmuxHasSession(info) {
		return fmt.Errorf("session %q is stale: tmux server %q is no longer running", id, info.SocketName)
	}
	if err := layout.Attach(info); err != nil {
		return err
	}
	updatedRecord, err := session.DefaultManager().Find(id)
	if err != nil {
		return err
	}
	a.printReconnectHint(updatedRecord.Session, updatedRecord.State)
	return a.cleanupClosedSession(updatedRecord)
}

func (a App) tmuxHasSession(info tmux.Info) bool {
	if a.TmuxHasSession != nil {
		return a.TmuxHasSession(info)
	}
	return tmux.Layout{}.HasSession(info)
}

func (a App) startLayout(runtimeSession session.Session, commandRunner []string, gameRunner []string) (tmux.Info, error) {
	if a.StartLayout != nil {
		return a.StartLayout(runtimeSession, commandRunner, gameRunner)
	}
	return tmux.Layout{}.Start(runtimeSession, commandRunner, gameRunner)
}

func (a App) attachLayout(info tmux.Info) error {
	if a.AttachLayout != nil {
		return a.AttachLayout(info)
	}
	return tmux.Layout{}.Attach(info)
}

func (a App) serveCommand(ctx context.Context, listener *session.CommandListener, command session.Command) (session.CommandStartup, error) {
	if a.ServeCommand != nil {
		return a.ServeCommand(ctx, listener, command)
	}
	return listener.Serve(ctx, command)
}

func (a App) updateState(runtimeSession session.Session, now time.Time, update func(*session.State)) error {
	if a.UpdateState != nil {
		return a.UpdateState(runtimeSession, now, update)
	}
	return session.UpdateState(runtimeSession, now, update)
}

func (a App) readState(runtimeSession session.Session) (session.State, error) {
	if a.ReadState != nil {
		return a.ReadState(runtimeSession)
	}
	return session.ReadState(runtimeSession)
}

func (a App) runLayout(runtimeSession session.Session, command session.Command) (err error) {
	if a.RunLayout != nil {
		return a.RunLayout(runtimeSession, command)
	}

	listener, err := session.ListenCommand(runtimeSession)
	if err != nil {
		return a.cleanupStartupFailure(err, runtimeSession, tmux.Info{}, false)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			err = appendCleanupError(err, fmt.Errorf("close command listener: %w", closeErr))
		}
	}()

	executable, err := os.Executable()
	if err != nil {
		return a.cleanupStartupFailure(fmt.Errorf("resolve sidequest executable: %w", err), runtimeSession, tmux.Info{}, false)
	}

	info, err := a.startLayout(
		runtimeSession,
		[]string{executable, commandRunnerMode, runtimeSession.SocketPath},
		[]string{executable, gameRunnerMode, runtimeSession.StatePath},
	)
	if err != nil {
		return a.cleanupStartupFailure(err, runtimeSession, info, ownsTmuxInfo(runtimeSession, info))
	}
	if err := a.updateState(runtimeSession, a.now(), func(state *session.State) {
		state.TmuxSocket = info.SocketName
	}); err != nil {
		return a.cleanupStartupFailure(err, runtimeSession, info, ownsTmuxInfo(runtimeSession, info))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	startup, err := a.serveCommand(ctx, listener, command)
	if err != nil {
		return a.cleanupStartupFailure(err, runtimeSession, info, ownsTmuxInfo(runtimeSession, info))
	}
	if startup.Status != session.CommandStartupStarted {
		return a.handleStartupTerminal(runtimeSession, info, startup)
	}

	if err := a.attachLayout(info); err != nil {
		return a.cleanupStartupFailure(err, runtimeSession, info, ownsTmuxInfo(runtimeSession, info))
	}

	state, err := a.readState(runtimeSession)
	if err != nil {
		return a.cleanupStartupFailure(err, runtimeSession, info, ownsTmuxInfo(runtimeSession, info))
	}
	a.printReconnectHint(runtimeSession, state)
	return a.cleanupClosedSession(session.Record{Session: runtimeSession, State: state})
}

func (a App) runCommandRunner(socketPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if a.ReceiveCommand != nil {
		command, err := a.ReceiveCommand(ctx, socketPath)
		if err != nil {
			return err
		}
		runtimeSession := session.FromSocketPath(socketPath)
		execute := a.ExecCommand
		if execute == nil {
			execute = commandexec.DefaultExecutor().Run
		}
		err = execute(runtimeSession, command)
		a.publishCommandPaneState(runtimeSession)
		return err
	}

	receive := a.ReceiveExchange
	if receive == nil {
		receive = session.ReceiveCommandExchange
	}
	command, exchange, err := receive(ctx, socketPath)
	if err != nil {
		return err
	}
	defer exchange.Close()

	runtimeSession := session.FromSocketPath(socketPath)

	execute := a.ExecCommand
	if execute != nil {
		if err := exchange.ReportStartup(session.CommandStartup{Status: session.CommandStartupStarted}); err != nil {
			return err
		}
		err := execute(runtimeSession, command)
		a.publishCommandPaneState(runtimeSession)
		return err
	}
	err = commandexec.DefaultExecutor().RunWithStartupReporter(runtimeSession, command, exchange)
	a.publishCommandPaneState(runtimeSession)
	return err
}

func (a App) publishCommandPaneState(runtimeSession session.Session) {
	state, err := session.ReadState(runtimeSession)
	if err != nil {
		return
	}
	record := session.Record{Session: runtimeSession, State: state}
	info, owned := ownedInfoFromRecord(record)
	if !owned {
		return
	}
	_ = tmux.Layout{}.SetCommandState(info, state)
}

func (a App) runGameShell(statePath string) error {
	if a.RunGameShell != nil {
		return a.RunGameShell(statePath)
	}

	runtimeSession := session.FromStatePath(statePath)
	shell := game.Shell{
		Random: rand.New(rand.NewSource(a.now().UnixNano())),
		ReadState: func() (session.State, error) {
			return session.ReadState(runtimeSession)
		},
		ReadFocus: func() (bool, error) {
			state, err := session.ReadState(runtimeSession)
			if err != nil {
				return false, err
			}
			record := session.Record{Session: runtimeSession, State: state}
			info, owned := ownedInfoFromRecord(record)
			if !owned {
				return false, fmt.Errorf("invalid Sidequest tmux metadata")
			}
			boss, err := tmux.Layout{}.BossState(info)
			if err != nil {
				return false, err
			}
			if boss.Hidden {
				return false, nil
			}
			return tmux.Layout{}.GamePaneActive(info)
		},
		ReadCommandPreview: func() (string, error) {
			state, err := session.ReadState(runtimeSession)
			if err != nil {
				return "", err
			}
			record := session.Record{Session: runtimeSession, State: state}
			info, owned := ownedInfoFromRecord(record)
			if !owned {
				return "", fmt.Errorf("invalid Sidequest tmux metadata")
			}
			return tmux.Layout{}.CaptureCommandPreview(info)
		},
		UpdatePanePause: func(paused bool) error {
			state, err := session.ReadState(runtimeSession)
			if err != nil {
				return err
			}
			record := session.Record{Session: runtimeSession, State: state}
			info, owned := ownedInfoFromRecord(record)
			if !owned {
				return nil
			}
			return tmux.Layout{}.SetGamePaused(info, paused)
		},
	}
	if a.RunShell != nil {
		return a.RunShell(shell)
	}
	return shell.Run(context.Background())
}

func (a App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a App) cleanupStaleFinishedSessions(manager session.Manager) error {
	_, err := manager.CleanupFinished(func(record session.Record) bool {
		if record.State.TmuxSocket == "" {
			return true
		}
		info, owned := ownedInfoFromRecord(record)
		if !owned {
			return true
		}
		return !a.tmuxHasSession(info)
	})
	return err
}

func (a App) cleanupClosedSession(record session.Record) error {
	if !session.IsTerminalStatus(record.State.Status) {
		return nil
	}

	if info, ok := ownedInfoFromRecord(record); ok {
		if a.tmuxHasSession(info) {
			if record.State.NoHistory {
				a.printHistoryDisabled(record.Session)
			} else {
				output, truncated, err := a.captureCommandPane(info)
				if err != nil {
					return err
				}
				run, err := a.storeRun(record, output, truncated)
				if err != nil {
					return err
				}
				a.printStoredRun(run)
			}
		}
		if err := a.closeTmux(info); err != nil {
			return err
		}
	}
	return a.cleanupSession(record.Session)
}

func (a App) printReconnectHint(runtimeSession session.Session, state session.State) {
	if session.IsTerminalStatus(state.Status) {
		return
	}
	fmt.Fprintf(a.outputWriter(), "Reconnect with: sidequest attach %s\n", runtimeSession.ID)
}

func (a App) printStoredRun(run runhistory.Run) {
	fmt.Fprintf(a.outputWriter(), "Saved output: %s\n", run.OutputPath)
	fmt.Fprintf(a.outputWriter(), "View it with: sidequest output %s\n", run.Result.ID)
	fmt.Fprintf(a.outputWriter(), "Metadata: sidequest show %s\n", run.Result.ID)
}

func (a App) printHistoryDisabled(runtimeSession session.Session) {
	fmt.Fprintf(a.outputWriter(), "History disabled: no command output saved for run %s\n", runtimeSession.ID)
}

func (a App) captureCommandPane(info tmux.Info) (string, bool, error) {
	if a.CapturePane != nil {
		return a.CapturePane(info)
	}
	return tmux.Layout{}.CaptureCommandPane(info)
}

func (a App) storeRun(record session.Record, output string, truncated bool) (runhistory.Run, error) {
	if a.StoreRun != nil {
		return a.StoreRun(record, output, truncated)
	}
	return runhistory.DefaultManager().Store(record, output, truncated)
}

func (a App) closeTmux(info tmux.Info) error {
	if a.CloseTmux != nil {
		return a.CloseTmux(info)
	}
	return tmux.Layout{}.Close(info)
}

func (a App) detachClients(info tmux.Info) error {
	if a.DetachClients != nil {
		return a.DetachClients(info)
	}
	return tmux.Layout{}.DetachClients(info)
}

func (a App) cleanupSession(runtimeSession session.Session) error {
	if a.CleanupSession != nil {
		return a.CleanupSession(runtimeSession)
	}
	return session.Cleanup(runtimeSession)
}

func (a App) cleanupStartupFailure(primary error, runtimeSession session.Session, info tmux.Info, tmuxStarted bool) error {
	err := primary
	if tmuxStarted {
		err = appendCleanupError(err, a.closeTmux(info))
	}
	return appendCleanupError(err, a.cleanupSession(runtimeSession))
}

func (a App) handleStartupTerminal(runtimeSession session.Session, info tmux.Info, startup session.CommandStartup) error {
	if ownsTmuxInfo(runtimeSession, info) && a.tmuxHasSession(info) {
		if output, _, err := a.captureCommandPane(info); err == nil && output != "" {
			fmt.Fprint(a.outputWriter(), output)
		}
	}

	primary := startupError(startup)
	return a.cleanupStartupFailure(primary, runtimeSession, info, ownsTmuxInfo(runtimeSession, info))
}

func ownsTmuxInfo(runtimeSession session.Session, info tmux.Info) bool {
	want := "sidequest-" + runtimeSession.ID
	return info.SocketName == want && info.SessionName == want
}

func appendCleanupError(primary error, cleanupErr error) error {
	if cleanupErr == nil {
		return primary
	}
	if primary == nil {
		return cleanupErr
	}
	return fmt.Errorf("%w; cleanup failed: %v", primary, cleanupErr)
}

type exitCoder interface {
	ExitCode() int
}

type commandExitError struct {
	message string
	code    int
}

func (e commandExitError) Error() string {
	return e.message
}

func (e commandExitError) ExitCode() int {
	return e.code
}

func exitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var coded exitCoder
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return commandexec.ExitCodeForError(exitErr)
	}
	return 2
}

func validateCommandExecutable(executable string) error {
	if executable == "" {
		return session.ErrEmptyExecutable
	}
	if strings.ContainsRune(executable, os.PathSeparator) {
		return validateExplicitExecutable(executable)
	}
	if _, err := exec.LookPath(executable); err != nil {
		return commandExitError{message: fmt.Sprintf("command not found: %s", executable), code: 127}
	}
	return nil
}

func validateExplicitExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return commandExitError{message: fmt.Sprintf("command not found: %s", path), code: 127}
		}
		if errors.Is(err, os.ErrPermission) {
			return commandExitError{message: fmt.Sprintf("cannot start %s: permission denied", path), code: 126}
		}
		return err
	}
	if info.IsDir() {
		return commandExitError{message: fmt.Sprintf("cannot start %s: is a directory", path), code: 126}
	}
	if info.Mode().Perm()&0o111 == 0 {
		return commandExitError{message: fmt.Sprintf("cannot start %s: permission denied", path), code: 126}
	}
	return nil
}

func startupError(startup session.CommandStartup) error {
	switch startup.Status {
	case session.CommandStartupStartFailed:
		return commandExitError{message: startupErrorMessage(startup, "command failed to start"), code: startupFailureExitCode(startup)}
	case session.CommandStartupCompleted:
		return nil
	case session.CommandStartupFailed:
		return commandExitError{message: startupErrorMessage(startup, "command exited during startup"), code: startupExitCode(startup, 1)}
	case session.CommandStartupInterrupted:
		return commandExitError{message: startupErrorMessage(startup, "command interrupted during startup"), code: startupExitCode(startup, 130)}
	default:
		return fmt.Errorf("unexpected command startup status %q", startup.Status)
	}
}

func startupErrorMessage(startup session.CommandStartup, fallback string) string {
	if startup.Error != "" {
		return startup.Error
	}
	if startup.ExitCode != nil {
		return fmt.Sprintf("%s with status %d", fallback, *startup.ExitCode)
	}
	if startup.ExitSignal != "" {
		return fmt.Sprintf("%s by signal %s", fallback, startup.ExitSignal)
	}
	return fallback
}

func startupFailureExitCode(startup session.CommandStartup) int {
	if startup.Error != "" && strings.Contains(strings.ToLower(startup.Error), "permission denied") {
		return 126
	}
	return 127
}

func startupExitCode(startup session.CommandStartup, fallback int) int {
	if startup.ExitCode != nil {
		return *startup.ExitCode
	}
	if startup.ExitSignal != "" {
		return fallback
	}
	return fallback
}

func (a App) listRuns() ([]runhistory.Run, error) {
	if a.ListRuns != nil {
		return a.ListRuns()
	}
	return runhistory.DefaultManager().List()
}

func (a App) findRun(id string) (runhistory.Run, error) {
	if a.FindRun != nil {
		return a.FindRun(id)
	}
	return runhistory.DefaultManager().Find(id)
}

func infoFromRecord(record session.Record) tmux.Info {
	return tmux.Info{
		SocketName:  record.State.TmuxSocket,
		SessionName: "sidequest-" + record.Session.ID,
	}
}

func ownedInfoFromRecord(record session.Record) (tmux.Info, bool) {
	info := infoFromRecord(record)
	want := "sidequest-" + record.Session.ID
	return info, info.SocketName == want && info.SessionName == want
}

func displayStartTime(state session.State) string {
	started := state.CreatedAt
	if state.StartedAt != nil {
		started = *state.StartedAt
	}
	if started.IsZero() {
		return "-"
	}
	return started.Local().Format("15:04:05")
}

func formatElapsed(state session.State, now time.Time) string {
	var elapsed time.Duration
	switch {
	case state.DurationMillis != nil:
		elapsed = time.Duration(*state.DurationMillis) * time.Millisecond
	case state.StartedAt != nil:
		elapsed = now.Sub(*state.StartedAt)
	default:
		elapsed = now.Sub(state.CreatedAt)
	}
	if elapsed < 0 {
		elapsed = 0
	}

	total := int64(elapsed.Round(time.Second).Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
