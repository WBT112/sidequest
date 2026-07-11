package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
}

type Result struct {
	Config      Config
	ShowHelp    bool
	ShowVersion bool
}

type App struct {
	Out            io.Writer
	Err            io.Writer
	Version        string
	Preflight      func() error
	CreateSession  func() (session.Session, error)
	ListSessions   func() ([]session.Record, error)
	AttachSession  func(string) error
	TmuxHasSession func(tmux.Info) bool
	CloseTmux      func(tmux.Info) error
	CloseGamePane  func(tmux.Info) error
	DetachClients  func(tmux.Info) error
	CapturePane    func(tmux.Info) (string, bool, error)
	CleanupSession func(session.Session) error
	StoreRun       func(session.Record, string, bool) (runhistory.Run, error)
	ListRuns       func() ([]runhistory.Run, error)
	FindRun        func(string) (runhistory.Run, error)
	OutputRun      func(string) error
	PurgeRun       func(string) error
	RunLayout      func(session.Session, session.Command) error
	ReceiveCommand func(context.Context, string) (session.Command, error)
	ExecCommand    func(session.Session, session.Command) error
	RunGameShell   func(string) error
	Now            func() time.Time
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
			return commandexec.ExitCodeForError(err)
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
		if err := a.runPreflight(); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return 2
		}

		command := session.Command{
			Executable: result.Config.Executable,
			Arguments:  result.Config.Arguments,
		}
		runtimeSession, err := a.createSession()
		if err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return 2
		}

		if err := a.runLayout(runtimeSession, command); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return 2
		}

		return 0
	}
}

func Parse(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{}, ErrMissingSeparator
	}

	for index, arg := range args {
		switch arg {
		case "-h", "--help":
			return Result{ShowHelp: true}, nil
		case "-v", "--version":
			return Result{ShowVersion: true}, nil
		case "--":
			return parseCommand(args[index+1:])
		default:
			if strings.HasPrefix(arg, "-") {
				return Result{}, fmt.Errorf("unknown option %q", arg)
			}
		}
	}

	return Result{}, ErrMissingSeparator
}

func Usage() string {
	return usage
}

func parseCommand(args []string) (Result, error) {
	if len(args) == 0 || args[0] == "" {
		return Result{}, ErrMissingCommand
	}

	return Result{
		Config: Config{
			Executable: args[0],
			Arguments:  append([]string(nil), args[1:]...),
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
	return a.cleanupClosedSession(updatedRecord)
}

func (a App) tmuxHasSession(info tmux.Info) bool {
	if a.TmuxHasSession != nil {
		return a.TmuxHasSession(info)
	}
	return tmux.Layout{}.HasSession(info)
}

func (a App) runLayout(runtimeSession session.Session, command session.Command) error {
	if a.RunLayout != nil {
		return a.RunLayout(runtimeSession, command)
	}

	listener, err := session.ListenCommand(runtimeSession)
	if err != nil {
		return err
	}
	defer listener.Close()

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve sidequest executable: %w", err)
	}

	layout := tmux.Layout{}
	info, err := layout.Start(
		runtimeSession,
		[]string{executable, commandRunnerMode, runtimeSession.SocketPath},
		[]string{executable, gameRunnerMode, runtimeSession.StatePath},
	)
	if err != nil {
		return err
	}
	if err := session.UpdateState(runtimeSession, a.now(), func(state *session.State) {
		state.TmuxSocket = info.SocketName
	}); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := listener.Serve(ctx, command); err != nil {
		return err
	}

	if err := layout.Attach(info); err != nil {
		return err
	}

	state, err := session.ReadState(runtimeSession)
	if err != nil {
		return err
	}
	return a.cleanupClosedSession(session.Record{Session: runtimeSession, State: state})
}

func (a App) runCommandRunner(socketPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	receive := a.ReceiveCommand
	if receive == nil {
		receive = session.ReceiveCommand
	}
	command, err := receive(ctx, socketPath)
	if err != nil {
		return err
	}
	runtimeSession := session.FromSocketPath(socketPath)

	execute := a.ExecCommand
	if execute == nil {
		execute = commandexec.DefaultExecutor().Run
	}
	return execute(runtimeSession, command)
}

func (a App) runGameShell(statePath string) error {
	if a.RunGameShell != nil {
		return a.RunGameShell(statePath)
	}

	runtimeSession := session.FromStatePath(statePath)
	shell := game.Shell{
		ReadState: func() (session.State, error) {
			return session.ReadState(runtimeSession)
		},
		OnQuitActive: func() error {
			state, err := session.ReadState(runtimeSession)
			if err != nil {
				return err
			}
			record := session.Record{Session: runtimeSession, State: state}
			info, owned := ownedInfoFromRecord(record)
			if !owned {
				return nil
			}
			return a.closeGamePane(info)
		},
		OnQuitTerminal: func() error {
			state, err := session.ReadState(runtimeSession)
			if err != nil {
				return err
			}
			record := session.Record{Session: runtimeSession, State: state}
			info, owned := ownedInfoFromRecord(record)
			if !owned {
				return nil
			}
			return a.detachClients(info)
		},
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
		if err := a.closeTmux(info); err != nil {
			return err
		}
	}
	return a.cleanupSession(record.Session)
}

func (a App) printStoredRun(run runhistory.Run) {
	fmt.Fprintf(a.outputWriter(), "Saved output: %s\n", run.OutputPath)
	fmt.Fprintf(a.outputWriter(), "View it with: sidequest output %s\n", run.Result.ID)
	fmt.Fprintf(a.outputWriter(), "Metadata: sidequest show %s\n", run.Result.ID)
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

func (a App) closeGamePane(info tmux.Info) error {
	if a.CloseGamePane != nil {
		return a.CloseGamePane(info)
	}
	return tmux.Layout{}.CloseGamePane(info)
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
