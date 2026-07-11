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
	"github.com/WBT112/sidequest/internal/preflight"
	"github.com/WBT112/sidequest/internal/session"
	"github.com/WBT112/sidequest/internal/tmux"
)

const commandRunnerMode = "__sidequest-command-runner"

const usage = `Usage:
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
	RunLayout      func(session.Session, session.Command) error
	ReceiveCommand func(context.Context, string) (session.Command, error)
	ExecCommand    func(session.Session, session.Command) error
	Now            func() time.Time
}

func (a App) Run(args []string) int {
	if len(args) == 2 && args[0] == commandRunnerMode {
		if err := a.runCommandRunner(args[1]); err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest command runner: %v\n", err)
			return commandexec.ExitCodeForError(err)
		}
		return 0
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
	return session.DefaultManager().Create()
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
	info, err := layout.Start(runtimeSession, []string{executable, commandRunnerMode, runtimeSession.SocketPath})
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

	return layout.Attach(info)
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

func (a App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
