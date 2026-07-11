package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WBT112/sidequest/internal/session"
	"github.com/WBT112/sidequest/internal/tmux"
)

func TestParseCommandAfterSeparator(t *testing.T) {
	result, err := Parse([]string{"--", "sleep", "1"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if result.Config.Executable != "sleep" {
		t.Fatalf("Executable = %q, want %q", result.Config.Executable, "sleep")
	}

	if got, want := result.Config.Arguments, []string{"1"}; !equalSlices(got, want) {
		t.Fatalf("Arguments = %#v, want %#v", got, want)
	}
}

func TestParsePreservesShellSyntaxAsArguments(t *testing.T) {
	result, err := Parse([]string{"--", "printf", "%s\n", "|", ">", "*.go"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	want := []string{"%s\n", "|", ">", "*.go"}
	if !equalSlices(result.Config.Arguments, want) {
		t.Fatalf("Arguments = %#v, want %#v", result.Config.Arguments, want)
	}
}

func TestParseRejectsMissingCommandAfterSeparator(t *testing.T) {
	_, err := Parse([]string{"--"})
	if !errors.Is(err, ErrMissingCommand) {
		t.Fatalf("Parse error = %v, want %v", err, ErrMissingCommand)
	}
}

func TestParseRejectsMissingSeparator(t *testing.T) {
	_, err := Parse([]string{"sleep", "1"})
	if !errors.Is(err, ErrMissingSeparator) {
		t.Fatalf("Parse error = %v, want %v", err, ErrMissingSeparator)
	}
}

func TestParseHelp(t *testing.T) {
	result, err := Parse([]string{"--help"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !result.ShowHelp {
		t.Fatal("ShowHelp = false, want true")
	}
}

func TestRunHelpDocumentsSeparator(t *testing.T) {
	var out bytes.Buffer
	app := App{Out: &out}

	code := app.Run([]string{"--help"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}

	if !strings.Contains(out.String(), "sidequest [options] -- <command> [arguments...]") {
		t.Fatalf("help output does not document command separator:\n%s", out.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	app := App{Out: &out, Version: "1.2.3"}

	code := app.Run([]string{"--version"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}

	if got, want := out.String(), "sidequest 1.2.3\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestRunCommandRunsPreflightBeforeReportingCommand(t *testing.T) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	createSessionCalled := false
	app := App{
		Out: &out,
		Err: &stderr,
		Preflight: func() error {
			return fmt.Errorf("preflight failed")
		},
		CreateSession: func() (session.Session, error) {
			createSessionCalled = true
			return session.Session{}, nil
		},
	}

	code := app.Run([]string{"--", "sleep", "1"})
	if code != 2 {
		t.Fatalf("Run exit code = %d, want 2", code)
	}

	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	if !strings.Contains(stderr.String(), "preflight failed") {
		t.Fatalf("stderr = %q, want preflight error", stderr.String())
	}
	if createSessionCalled {
		t.Fatal("CreateSession was called after preflight failure")
	}
}

func TestRunHelpSkipsPreflight(t *testing.T) {
	var out bytes.Buffer
	app := App{
		Out: &out,
		Preflight: func() error {
			t.Fatal("preflight should not run for help")
			return nil
		},
	}

	code := app.Run([]string{"--help"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
}

func TestRunCommandCreatesSessionAndHandoffsCommand(t *testing.T) {
	var out bytes.Buffer
	runtimeSession := session.Session{ID: "session-1", Dir: "/runtime/session-1"}
	var layoutSession session.Session
	var layoutCommand session.Command

	app := App{
		Out:       &out,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			return runtimeSession, nil
		},
		RunLayout: func(gotSession session.Session, gotCommand session.Command) error {
			layoutSession = gotSession
			layoutCommand = gotCommand
			return nil
		},
	}

	code := app.Run([]string{"--", "printf", "%s\n", "|", ">", "*.go"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if layoutSession.ID != runtimeSession.ID {
		t.Fatalf("layout session = %#v, want %#v", layoutSession, runtimeSession)
	}
	if layoutCommand.Executable != "printf" {
		t.Fatalf("layout executable = %q, want %q", layoutCommand.Executable, "printf")
	}
	wantArgs := []string{"%s\n", "|", ">", "*.go"}
	if !equalSlices(layoutCommand.Arguments, wantArgs) {
		t.Fatalf("layout args = %#v, want %#v", layoutCommand.Arguments, wantArgs)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestRunCommandRunnerReceivesAndExecsCommand(t *testing.T) {
	var executed session.Command
	app := App{
		ReceiveCommand: func(ctx context.Context, socketPath string) (session.Command, error) {
			if socketPath != "/tmp/sidequest-1000/session-1/command.sock" {
				t.Fatalf("socketPath = %q, want %q", socketPath, "/tmp/sidequest-1000/session-1/command.sock")
			}
			return session.Command{Executable: "bash", Arguments: []string{"-c", "exit 7"}}, nil
		},
		ExecCommand: func(runtimeSession session.Session, command session.Command) error {
			if runtimeSession.ID != "session-1" {
				t.Fatalf("runtimeSession.ID = %q, want %q", runtimeSession.ID, "session-1")
			}
			executed = command
			return nil
		},
	}

	code := app.Run([]string{commandRunnerMode, "/tmp/sidequest-1000/session-1/command.sock"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if executed.Executable != "bash" {
		t.Fatalf("executed command = %#v, want bash", executed)
	}
	if !equalSlices(executed.Arguments, []string{"-c", "exit 7"}) {
		t.Fatalf("executed arguments = %#v", executed.Arguments)
	}
}

func TestRunGameRunnerDispatchesStatePath(t *testing.T) {
	receivedPath := ""
	app := App{
		RunGameShell: func(statePath string) error {
			receivedPath = statePath
			return nil
		},
	}

	code := app.Run([]string{gameRunnerMode, "/tmp/sidequest-1000/session-1/state.json"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if receivedPath != "/tmp/sidequest-1000/session-1/state.json" {
		t.Fatalf("receivedPath = %q", receivedPath)
	}
}

func TestRunListShowsMetadataWithoutCommandArguments(t *testing.T) {
	var out bytes.Buffer
	started := time.Date(2026, 7, 11, 16, 40, 12, 0, time.Local)
	durationMillis := int64(3*time.Minute+18*time.Second) / int64(time.Millisecond)
	app := App{
		Out: &out,
		Now: func() time.Time { return started.Add(10 * time.Minute) },
		ListSessions: func() ([]session.Record, error) {
			return []session.Record{
				{
					Session: session.Session{ID: "brave-otter"},
					State: session.State{
						ID:             "brave-otter",
						Status:         session.StatusCompleted,
						CreatedAt:      started.Add(-time.Minute),
						StartedAt:      &started,
						DurationMillis: &durationMillis,
						TmuxSocket:     "sidequest-brave-otter",
					},
				},
			}, nil
		},
		TmuxHasSession: func(tmux.Info) bool { return true },
	}

	code := app.Run([]string{"list"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}

	output := out.String()
	for _, want := range []string{"ID", "STATE", "STARTED", "ELAPSED", "brave-otter", "completed", "16:40:12", "00:03:18"} {
		if !strings.Contains(output, want) {
			t.Fatalf("list output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"bash", "sleep 30", "exit 7"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("list output exposes command data %q:\n%s", forbidden, output)
		}
	}
}

func TestRunListMarksStaleTmuxMetadata(t *testing.T) {
	var out bytes.Buffer
	created := time.Date(2026, 7, 11, 16, 40, 12, 0, time.UTC)
	app := App{
		Out: &out,
		Now: func() time.Time { return created },
		ListSessions: func() ([]session.Record, error) {
			return []session.Record{
				{
					Session: session.Session{ID: "quiet-fox"},
					State: session.State{
						ID:         "quiet-fox",
						Status:     session.StatusRunning,
						CreatedAt:  created,
						TmuxSocket: "sidequest-quiet-fox",
					},
				},
			}, nil
		},
		TmuxHasSession: func(tmux.Info) bool { return false },
	}

	code := app.Run([]string{"list"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "stale") {
		t.Fatalf("list output = %q, want stale marker", out.String())
	}
}

func TestRunAttachValidatesPreflightFirst(t *testing.T) {
	var stderr bytes.Buffer
	attachCalled := false
	app := App{
		Err: &stderr,
		Preflight: func() error {
			return fmt.Errorf("not interactive")
		},
		AttachSession: func(string) error {
			attachCalled = true
			return nil
		},
	}

	code := app.Run([]string{"attach", "session-1"})
	if code != 2 {
		t.Fatalf("Run exit code = %d, want 2", code)
	}
	if attachCalled {
		t.Fatal("AttachSession was called after preflight failure")
	}
	if !strings.Contains(stderr.String(), "not interactive") {
		t.Fatalf("stderr = %q, want preflight error", stderr.String())
	}
}

func TestRunAttachCallsAttachSession(t *testing.T) {
	attachedID := ""
	app := App{
		Preflight: func() error { return nil },
		AttachSession: func(id string) error {
			attachedID = id
			return nil
		},
	}

	code := app.Run([]string{"attach", "session-1"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if attachedID != "session-1" {
		t.Fatalf("attachedID = %q, want %q", attachedID, "session-1")
	}
}

func equalSlices(a, b []string) bool {
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
