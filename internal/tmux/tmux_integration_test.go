package tmux

import (
	"os/exec"
	"testing"

	"github.com/WBT112/sidequest/internal/session"
)

func TestIntegrationStartAndCloseIsolatedTmuxSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed; skipping tmux integration test")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not installed; skipping tmux integration test")
	}

	layout := Layout{}
	first := session.Session{ID: "qa-one", SocketPath: "/tmp/sidequest-qa/qa-one/command.sock"}
	second := session.Session{ID: "qa-two", SocketPath: "/tmp/sidequest-qa/qa-two/command.sock"}
	command := []string{"sh", "-c", "sleep 30"}
	game := []string{"sh", "-c", "sleep 30"}

	firstInfo, err := layout.Start(first, command, game)
	if err != nil {
		t.Fatalf("Start first returned error: %v", err)
	}
	defer func() { _ = layout.Close(firstInfo) }()

	secondInfo, err := layout.Start(second, command, game)
	if err != nil {
		t.Fatalf("Start second returned error: %v", err)
	}
	defer func() { _ = layout.Close(secondInfo) }()

	if firstInfo.SocketName == secondInfo.SocketName {
		t.Fatalf("sessions share tmux socket %q", firstInfo.SocketName)
	}
	if !layout.HasSession(firstInfo) {
		t.Fatalf("first tmux session is not running")
	}
	if !layout.HasSession(secondInfo) {
		t.Fatalf("second tmux session is not running")
	}

	if err := layout.Close(firstInfo); err != nil {
		t.Fatalf("Close first returned error: %v", err)
	}
	if layout.HasSession(firstInfo) {
		t.Fatalf("first tmux session still exists after close")
	}
	if !layout.HasSession(secondInfo) {
		t.Fatalf("closing first tmux session affected second session")
	}
}
