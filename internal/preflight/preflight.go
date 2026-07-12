package preflight

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

const (
	MinimumColumns = 80
	MinimumRows    = 16
)

var (
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	ErrMissingTmux         = errors.New("missing tmux")
	ErrNonInteractive      = errors.New("non-interactive terminal")
	ErrTerminalSize        = errors.New("terminal size unavailable")
	ErrTerminalTooSmall    = errors.New("terminal too small")
)

type Size struct {
	Columns int
	Rows    int
}

type Environment struct {
	GOOS         string
	LookPath     func(string) (string, error)
	StdinFD      uintptr
	StdoutFD     uintptr
	IsTerminal   func(uintptr) bool
	TerminalSize func(uintptr) (Size, error)
}

func DefaultEnvironment() Environment {
	return Environment{
		GOOS:         runtime.GOOS,
		LookPath:     exec.LookPath,
		StdinFD:      os.Stdin.Fd(),
		StdoutFD:     os.Stdout.Fd(),
		IsTerminal:   isTerminal,
		TerminalSize: terminalSize,
	}
}

func Validate(env Environment) error {
	if env.GOOS == "" {
		env.GOOS = runtime.GOOS
	}
	if env.LookPath == nil {
		env.LookPath = exec.LookPath
	}
	if env.IsTerminal == nil {
		env.IsTerminal = isTerminal
	}
	if env.TerminalSize == nil {
		env.TerminalSize = terminalSize
	}

	if env.GOOS != "linux" {
		return fmt.Errorf("%w: sidequest currently supports Linux only (detected %s)", ErrUnsupportedPlatform, env.GOOS)
	}

	stdinTTY := env.IsTerminal(env.StdinFD)
	stdoutTTY := env.IsTerminal(env.StdoutFD)
	if !stdinTTY || !stdoutTTY {
		return fmt.Errorf(
			"%w: stdin and stdout must both be attached to a terminal (stdin=%t, stdout=%t)",
			ErrNonInteractive,
			stdinTTY,
			stdoutTTY,
		)
	}

	size, err := env.TerminalSize(env.StdoutFD)
	if err != nil {
		return fmt.Errorf("%w: could not read terminal dimensions: %v", ErrTerminalSize, err)
	}
	if size.Columns < MinimumColumns || size.Rows < MinimumRows {
		return fmt.Errorf(
			"%w: required at least %dx%d, detected %dx%d",
			ErrTerminalTooSmall,
			MinimumColumns,
			MinimumRows,
			size.Columns,
			size.Rows,
		)
	}

	if _, err := env.LookPath("tmux"); err != nil {
		return fmt.Errorf("%w: tmux was not found in PATH; install tmux and try again", ErrMissingTmux)
	}

	return nil
}
