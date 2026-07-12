package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/WBT112/sidequest/internal/session"
)

type Runner interface {
	Run(name string, args ...string) error
}

type OutputRunner interface {
	Output(name string, args ...string) ([]byte, error)
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

type BossState struct {
	Hidden            bool
	PreviousGameFocus bool
}

type Info struct {
	SocketName  string
	SessionName string
}

const commandPaneHistoryLimit = 100000

const (
	bossHiddenOption    = "@sidequest_boss_hidden"
	bossPrevGameOption  = "@sidequest_boss_prev_game"
	bossContinueMessage = "F9 Continue"
)

const (
	paneBorderStatus      = "top"
	paneBorderStyle       = "fg=colour244"
	paneActiveBorderStyle = "fg=brightwhite,bold"
)

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
	if err := run("set-option", "-t", info.SessionName, "history-limit", fmt.Sprintf("%d", commandPaneHistoryLimit)); err != nil {
		return cleanup(fmt.Errorf("set command pane history limit: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-border-status", paneBorderStatus); err != nil {
		return cleanup(fmt.Errorf("enable pane titles: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-border-format", paneBorderFormat()); err != nil {
		return cleanup(fmt.Errorf("configure pane titles: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-border-style", paneBorderStyle); err != nil {
		return cleanup(fmt.Errorf("configure inactive pane border style: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-active-border-style", paneActiveBorderStyle); err != nil {
		return cleanup(fmt.Errorf("configure active pane border style: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.0", "-T", "Command - F9 hide, F12 Snake, F10 shell"); err != nil {
		return cleanup(fmt.Errorf("title command pane: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "remain-on-exit", "on"); err != nil {
		return cleanup(fmt.Errorf("enable command pane remain-on-exit: %w", err))
	}
	if err := run("split-window", "-v", "-l", "16", "-t", info.SessionName+":0.0", shellJoin(gameRunner)); err != nil {
		return cleanup(fmt.Errorf("create placeholder pane: %w", err))
	}
	gamePaneTitle := "Snake - arrows/WASD, F9 hide, F12 Command, F10 shell"
	if outputRunner, ok := runner.(OutputRunner); ok {
		gamePaneTitle = centeredPaneTitle(outputRunner, tmuxPath, baseArgs, info.SessionName+":0.1", gamePaneTitle)
	}
	if err := run("select-pane", "-t", info.SessionName+":0.1", "-T", gamePaneTitle); err != nil {
		return cleanup(fmt.Errorf("title game pane: %w", err))
	}
	if err := run("bind-key", "-n", "F12", "select-pane", "-t", ":.+"); err != nil {
		return cleanup(fmt.Errorf("bind F12 pane switch: %w", err))
	}
	if err := run("bind-key", "-n", "F10", "detach-client"); err != nil {
		return cleanup(fmt.Errorf("bind F10 detach: %w", err))
	}
	if err := run("bind-key", "-n", "F9", "if-shell", "-F", "#{==:#{"+bossHiddenOption+"},1}", bossRestoreCommand(info), bossHideCommand(info)); err != nil {
		return cleanup(fmt.Errorf("bind F9 boss key: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.1"); err != nil {
		return cleanup(fmt.Errorf("focus game pane: %w", err))
	}

	return info, nil
}

func bossHideCommand(info Info) string {
	return strings.Join([]string{
		fmt.Sprintf("if-shell -F '#{==:#{pane_index},1}' 'set-option -q -t %s %s 1' 'set-option -q -t %s %s 0'", info.SessionName, bossPrevGameOption, info.SessionName, bossPrevGameOption),
		fmt.Sprintf("set-option -q -t %s pane-border-status off", info.SessionName),
		fmt.Sprintf("select-pane -t %s:0.0", info.SessionName),
		fmt.Sprintf("if-shell -F '#{window_zoomed_flag}' '' 'resize-pane -Z -t %s:0.0'", info.SessionName),
		fmt.Sprintf("display-message -d 1500 '%s'", bossContinueMessage),
		fmt.Sprintf("set-option -q -t %s %s 1", info.SessionName, bossHiddenOption),
	}, " ; ")
}

func bossRestoreCommand(info Info) string {
	return strings.Join([]string{
		fmt.Sprintf("if-shell -F '#{window_zoomed_flag}' 'resize-pane -Z -t %s:0.0' ''", info.SessionName),
		fmt.Sprintf("set-option -q -t %s pane-border-status %s", info.SessionName, paneBorderStatus),
		fmt.Sprintf("set-option -q -t %s pane-border-format '%s'", info.SessionName, paneBorderFormat()),
		fmt.Sprintf("if-shell -F '#{==:#{%s},1}' 'select-pane -t %s:0.1' 'select-pane -t %s:0.0'", bossPrevGameOption, info.SessionName, info.SessionName),
		fmt.Sprintf("set-option -q -t %s %s 0", info.SessionName, bossHiddenOption),
	}, " ; ")
}

func paneBorderFormat() string {
	return "#{?pane_active,#[bold]#[reverse] ▶ #{pane_title} - #{?#{==:#{pane_index},0},INPUT ACTIVE,CONTROLS ACTIVE} #[default],#[dim]   #{pane_title} - #{?#{==:#{pane_index},0},RUNNING,PAUSED} #[default]}"
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

func (l Layout) BossState(info Info) (BossState, error) {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}
	outputRunner, ok := runner.(OutputRunner)
	if !ok {
		return BossState{}, fmt.Errorf("tmux runner cannot read boss state")
	}

	output, err := outputRunner.Output(
		tmuxPath,
		"-f", "/dev/null",
		"-L", info.SocketName,
		"display-message",
		"-p",
		"-t", info.SessionName+":0",
		"#{"+bossHiddenOption+"} #{"+bossPrevGameOption+"} #{window_zoomed_flag}",
	)
	if err != nil {
		return BossState{}, fmt.Errorf("read boss state: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 3 {
		return BossState{}, nil
	}
	return BossState{
		Hidden:            fields[0] == "1" && fields[2] == "1",
		PreviousGameFocus: fields[1] == "1",
	}, nil
}

func (l Layout) DetachClients(info Info) error {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}

	if err := runner.Run(tmuxPath, "-f", "/dev/null", "-L", info.SocketName, "detach-client", "-s", info.SessionName); err != nil {
		return fmt.Errorf("detach tmux clients: %w", err)
	}

	return nil
}

func (l Layout) GamePaneActive(info Info) (bool, error) {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}
	outputRunner, ok := runner.(OutputRunner)
	if !ok {
		return false, fmt.Errorf("tmux runner cannot read pane focus")
	}

	output, err := outputRunner.Output(
		tmuxPath,
		"-f", "/dev/null",
		"-L", info.SocketName,
		"display-message",
		"-p",
		"-t", info.SessionName+":0.1",
		"#{pane_active}",
	)
	if err != nil {
		return false, fmt.Errorf("read game pane focus: %w", err)
	}
	return strings.TrimSpace(string(output)) == "1", nil
}

func (l Layout) CaptureCommandPane(info Info) (string, bool, error) {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}
	outputRunner, ok := runner.(OutputRunner)
	if !ok {
		return "", false, fmt.Errorf("tmux runner cannot capture output")
	}

	output, err := outputRunner.Output(
		tmuxPath,
		"-f", "/dev/null",
		"-L", info.SocketName,
		"capture-pane",
		"-p",
		"-J",
		"-S", fmt.Sprintf("-%d", commandPaneHistoryLimit),
		"-t", info.SessionName+":0.0",
	)
	if err != nil {
		return "", false, fmt.Errorf("capture command pane: %w", err)
	}
	text := string(output)
	truncated := strings.Count(text, "\n") >= commandPaneHistoryLimit
	return text, truncated, nil
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

func (r ExecRunner) Output(name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Stdin = r.Stdin
	command.Stderr = r.Stderr
	return command.Output()
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

func centeredPaneTitle(outputRunner OutputRunner, tmuxPath string, baseArgs []string, paneTarget string, title string) string {
	width, err := paneWidth(outputRunner, tmuxPath, baseArgs, paneTarget)
	if err != nil || width <= 0 {
		return title
	}
	titleWidth := len([]rune(title))
	if titleWidth >= width {
		return title
	}
	padding := (width - titleWidth) / 2
	if padding <= 0 {
		return title
	}
	return strings.Repeat(" ", padding) + title
}

func paneWidth(outputRunner OutputRunner, tmuxPath string, baseArgs []string, paneTarget string) (int, error) {
	args := append(append([]string{}, baseArgs...), "display-message", "-p", "-t", paneTarget, "#{pane_width}")
	output, err := outputRunner.Output(tmuxPath, args...)
	if err != nil {
		return 0, err
	}
	width, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}
	return width, nil
}
