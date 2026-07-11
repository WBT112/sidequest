package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/WBT112/sidequest/internal/session"
)

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

func (l Layout) Start(runtimeSession session.Session, commandRunner []string, gameRunner []string) (Info, error) {
	if len(commandRunner) == 0 {
		return Info{}, fmt.Errorf("missing command runner")
	}
	if len(gameRunner) == 0 {
		return Info{}, fmt.Errorf("missing game runner")
	}

	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
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
	if err := run("set-option", "-t", info.SessionName, "pane-border-status", "top"); err != nil {
		return cleanup(fmt.Errorf("enable pane titles: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-border-format", "#{pane_title}"); err != nil {
		return cleanup(fmt.Errorf("configure pane titles: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.0", "-T", "Command - F12 Snake, F10 shell"); err != nil {
		return cleanup(fmt.Errorf("title command pane: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "remain-on-exit", "on"); err != nil {
		return cleanup(fmt.Errorf("enable command pane remain-on-exit: %w", err))
	}
	if err := run("split-window", "-v", "-l", "16", "-t", info.SessionName+":0.0", shellJoin(gameRunner)); err != nil {
		return cleanup(fmt.Errorf("create placeholder pane: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.1", "-T", "Snake - arrows/WASD, R restart, F12 back, F10 shell"); err != nil {
		return cleanup(fmt.Errorf("title game pane: %w", err))
	}
	if err := run("bind-key", "-n", "F12", "select-pane", "-t", ":.+"); err != nil {
		return cleanup(fmt.Errorf("bind F12 pane switch: %w", err))
	}
	if err := run("bind-key", "-n", "F10", "detach-client"); err != nil {
		return cleanup(fmt.Errorf("bind F10 detach: %w", err))
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
		runner = attachRunner()
	}

	if err := runner.Run(tmuxPath, "-f", "/dev/null", "-L", info.SocketName, "attach-session", "-t", info.SessionName); err != nil {
		return fmt.Errorf("attach tmux session: %w", err)
	}

	return nil
}

func (l Layout) Close(info Info) error {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}

	if err := runner.Run(tmuxPath, "-f", "/dev/null", "-L", info.SocketName, "kill-session", "-t", info.SessionName); err != nil {
		return fmt.Errorf("close tmux session: %w", err)
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
		runner = quietRunner()
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

func attachRunner() ExecRunner {
	return ExecRunner{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
}

func quietRunner() ExecRunner {
	return ExecRunner{}
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
