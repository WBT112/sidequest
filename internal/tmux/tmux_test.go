package tmux

import (
	"errors"
	"strings"
	"testing"

	"github.com/WBT112/sidequest/internal/session"
)

func TestStartCreatesIsolatedLayout(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "abc123", SocketPath: "/run/user/1000/sidequest/abc123/command.sock"}

	info, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/run/user/1000/sidequest/abc123/state.json"},
	)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if info.SocketName != "sidequest-abc123" {
		t.Fatalf("SocketName = %q, want %q", info.SocketName, "sidequest-abc123")
	}
	if info.SessionName != "sidequest-abc123" {
		t.Fatalf("SessionName = %q, want %q", info.SessionName, "sidequest-abc123")
	}

	wantPrefixes := [][]string{
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "new-session", "-d", "-s", "sidequest-abc123", "-n", "sidequest"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "status", "off"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "history-limit", "100000"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "pane-border-status", "top"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "pane-border-format"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "pane-border-style", "fg=colour244"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "pane-active-border-style", "fg=brightwhite,bold"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "select-pane", "-t", "sidequest-abc123:0.0", "-T", "Command - PgUp/PgDn scroll, F12 Snake, F10 shell"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "remain-on-exit", "on"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "split-window", "-v", "-l", "16", "-t", "sidequest-abc123:0.0"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "select-pane", "-t", "sidequest-abc123:0.1", "-T", "Snake - arrows/WASD, F9 hide, F12 Command, F10 shell"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "F12", "select-pane", "-t", ":.+"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "F10", "detach-client"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "F9", "if-shell", "-F", "#{==:#{@sidequest_boss_hidden},1}"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "PPage", "if-shell", "-F", "#{==:#{pane_index},0}"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "NPage", "if-shell", "-F", "#{==:#{pane_index},0}"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "Up", "if-shell", "-F", "#{==:#{pane_index},0}"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "Down", "if-shell", "-F", "#{==:#{pane_index},0}"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "select-pane", "-t", "sidequest-abc123:0.1"},
	}

	if len(runner.calls) != len(wantPrefixes) {
		t.Fatalf("recorded %d tmux calls, want %d:\n%#v", len(runner.calls), len(wantPrefixes), runner.calls)
	}
	for index, want := range wantPrefixes {
		if !hasPrefix(runner.calls[index], want) {
			t.Fatalf("call %d = %#v, want prefix %#v", index, runner.calls[index], want)
		}
	}
	splitCall := runner.calls[9]
	splitCommand := splitCall[len(splitCall)-1]
	if !strings.Contains(splitCommand, "__sidequest-game") {
		t.Fatalf("split command = %q, want game runner", splitCommand)
	}
}

func TestStartConfiguresEnhancedPaneFocusFormatting(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "focus", SocketPath: "/tmp/sidequest-1000/focus/command.sock"}

	if _, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/tmp/sidequest-1000/focus/state.json"},
	); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	joined := runner.joinedCalls()
	for _, want := range []string{
		"pane-border-status top",
		"pane-border-style fg=colour244",
		"pane-active-border-style fg=brightwhite,bold",
		"▶ #{pane_title}",
		"INPUT ACTIVE",
		"CONTROLS ACTIVE",
		"RUNNING",
		"PAUSED",
		"#{?pane_active",
		"#{pane_index}",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("enhanced focus formatting missing %q:\n%s", want, joined)
		}
	}
}

func TestStartPreservesPaneTitleStrings(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "titles", SocketPath: "/tmp/sidequest-1000/titles/command.sock"}

	if _, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/tmp/sidequest-1000/titles/state.json"},
	); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	joined := runner.joinedCalls()
	for _, title := range []string{
		"Command - PgUp/PgDn scroll, F12 Snake, F10 shell",
		"Snake - arrows/WASD, F9 hide, F12 Command, F10 shell",
	} {
		if !strings.Contains(joined, title) {
			t.Fatalf("pane title %q was not preserved:\n%s", title, joined)
		}
	}
}

func TestStartBindsCommandPaneScrollKeysOnlyForCommandPane(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "scroll", SocketPath: "/tmp/sidequest-1000/scroll/command.sock"}

	if _, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/tmp/sidequest-1000/scroll/state.json"},
	); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	joined := runner.joinedCalls()
	for _, want := range []string{
		"bind-key -n PPage if-shell -F #{==:#{pane_index},0} copy-mode -e -u send-keys PPage",
		"bind-key -n NPage if-shell -F #{==:#{pane_index},0} copy-mode -e ; send-keys -X page-down send-keys NPage",
		"bind-key -n Up if-shell -F #{==:#{pane_index},0} copy-mode -e ; send-keys -X cursor-up send-keys Up",
		"bind-key -n Down if-shell -F #{==:#{pane_index},0} copy-mode -e ; send-keys -X cursor-down send-keys Down",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("scroll key binding missing %q:\n%s", want, joined)
		}
	}
}

func TestStartDoesNotPutUserCommandInTmuxCommands(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "no-secret", SocketPath: "/tmp/sidequest-1000/no-secret/command.sock"}

	_, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/tmp/sidequest-1000/no-secret/state.json"},
	)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	allCalls := runner.joinedCalls()
	for _, forbidden := range []string{"sleep 30", "exit 7", "bash -c"} {
		if strings.Contains(allCalls, forbidden) {
			t.Fatalf("tmux calls contain user command %q:\n%s", forbidden, allCalls)
		}
	}
}

func TestStartCentersGamePaneTitleWhenWidthAvailable(t *testing.T) {
	runner := &recordingRunner{output: "100\n"}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "abc123", SocketPath: "/run/user/1000/sidequest/abc123/command.sock"}

	_, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/run/user/1000/sidequest/abc123/state.json"},
	)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if len(runner.outputCalls) != 1 {
		t.Fatalf("output calls = %d, want 1", len(runner.outputCalls))
	}
	wantOutputCall := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "display-message", "-p", "-t", "sidequest-abc123:0.1", "#{pane_width}"}
	if !equalStrings(runner.outputCalls[0], wantOutputCall) {
		t.Fatalf("output call = %#v, want %#v", runner.outputCalls[0], wantOutputCall)
	}

	const baseTitle = "Snake - arrows/WASD, F9 hide, F12 Command, F10 shell"
	title := ""
	for _, call := range runner.calls {
		if len(call) >= 5 && call[len(call)-3] == "sidequest-abc123:0.1" && call[len(call)-2] == "-T" {
			title = call[len(call)-1]
			break
		}
	}
	if title == "" {
		t.Fatalf("game pane title call not found:\n%#v", runner.calls)
	}
	if !strings.HasSuffix(title, baseTitle) {
		t.Fatalf("game pane title = %q, want suffix %q", title, baseTitle)
	}
	if len(title) <= len(baseTitle) || !strings.HasPrefix(title, " ") {
		t.Fatalf("game pane title = %q, want centered title with leading padding", title)
	}
}

func TestAttachUsesIsolatedServer(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	if err := layout.Attach(info); err != nil {
		t.Fatalf("Attach returned error: %v", err)
	}

	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "attach-session", "-t", "sidequest-abc123"}
	if !equalStrings(runner.calls[0], want) {
		t.Fatalf("attach call = %#v, want %#v", runner.calls[0], want)
	}
}

func TestDefaultAttachRunnerUsesTerminalStdio(t *testing.T) {
	runner := attachRunner()

	if runner.Stdin == nil || runner.Stdout == nil || runner.Stderr == nil {
		t.Fatalf("attachRunner = %#v, want terminal stdio configured", runner)
	}
}

func TestQuietRunnerDoesNotUseTerminalStdio(t *testing.T) {
	runner := quietRunner()

	if runner.Stdin != nil || runner.Stdout != nil || runner.Stderr != nil {
		t.Fatalf("quietRunner = %#v, want no terminal stdio", runner)
	}
}

func TestCloseKillsOnlyIsolatedSession(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	if err := layout.Close(info); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "kill-session", "-t", "sidequest-abc123"}
	if !equalStrings(runner.calls[0], want) {
		t.Fatalf("close call = %#v, want %#v", runner.calls[0], want)
	}
}

func TestDetachClientsUsesIsolatedSession(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	if err := layout.DetachClients(info); err != nil {
		t.Fatalf("DetachClients returned error: %v", err)
	}

	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "detach-client", "-s", "sidequest-abc123"}
	if !equalStrings(runner.calls[0], want) {
		t.Fatalf("detach call = %#v, want %#v", runner.calls[0], want)
	}
}

func TestCaptureCommandPaneReadsPlainCommandPaneOutput(t *testing.T) {
	runner := &recordingRunner{output: "line one\nline two\n"}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	output, truncated, err := layout.CaptureCommandPane(info)
	if err != nil {
		t.Fatalf("CaptureCommandPane returned error: %v", err)
	}
	if output != "line one\nline two\n" {
		t.Fatalf("output = %q", output)
	}
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "capture-pane", "-p", "-J", "-S", "-100000", "-t", "sidequest-abc123:0.0"}
	if !equalStrings(runner.outputCalls[0], want) {
		t.Fatalf("capture call = %#v, want %#v", runner.outputCalls[0], want)
	}
}

func TestGamePaneActiveReadsOwnedGamePane(t *testing.T) {
	runner := &recordingRunner{output: "1\n"}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	active, err := layout.GamePaneActive(info)
	if err != nil {
		t.Fatalf("GamePaneActive returned error: %v", err)
	}
	if !active {
		t.Fatal("GamePaneActive = false, want true")
	}
	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "display-message", "-p", "-t", "sidequest-abc123:0.1", "#{pane_active}"}
	if !equalStrings(runner.outputCalls[0], want) {
		t.Fatalf("focus call = %#v, want %#v", runner.outputCalls[0], want)
	}
}

func TestStartBindsBossKeyToOwnedSession(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "boss", SocketPath: "/tmp/sidequest-1000/boss/command.sock"}

	if _, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/tmp/sidequest-1000/boss/state.json"},
	); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	joined := runner.joinedCalls()
	for _, want := range []string{
		"bind-key -n F9",
		"@sidequest_boss_hidden",
		"@sidequest_boss_prev_game",
		"pane-border-status off",
		"pane-border-status top",
		"pane-border-format",
		"resize-pane -Z -t sidequest-boss:0.0",
		"select-pane -t sidequest-boss:0.1",
		"F9 Continue",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("boss key binding missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "kill-pane") {
		t.Fatalf("boss key binding kills panes:\n%s", joined)
	}
}

func TestBossStateReadsOwnedSessionOptionsAndZoom(t *testing.T) {
	runner := &recordingRunner{output: "1 1 1\n"}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	state, err := layout.BossState(info)
	if err != nil {
		t.Fatalf("BossState returned error: %v", err)
	}
	if !state.Hidden || !state.PreviousGameFocus {
		t.Fatalf("BossState = %#v, want hidden with previous game focus", state)
	}
	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "display-message", "-p", "-t", "sidequest-abc123:0", "#{@sidequest_boss_hidden} #{@sidequest_boss_prev_game} #{window_zoomed_flag}"}
	if !equalStrings(runner.outputCalls[0], want) {
		t.Fatalf("boss state call = %#v, want %#v", runner.outputCalls[0], want)
	}
}

func TestBossStateReconcilesStaleHiddenOptionWithoutZoom(t *testing.T) {
	runner := &recordingRunner{output: "1 1 0\n"}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	state, err := layout.BossState(info)
	if err != nil {
		t.Fatalf("BossState returned error: %v", err)
	}
	if state.Hidden {
		t.Fatalf("BossState hidden = true for stale unzoomed state: %#v", state)
	}
}

func TestHasSessionUsesIsolatedServer(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	info := Info{SocketName: "sidequest-abc123", SessionName: "sidequest-abc123"}

	if !layout.HasSession(info) {
		t.Fatal("HasSession returned false, want true")
	}

	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "has-session", "-t", "sidequest-abc123"}
	if !equalStrings(runner.calls[0], want) {
		t.Fatalf("has-session call = %#v, want %#v", runner.calls[0], want)
	}
}

func TestStartKillsSessionWhenSetupFails(t *testing.T) {
	runner := &recordingRunner{failAt: 2}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "cleanup", SocketPath: "/tmp/sidequest-1000/cleanup/command.sock"}

	_, err := layout.Start(
		runtimeSession,
		[]string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath},
		[]string{"/usr/bin/sidequest", "__sidequest-game", "/tmp/sidequest-1000/cleanup/state.json"},
	)
	if err == nil {
		t.Fatal("Start succeeded, want error")
	}

	lastCall := runner.calls[len(runner.calls)-1]
	want := []string{"tmux", "-f", "/dev/null", "-L", "sidequest-cleanup", "kill-session", "-t", "sidequest-cleanup"}
	if !equalStrings(lastCall, want) {
		t.Fatalf("last call = %#v, want cleanup call %#v", lastCall, want)
	}
}

func TestShellJoinQuotesRunnerCommand(t *testing.T) {
	got := shellJoin([]string{"/tmp/side quest", "__runner", "/tmp/has'quote.sock"})
	want := "'/tmp/side quest' '__runner' '/tmp/has'\"'\"'quote.sock'"
	if got != want {
		t.Fatalf("shellJoin = %q, want %q", got, want)
	}
}

type recordingRunner struct {
	calls       [][]string
	outputCalls [][]string
	output      string
	failAt      int
}

func (r *recordingRunner) Run(name string, args ...string) error {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if r.failAt > 0 && len(r.calls) == r.failAt {
		return errors.New("tmux failed")
	}
	return nil
}

func (r *recordingRunner) Output(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.outputCalls = append(r.outputCalls, call)
	return []byte(r.output), nil
}

func (r *recordingRunner) joinedCalls() string {
	var builder strings.Builder
	for _, call := range r.calls {
		builder.WriteString(strings.Join(call, " "))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func hasPrefix(values, prefix []string) bool {
	if len(values) < len(prefix) {
		return false
	}
	for index := range prefix {
		if values[index] != prefix[index] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
