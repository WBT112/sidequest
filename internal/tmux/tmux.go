package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/WBT112/sidequest/internal/session"
)

const placeholderCommand = "printf 'Sidequest placeholder pane\\n'; while :; do sleep 3600; done"

type Runner interface {
	Run(name string, args ...string) error
}

type ExecRunner struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

type Layout struct {
	TmuxPath      string
	CommandRunner Runner
}

type Info struct {
	SocketName  string
	SessionName string
}

func (l Layout) Start(runtimeSession session.Session, commandRunner []string) (Info, error) {
	if len(commandRunner) == 0 {
		return Info{}, fmt.Errorf("missing command runner")
	}

	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = ExecRunner{}
	}

	info := Info{
		SocketName:  "sidequest-" + runtimeSession.ID,
		SessionName: "sidequest-" + runtimeSession.ID,
	}

	baseArgs := []string{"-f", "/dev/null", "-L", info.SocketName}
	run := func(args ...string) error {
		return runner.Run(tmuxPath, append(baseArgs, args...)...)
	}

	if err := run("new-session", "-d", "-s", info.SessionName, "-n", "sidequest", shellJoin(commandRunner)); err != nil {
		return Info{}, fmt.Errorf("create tmux session: %w", err)
	}

	cleanup := func(original error) (Info, error) {
		_ = run("kill-session", "-t", info.SessionName)
		return Info{}, original
	}

	if err := run("set-option", "-t", info.SessionName, "status", "off"); err != nil {
		return cleanup(fmt.Errorf("disable tmux status bar: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "remain-on-exit", "on"); err != nil {
		return cleanup(fmt.Errorf("enable command pane remain-on-exit: %w", err))
	}
	if err := run("split-window", "-v", "-l", "10", "-t", info.SessionName+":0.0", placeholderCommand); err != nil {
		return cleanup(fmt.Errorf("create placeholder pane: %w", err))
	}
	if err := run("bind-key", "-n", "F12", "select-pane", "-t", ":.+"); err != nil {
		return cleanup(fmt.Errorf("bind F12 pane switch: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.0"); err != nil {
		return cleanup(fmt.Errorf("focus command pane: %w", err))
	}

	return info, nil
}

func (l Layout) Attach(info Info) error {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = ExecRunner{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	}

	if err := runner.Run(tmuxPath, "-f", "/dev/null", "-L", info.SocketName, "attach-session", "-t", info.SessionName); err != nil {
		return fmt.Errorf("attach tmux session: %w", err)
	}

	return nil
}

func (l Layout) HasSession(info Info) bool {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = ExecRunner{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	}

	err := runner.Run(tmuxPath, "-f", "/dev/null", "-L", info.SocketName, "has-session", "-t", info.SessionName)
	return err == nil
}

func (r ExecRunner) Run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdin = r.Stdin
	command.Stdout = r.Stdout
	command.Stderr = r.Stderr
	return command.Run()
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
