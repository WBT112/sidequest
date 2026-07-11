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

	info, err := layout.Start(runtimeSession, []string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath})
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
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "set-option", "-t", "sidequest-abc123", "remain-on-exit", "on"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "split-window", "-v", "-l", "10", "-t", "sidequest-abc123:0.0"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "bind-key", "-n", "F12", "select-pane", "-t", ":.+"},
		{"tmux", "-f", "/dev/null", "-L", "sidequest-abc123", "select-pane", "-t", "sidequest-abc123:0.0"},
	}

	if len(runner.calls) != len(wantPrefixes) {
		t.Fatalf("recorded %d tmux calls, want %d:\n%#v", len(runner.calls), len(wantPrefixes), runner.calls)
	}
	for index, want := range wantPrefixes {
		if !hasPrefix(runner.calls[index], want) {
			t.Fatalf("call %d = %#v, want prefix %#v", index, runner.calls[index], want)
		}
	}
}

func TestStartDoesNotPutUserCommandInTmuxCommands(t *testing.T) {
	runner := &recordingRunner{}
	layout := Layout{CommandRunner: runner}
	runtimeSession := session.Session{ID: "no-secret", SocketPath: "/tmp/sidequest-1000/no-secret/command.sock"}

	_, err := layout.Start(runtimeSession, []string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath})
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

	_, err := layout.Start(runtimeSession, []string{"/usr/bin/sidequest", "__sidequest-command-runner", runtimeSession.SocketPath})
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
	calls  [][]string
	failAt int
}

func (r *recordingRunner) Run(name string, args ...string) error {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if r.failAt > 0 && len(r.calls) == r.failAt {
		return errors.New("tmux failed")
	}
	return nil
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
