package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/WBT112/sidequest/internal/preflight"
	"github.com/WBT112/sidequest/internal/session"
)

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
	HandoffCommand func(session.Session, session.Command) (session.Command, error)
}

func (a App) Run(args []string) int {
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
		receivedCommand, err := a.handoffCommand(runtimeSession, command)
		if err != nil {
			fmt.Fprintf(a.errorWriter(), "sidequest: %v\n", err)
			return 2
		}

		fmt.Fprintf(
			a.outputWriter(),
			"created session: %s\ncommand handoff:\n  executable: %q\n  arguments: %q\n",
			runtimeSession.ID,
			receivedCommand.Executable,
			receivedCommand.Arguments,
		)
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

func (a App) handoffCommand(runtimeSession session.Session, command session.Command) (session.Command, error) {
	if a.HandoffCommand != nil {
		return a.HandoffCommand(runtimeSession, command)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener, err := session.ListenCommand(runtimeSession)
	if err != nil {
		return session.Command{}, err
	}
	defer listener.Close()

	errc := make(chan error, 1)
	go func() {
		errc <- session.SendCommand(ctx, runtimeSession.SocketPath, command)
	}()

	receivedCommand, err := listener.Receive(ctx)
	if err != nil {
		return session.Command{}, err
	}
	if err := <-errc; err != nil {
		return session.Command{}, err
	}

	return receivedCommand, nil
}
