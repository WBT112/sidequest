package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/WBT112/sidequest/internal/preflight"
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
	TerminalSize  TerminalSizeReader
}

type TerminalSizeReader func() (columns int, rows int, err error)

type BossState struct {
	Hidden            bool
	PreviousGameFocus bool
}

type Info struct {
	SocketName  string
	SessionName string
}

const commandPaneHistoryLimit = 100000
const commandPanePreviewLines = 20

const (
	defaultGamePaneHeight = 16
	// 24 arena rows plus HUD/top offset and the bottom wall.
	gamePaneMaxHeight    = 30
	commandPaneMinHeight = 6
)

const (
	bossHiddenOption    = "@sidequest_boss_hidden"
	bossPrevGameOption  = "@sidequest_boss_prev_game"
	gamePausedOption    = "@sidequest_game_paused"
	commandStatusOption = "@sidequest_command_status"
	commandExitOption   = "@sidequest_command_exit"
	bossContinueMessage = "F9 Continue"
)

const (
	paneBorderStatus      = "top"
	paneBorderLines       = "single"
	paneBorderStyle       = "fg=colour244"
	paneActiveBorderStyle = "fg=cyan,bold"
	monoPaneBorderStyle   = "default"
	monoActiveBorderStyle = "bold,reverse"
	commandPaneTitle      = "COMMAND"
	gamePaneTitle         = "SNAKE"
	remainOnExitFormat    = "Command finished - F12 Snake - F10 Shell"
	fullTitleMinWidth     = 72
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
	runOptional := func(args ...string) {
		_ = run(args...)
	}
	ui := uiPresetForSession(runtimeSession)

	terminalRows := 0
	newSessionArgs := []string{"new-session", "-d"}
	if columns, rows, err := l.currentTerminalSize(); err == nil && columns > 0 && rows > 0 {
		newSessionArgs = append(newSessionArgs, "-x", strconv.Itoa(columns), "-y", strconv.Itoa(rows))
		terminalRows = rows
	}
	newSessionArgs = append(newSessionArgs, "-s", info.SessionName, "-n", "sidequest", shellJoin(commandRunner))
	if err := run(newSessionArgs...); err != nil {
		return Info{}, fmt.Errorf("create tmux session: %w", err)
	}

	cleanup := func(original error) (Info, error) {
		_ = run("kill-session", "-t", info.SessionName)
		return Info{}, original
	}

	if err := run("set-option", "-t", info.SessionName, "status", "off"); err != nil {
		return cleanup(fmt.Errorf("disable tmux status bar: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "prefix", "None"); err != nil {
		return cleanup(fmt.Errorf("disable tmux prefix: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "prefix2", "None"); err != nil {
		return cleanup(fmt.Errorf("disable tmux secondary prefix: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "history-limit", fmt.Sprintf("%d", commandPaneHistoryLimit)); err != nil {
		return cleanup(fmt.Errorf("set command pane history limit: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-border-status", paneBorderStatus); err != nil {
		return cleanup(fmt.Errorf("enable pane titles: %w", err))
	}
	runOptional("set-option", "-t", info.SessionName, "pane-border-lines", paneBorderLines)
	if err := run("set-option", "-t", info.SessionName, "pane-border-format", paneBorderFormat(ui)); err != nil {
		return cleanup(fmt.Errorf("configure pane titles: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-border-style", ui.paneBorderStyle()); err != nil {
		return cleanup(fmt.Errorf("configure inactive pane border style: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "pane-active-border-style", ui.paneActiveBorderStyle()); err != nil {
		return cleanup(fmt.Errorf("configure active pane border style: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.0", "-T", commandPaneTitle); err != nil {
		return cleanup(fmt.Errorf("title command pane: %w", err))
	}
	if err := run("set-option", "-q", "-t", info.SessionName, gamePausedOption, "0"); err != nil {
		return cleanup(fmt.Errorf("initialize game pane state: %w", err))
	}
	if err := run("set-option", "-q", "-t", info.SessionName, commandStatusOption, session.StatusRunning); err != nil {
		return cleanup(fmt.Errorf("initialize command pane state: %w", err))
	}
	if err := run("set-option", "-q", "-t", info.SessionName, commandExitOption, ""); err != nil {
		return cleanup(fmt.Errorf("initialize command pane exit code: %w", err))
	}
	if err := run("set-option", "-t", info.SessionName, "remain-on-exit", "on"); err != nil {
		return cleanup(fmt.Errorf("enable command pane remain-on-exit: %w", err))
	}
	runOptional("set-option", "-t", info.SessionName, "remain-on-exit-format", remainOnExitFormat)
	if err := run("split-window", "-v", "-l", strconv.Itoa(gamePaneHeightForWindow(terminalRows)), "-t", info.SessionName+":0.0", shellJoin(gameRunner)); err != nil {
		return cleanup(fmt.Errorf("create placeholder pane: %w", err))
	}
	if err := run("select-pane", "-t", info.SessionName+":0.1", "-T", gamePaneTitle); err != nil {
		return cleanup(fmt.Errorf("title game pane: %w", err))
	}
	if err := run("set-hook", "-t", info.SessionName, "window-resized", gamePaneResizeCommand(tmuxPath, info)); err != nil {
		return cleanup(fmt.Errorf("configure game pane resize hook: %w", err))
	}
	if err := run("bind-key", "-n", "F12", "select-pane", "-t", ":.+"); err != nil {
		return cleanup(fmt.Errorf("bind F12 pane switch: %w", err))
	}
	if err := run("bind-key", "-n", "F10", "detach-client"); err != nil {
		return cleanup(fmt.Errorf("bind F10 detach: %w", err))
	}
	if err := run("bind-key", "-n", "F9", "if-shell", "-F", "#{==:#{"+bossHiddenOption+"},1}", bossRestoreCommand(tmuxPath, info), bossHideCommand(info)); err != nil {
		return cleanup(fmt.Errorf("bind F9 boss key: %w", err))
	}
	for _, binding := range commandPaneScrollBindings() {
		if err := run("bind-key", "-n", binding.key, "if-shell", "-F", "#{==:#{pane_index},0}", binding.command, "send-keys "+binding.key); err != nil {
			return cleanup(fmt.Errorf("bind command pane scroll key %s: %w", binding.key, err))
		}
	}
	if err := run("select-pane", "-t", info.SessionName+":0.1"); err != nil {
		return cleanup(fmt.Errorf("focus game pane: %w", err))
	}

	return info, nil
}

func (l Layout) currentTerminalSize() (columns int, rows int, err error) {
	if l.TerminalSize != nil {
		return l.TerminalSize()
	}
	env := preflight.DefaultEnvironment()
	size, err := env.TerminalSize(env.StdoutFD)
	if err != nil {
		return 0, 0, err
	}
	return size.Columns, size.Rows, nil
}

func gamePaneHeightForWindow(windowHeight int) int {
	if windowHeight <= 0 {
		return defaultGamePaneHeight
	}
	maxAllowed := windowHeight - commandPaneMinHeight
	if maxAllowed < 1 {
		return 1
	}
	if maxAllowed < gamePaneMaxHeight {
		return maxAllowed
	}
	return gamePaneMaxHeight
}

func gamePaneResizeCommand(tmuxPath string, info Info) string {
	shell := fmt.Sprintf(
		"h=#{window_height}; z=#{window_zoomed_flag}; "+
			"case \"$h\" in ''|*[!0-9]*) exit 0;; esac; "+
			"[ \"$z\" = 1 ] && exit 0; "+
			"target=%d; max_for_game=$((h - %d)); "+
			"[ \"$max_for_game\" -lt 1 ] && max_for_game=1; "+
			"[ \"$max_for_game\" -lt \"$target\" ] && target=\"$max_for_game\"; "+
			"%s -f /dev/null -L %s resize-pane -t %s -y \"$target\"",
		gamePaneMaxHeight,
		commandPaneMinHeight,
		shellQuote(tmuxPath),
		shellQuote(info.SocketName),
		shellQuote(info.SessionName+":0.1"),
	)
	return "run-shell -b " + shellQuote(shell)
}

type commandPaneScrollBinding struct {
	key     string
	command string
}

func commandPaneScrollBindings() []commandPaneScrollBinding {
	return []commandPaneScrollBinding{
		{key: "PPage", command: "copy-mode -e -u"},
		{key: "NPage", command: copyModeScrollDownCommand("page-down")},
		{key: "Up", command: "copy-mode -e ; send-keys -X scroll-up"},
		{key: "Down", command: copyModeScrollDownCommand("scroll-down")},
	}
}

func copyModeScrollDownCommand(command string) string {
	return fmt.Sprintf(
		"if-shell -F '#{pane_in_mode}' 'send-keys -X %s ; if-shell -F \"#{==:#{scroll_position},0}\" \"send-keys -X cancel\"' 'display-message -d 1 \"\"'",
		command,
	)
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

func bossRestoreCommand(tmuxPath string, info Info) string {
	return strings.Join([]string{
		fmt.Sprintf("if-shell -F '#{window_zoomed_flag}' 'resize-pane -Z -t %s:0.0' ''", info.SessionName),
		gamePaneResizeCommand(tmuxPath, info),
		fmt.Sprintf("set-option -q -t %s pane-border-status %s", info.SessionName, paneBorderStatus),
		fmt.Sprintf("if-shell -F '#{==:#{%s},1}' 'select-pane -t %s:0.1' 'select-pane -t %s:0.0'", bossPrevGameOption, info.SessionName, info.SessionName),
		fmt.Sprintf("set-option -q -t %s %s 0", info.SessionName, bossHiddenOption),
	}, " ; ")
}

type uiPreset struct {
	NoColor bool
}

func uiPresetForSession(runtimeSession session.Session) uiPreset {
	state, err := session.ReadState(runtimeSession)
	if err != nil {
		return uiPreset{}
	}
	return uiPreset{NoColor: state.NoColor}
}

func (p uiPreset) paneBorderStyle() string {
	if p.NoColor {
		return monoPaneBorderStyle
	}
	return paneBorderStyle
}

func (p uiPreset) paneActiveBorderStyle() string {
	if p.NoColor {
		return monoActiveBorderStyle
	}
	return paneActiveBorderStyle
}

func paneBorderFormat(preset uiPreset) string {
	activeStyle := "#[align=centre]#[fg=cyan]#[bold]#[reverse]"
	inactiveStyle := "#[align=centre]#[fg=colour244]"
	if preset.NoColor {
		activeStyle = "#[align=centre]#[bold]#[reverse]"
		inactiveStyle = "#[default]#[align=centre]"
	}
	return "#{?pane_active," + activeStyle + "> " + paneTitleFormat(true) + "#[default]," + inactiveStyle + paneTitleFormat(false) + "#[default]}"
}

func paneTitleFormat(active bool) string {
	command := tmuxWidthVariant(commandPaneTitleFormat(active, true), commandPaneTitleFormat(active, false))
	game := tmuxWidthVariant(gamePaneTitleFormat(active, true), gamePaneTitleFormat(active, false))
	return "#{?#{==:#{pane_index},0}," + command + "," + game + "}"
}

func tmuxWidthVariant(full string, compact string) string {
	return fmt.Sprintf("#{?#{>=:#{pane_width},%d},%s,%s}", fullTitleMinWidth, full, compact)
}

func commandPaneTitleVariant(active bool, width int) string {
	return commandPaneTitleVariantFor(active, width, session.StatusRunning, nil)
}

func commandPaneTitleVariantFor(active bool, width int, status string, exitCode *int) string {
	if width < fullTitleMinWidth {
		return "COMMAND - F12 Snake - F10 Shell"
	}
	if session.IsTerminalStatus(status) {
		return "COMMAND - " + commandResultLabel(status, exitCode) + " - F12 Snake - F10 Shell"
	}
	state := "RUNNING"
	if active {
		state = "INPUT"
	}
	return "COMMAND - " + state + " - F12 Snake - PgUp/PgDn Scroll - F10 Shell"
}

func commandPaneTitleFormat(active bool, full bool) string {
	if !full {
		return commandPaneTitleVariantFor(active, fullTitleMinWidth-1, session.StatusRunning, nil)
	}
	live := commandPaneTitleVariantFor(active, fullTitleMinWidth, session.StatusRunning, nil)
	return "#{?pane_dead,COMMAND - " + commandDeadResultFormat() + " - F12 Snake - F10 Shell," + live + "}"
}

func commandDeadResultFormat() string {
	status := "#{" + commandStatusOption + "}"
	exitCode := "#{" + commandExitOption + "}"
	fallbackStatus := "#{pane_dead_status}"
	fallback := "#{?#{==:" + fallbackStatus + ",0},DONE - EXIT 0," +
		"#{?#{==:" + fallbackStatus + ",126},START FAILED," +
		"#{?#{==:" + fallbackStatus + ",127},START FAILED," +
		"#{?#{==:" + fallbackStatus + ",130},INTERRUPTED,FAILED - EXIT " + fallbackStatus + "}}}}"
	return "#{?#{==:" + status + "," + session.StatusCompleted + "},DONE - EXIT " + exitCode + "," +
		"#{?#{==:" + status + "," + session.StatusFailed + "},FAILED - EXIT " + exitCode + "," +
		"#{?#{==:" + status + "," + session.StatusInterrupted + "},INTERRUPTED," +
		"#{?#{==:" + status + "," + session.StatusStartFailed + "},START FAILED," + fallback + "}}}}"
}

func commandResultLabel(status string, exitCode *int) string {
	switch status {
	case session.StatusCompleted:
		if exitCode != nil {
			return fmt.Sprintf("DONE - EXIT %d", *exitCode)
		}
		return "DONE"
	case session.StatusFailed:
		if exitCode != nil {
			return fmt.Sprintf("FAILED - EXIT %d", *exitCode)
		}
		return "FAILED"
	case session.StatusInterrupted:
		return "INTERRUPTED"
	case session.StatusStartFailed:
		return "START FAILED"
	default:
		return strings.ToUpper(status)
	}
}

func gamePaneTitleVariant(active bool, width int) string {
	return gamePaneTitleVariantFor(active, width, false)
}

func gamePaneTitleVariantFor(active bool, width int, paused bool) string {
	if width < fullTitleMinWidth {
		return "SNAKE - F12 Command - P Pause - F10 Shell"
	}
	state := "PAUSED"
	if active && !paused {
		state = "ACTIVE"
	}
	return "SNAKE - " + state + " - F12 Command - P Pause - F10 Shell"
}

func gamePaneTitleFormat(active bool, full bool) string {
	if !full {
		return gamePaneTitleVariantFor(active, fullTitleMinWidth-1, false)
	}
	if !active {
		return gamePaneTitleVariantFor(false, fullTitleMinWidth, false)
	}
	return "SNAKE - #{?#{==:#{@" + strings.TrimPrefix(gamePausedOption, "@") + "},1},PAUSED,ACTIVE} - F12 Command - P Pause - F10 Shell"
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

func (l Layout) SetGamePaused(info Info, paused bool) error {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}

	value := "0"
	if paused {
		value = "1"
	}
	if err := runner.Run(tmuxPath, "-f", "/dev/null", "-L", info.SocketName, "set-option", "-q", "-t", info.SessionName, gamePausedOption, value); err != nil {
		return fmt.Errorf("update game pane state: %w", err)
	}

	return nil
}

func (l Layout) SetCommandState(info Info, state session.State) error {
	tmuxPath := l.TmuxPath
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	runner := l.CommandRunner
	if runner == nil {
		runner = quietRunner()
	}

	exitCode := ""
	if state.ExitCode != nil {
		exitCode = fmt.Sprintf("%d", *state.ExitCode)
	}
	baseArgs := []string{"-f", "/dev/null", "-L", info.SocketName, "set-option", "-q", "-t", info.SessionName}
	if err := runner.Run(tmuxPath, append(baseArgs, commandStatusOption, state.Status)...); err != nil {
		return fmt.Errorf("update command pane state: %w", err)
	}
	if err := runner.Run(tmuxPath, append(baseArgs, commandExitOption, exitCode)...); err != nil {
		return fmt.Errorf("update command pane exit code: %w", err)
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

func (l Layout) CaptureCommandPreview(info Info) (string, error) {
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
		return "", fmt.Errorf("tmux runner cannot capture output")
	}

	output, err := outputRunner.Output(
		tmuxPath,
		"-f", "/dev/null",
		"-L", info.SocketName,
		"capture-pane",
		"-p",
		"-J",
		"-S", fmt.Sprintf("-%d", commandPanePreviewLines),
		"-t", info.SessionName+":0.0",
	)
	if err != nil {
		return "", fmt.Errorf("capture command preview: %w", err)
	}
	return lastCommandPreviewLine(string(output)), nil
}

func lastCommandPreviewLine(output string) string {
	lines := strings.Split(output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			return line
		}
	}
	return ""
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
