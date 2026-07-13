package commandexec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WBT112/sidequest/internal/session"
)

func TestRunRecordsSuccessfulCommand(t *testing.T) {
	runtimeSession := newTestSession(t, "success")
	executor := Executor{Now: fixedClock(
		time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 11, 12, 0, 2, 0, time.UTC),
	)}

	err := executor.Run(runtimeSession, session.Command{Executable: "true"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	state := readState(t, runtimeSession)
	if state.Status != session.StatusCompleted {
		t.Fatalf("Status = %q, want %q", state.Status, session.StatusCompleted)
	}
	if state.ExitCode == nil || *state.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", state.ExitCode)
	}
	if state.DurationMillis == nil || *state.DurationMillis != 2000 {
		t.Fatalf("DurationMillis = %v, want 2000", state.DurationMillis)
	}
}

func TestRunRecordsNonZeroExitCode(t *testing.T) {
	runtimeSession := newTestSession(t, "exit7")
	executor := Executor{Now: fixedClock(time.Now(), time.Now().Add(time.Second))}

	err := executor.Run(runtimeSession, session.Command{Executable: "bash", Arguments: []string{"-c", "exit 7"}})
	if err == nil {
		t.Fatal("Run succeeded, want non-zero exit error")
	}

	state := readState(t, runtimeSession)
	if state.Status != session.StatusFailed {
		t.Fatalf("Status = %q, want %q", state.Status, session.StatusFailed)
	}
	if state.ExitCode == nil || *state.ExitCode != 7 {
		t.Fatalf("ExitCode = %v, want 7", state.ExitCode)
	}
}

func TestRunRecordsStartFailure(t *testing.T) {
	runtimeSession := newTestSession(t, "missing")
	executor := Executor{Now: fixedClock(time.Now(), time.Now().Add(time.Second))}

	err := executor.Run(runtimeSession, session.Command{Executable: "sidequest-command-that-does-not-exist"})
	if err == nil {
		t.Fatal("Run succeeded, want start failure")
	}

	state := readState(t, runtimeSession)
	if state.Status != session.StatusStartFailed {
		t.Fatalf("Status = %q, want %q", state.Status, session.StatusStartFailed)
	}
	if state.StartError == "" {
		t.Fatal("StartError is empty")
	}
	if state.Status == session.StatusRunning {
		t.Fatal("Status was set to running before process start succeeded")
	}
}

func TestRunReportsStartedAfterStartupGrace(t *testing.T) {
	runtimeSession := newTestSession(t, "started")
	reporter := &startupRecorder{}
	executor := Executor{StartupGrace: 10 * time.Millisecond, Now: fixedClock(
		time.Now(),
		time.Now().Add(250*time.Millisecond),
	)}

	err := executor.RunWithStartupReporter(runtimeSession, session.Command{
		Executable: "bash",
		Arguments:  []string{"-c", "sleep 0.05"},
	}, reporter)
	if err != nil {
		t.Fatalf("RunWithStartupReporter returned error: %v", err)
	}
	if reporter.startup.Status != session.CommandStartupStarted {
		t.Fatalf("startup status = %#v, want started", reporter.startup)
	}
}

func TestRunReportsImmediateZeroExitDuringStartupGrace(t *testing.T) {
	runtimeSession := newTestSession(t, "immediate-zero")
	reporter := &startupRecorder{}
	executor := Executor{StartupGrace: 100 * time.Millisecond, Now: fixedClock(time.Now(), time.Now().Add(time.Millisecond))}

	err := executor.RunWithStartupReporter(runtimeSession, session.Command{Executable: "true"}, reporter)
	if err != nil {
		t.Fatalf("RunWithStartupReporter returned error: %v", err)
	}
	if reporter.startup.Status != session.CommandStartupCompleted {
		t.Fatalf("startup status = %#v, want completed", reporter.startup)
	}
	if reporter.startup.ExitCode == nil || *reporter.startup.ExitCode != 0 {
		t.Fatalf("startup exit code = %v, want 0", reporter.startup.ExitCode)
	}
}

func TestRunReportsImmediateNonZeroExitDuringStartupGrace(t *testing.T) {
	runtimeSession := newTestSession(t, "immediate-fail")
	reporter := &startupRecorder{}
	executor := Executor{StartupGrace: 100 * time.Millisecond, Now: fixedClock(time.Now(), time.Now().Add(time.Millisecond))}

	err := executor.RunWithStartupReporter(runtimeSession, session.Command{
		Executable: "bash",
		Arguments:  []string{"-c", "exit 7"},
	}, reporter)
	if err == nil {
		t.Fatal("RunWithStartupReporter succeeded, want exit error")
	}
	if reporter.startup.Status != session.CommandStartupFailed {
		t.Fatalf("startup status = %#v, want failed", reporter.startup)
	}
	if reporter.startup.ExitCode == nil || *reporter.startup.ExitCode != 7 {
		t.Fatalf("startup exit code = %v, want 7", reporter.startup.ExitCode)
	}
}

func TestRunReportsBadShebangStartFailure(t *testing.T) {
	runtimeSession := newTestSession(t, "bad-shebang")
	script := filepath.Join(t.TempDir(), "bad-shebang")
	if err := os.WriteFile(script, []byte("#!/definitely/missing/sidequest-interpreter\n"), 0o700); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	reporter := &startupRecorder{}
	executor := Executor{StartupGrace: 100 * time.Millisecond, Now: fixedClock(time.Now(), time.Now().Add(time.Millisecond))}

	err := executor.RunWithStartupReporter(runtimeSession, session.Command{Executable: script}, reporter)
	if err == nil {
		t.Fatal("RunWithStartupReporter succeeded, want start failure")
	}
	if reporter.startup.Status != session.CommandStartupStartFailed {
		t.Fatalf("startup status = %#v, want start_failed", reporter.startup)
	}
	if reporter.startup.Error == "" {
		t.Fatal("startup error is empty")
	}
}

func TestRunPreservesArgumentsWithSpaces(t *testing.T) {
	runtimeSession := newTestSession(t, "spaces")
	var stdout bytes.Buffer
	executor := Executor{Stdout: &stdout, Now: fixedClock(time.Now(), time.Now().Add(time.Second))}

	err := executor.Run(runtimeSession, session.Command{
		Executable: "printf",
		Arguments:  []string{"%s", "value with spaces"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got, want := stdout.String(), "value with spaces"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunDoesNotWriteCommandOutputOrArgumentsToState(t *testing.T) {
	runtimeSession := newTestSession(t, "no-output")
	var stdout bytes.Buffer
	executor := Executor{Stdout: &stdout, Now: fixedClock(time.Now(), time.Now().Add(time.Second))}

	err := executor.Run(runtimeSession, session.Command{
		Executable: "printf",
		Arguments:  []string{"sensitive-output"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	data, err := os.ReadFile(runtimeSession.StatePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	for _, forbidden := range []string{"printf", "sensitive-output"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("state contains command data %q:\n%s", forbidden, data)
		}
	}
}

func TestRunRecordsSignalTermination(t *testing.T) {
	runtimeSession := newTestSession(t, "signal")
	executor := Executor{Now: fixedClock(time.Now(), time.Now().Add(time.Second))}

	err := executor.Run(runtimeSession, session.Command{
		Executable: "bash",
		Arguments:  []string{"-c", "kill -TERM $$"},
	})
	if err == nil {
		t.Fatal("Run succeeded, want signal termination error")
	}

	state := readState(t, runtimeSession)
	if state.Status != session.StatusInterrupted {
		t.Fatalf("Status = %q, want %q", state.Status, session.StatusInterrupted)
	}
	if state.ExitSignal != "terminated" {
		t.Fatalf("ExitSignal = %q, want %q", state.ExitSignal, "terminated")
	}
}

func newTestSession(t *testing.T, id string) session.Session {
	t.Helper()
	manager := session.Manager{
		BaseDir:     filepath.Join(t.TempDir(), "sidequest"),
		IDGenerator: func() (string, error) { return id, nil },
		Now:         time.Now,
	}
	runtimeSession, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	return runtimeSession
}

func readState(t *testing.T, runtimeSession session.Session) session.State {
	t.Helper()
	state, err := session.ReadState(runtimeSession)
	if err != nil {
		t.Fatalf("ReadState returned error: %v", err)
	}
	return state
}

func fixedClock(times ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		if index >= len(times) {
			return times[len(times)-1]
		}
		value := times[index]
		index++
		return value
	}
}

type startupRecorder struct {
	startup session.CommandStartup
}

func (r *startupRecorder) ReportStartup(startup session.CommandStartup) error {
	r.startup = startup
	return nil
}
