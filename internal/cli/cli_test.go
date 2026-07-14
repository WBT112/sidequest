package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WBT112/sidequest/internal/game"
	"github.com/WBT112/sidequest/internal/runhistory"
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
	if result.Config.Mode != "classic" {
		t.Fatalf("Mode = %q, want classic", result.Config.Mode)
	}
}

func TestParseModeBeforeSeparator(t *testing.T) {
	result, err := Parse([]string{"--mode", "quest", "--", "sleep", "1"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if result.Config.Mode != "quest" {
		t.Fatalf("Mode = %q, want quest", result.Config.Mode)
	}
	if result.Config.Executable != "sleep" {
		t.Fatalf("Executable = %q, want sleep", result.Config.Executable)
	}
}

func TestParseNoHistoryBeforeSeparator(t *testing.T) {
	result, err := Parse([]string{"--no-history", "--", "sleep", "1"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !result.Config.NoHistory {
		t.Fatal("NoHistory = false, want true")
	}
	if result.Config.Executable != "sleep" {
		t.Fatalf("Executable = %q, want sleep", result.Config.Executable)
	}
}

func TestParseNoColorBeforeSeparator(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	result, err := Parse([]string{"--no-color", "--", "sleep", "1"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !result.Config.NoColor {
		t.Fatal("NoColor = false, want true")
	}
	if result.Config.Executable != "sleep" {
		t.Fatalf("Executable = %q, want sleep", result.Config.Executable)
	}
}

func TestParseNoColorFromEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	result, err := Parse([]string{"--", "sleep", "1"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !result.Config.NoColor {
		t.Fatal("NoColor = false, want true from NO_COLOR")
	}
}

func TestParseAugmentedBeforeSeparator(t *testing.T) {
	result, err := Parse([]string{"--aug", "--", "sleep", "1"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !result.Config.Augmented {
		t.Fatal("Augmented = false, want true")
	}
	if result.Config.Executable != "sleep" {
		t.Fatalf("Executable = %q, want sleep", result.Config.Executable)
	}
}

func TestParseRejectsUnknownMode(t *testing.T) {
	_, err := Parse([]string{"--mode", "arena", "--", "true"})
	if err == nil || !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("Parse error = %v, want unknown mode", err)
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

func TestParsePreservesVariablesGlobsAndSpacesAsArguments(t *testing.T) {
	result, err := Parse([]string{"--", "printf", "%s", "$HOME", "*.go", "value with spaces"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	want := []string{"%s", "$HOME", "*.go", "value with spaces"}
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

func TestParseRejectsUnknownSidequestOption(t *testing.T) {
	_, err := Parse([]string{"--bogus", "--", "true"})
	if err == nil {
		t.Fatal("Parse succeeded, want unknown option error")
	}
	if !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("Parse error = %v, want unknown option", err)
	}
}

func TestParseRejectsUnexpectedArgumentBeforeSeparator(t *testing.T) {
	_, err := Parse([]string{"qest", "--", "make", "test"})
	if err == nil || !strings.Contains(err.Error(), `unexpected argument "qest" before --`) {
		t.Fatalf("Parse error = %v, want unexpected argument before separator", err)
	}
}

func TestParseRejectsMultipleUnexpectedArgumentsBeforeSeparator(t *testing.T) {
	_, err := Parse([]string{"foo", "bar", "--", "true"})
	if err == nil || !strings.Contains(err.Error(), `unexpected argument "foo" before --`) {
		t.Fatalf("Parse error = %v, want first unexpected argument before separator", err)
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

	for _, want := range []string{"sidequest [options] -- <command> [arguments...]", "--no-history", "--no-color", "--aug"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunCommandStoresNoHistoryChoice(t *testing.T) {
	var out bytes.Buffer
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("no-history")}
	app := App{
		Out:       &out,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			return manager.Create()
		},
		RunLayout: func(gotSession session.Session, gotCommand session.Command) error {
			state, err := session.ReadState(gotSession)
			if err != nil {
				t.Fatalf("ReadState returned error: %v", err)
			}
			if !state.NoHistory {
				t.Fatal("NoHistory = false, want true")
			}
			return nil
		},
	}

	code := app.Run([]string{"--no-history", "--", "true"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestRunCommandStoresNoColorChoice(t *testing.T) {
	var out bytes.Buffer
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("no-color")}
	app := App{
		Out:       &out,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			return manager.Create()
		},
		RunLayout: func(gotSession session.Session, gotCommand session.Command) error {
			state, err := session.ReadState(gotSession)
			if err != nil {
				t.Fatalf("ReadState returned error: %v", err)
			}
			if !state.NoColor {
				t.Fatal("NoColor = false, want true")
			}
			return nil
		},
	}

	code := app.Run([]string{"--no-color", "--", "true"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestRunCommandStoresAugmentedChoice(t *testing.T) {
	var out bytes.Buffer
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("augmented")}
	app := App{
		Out:       &out,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			return manager.Create()
		},
		RunLayout: func(gotSession session.Session, gotCommand session.Command) error {
			state, err := session.ReadState(gotSession)
			if err != nil {
				t.Fatalf("ReadState returned error: %v", err)
			}
			if !state.Augmented {
				t.Fatal("Augmented = false, want true")
			}
			return nil
		},
	}

	code := app.Run([]string{"--aug", "--", "true"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
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

func TestRunCommandStoresSelectedGameMode(t *testing.T) {
	var out bytes.Buffer
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("quest-mode")}
	app := App{
		Out:       &out,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			return manager.Create()
		},
		RunLayout: func(gotSession session.Session, gotCommand session.Command) error {
			state, err := session.ReadState(gotSession)
			if err != nil {
				t.Fatalf("ReadState returned error: %v", err)
			}
			if state.GameMode != "quest" {
				t.Fatalf("GameMode = %q, want quest", state.GameMode)
			}
			return nil
		},
	}

	code := app.Run([]string{"--mode=quest", "--", "true"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
}

func TestRunCommandRejectsMissingPATHExecutableBeforeCreatingSession(t *testing.T) {
	var stderr bytes.Buffer
	createSessionCalled := false
	app := App{
		Err: &stderr,
		Preflight: func() error {
			t.Fatal("Preflight should not run for missing command")
			return nil
		},
		CreateSession: func() (session.Session, error) {
			createSessionCalled = true
			return session.Session{}, nil
		},
	}

	code := app.Run([]string{"--", "sidequest-command-that-does-not-exist"})
	if code != 127 {
		t.Fatalf("Run exit code = %d, want 127", code)
	}
	if createSessionCalled {
		t.Fatal("CreateSession was called after missing command validation")
	}
	if !strings.Contains(stderr.String(), "command not found: sidequest-command-that-does-not-exist") {
		t.Fatalf("stderr = %q, want command not found", stderr.String())
	}
}

func TestRunCommandRejectsMissingExplicitPathBeforeCreatingSession(t *testing.T) {
	var stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing-script")
	app := App{
		Err:       &stderr,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			t.Fatal("CreateSession should not run for missing explicit path")
			return session.Session{}, nil
		},
	}

	code := app.Run([]string{"--", missing})
	if code != 127 {
		t.Fatalf("Run exit code = %d, want 127", code)
	}
	if !strings.Contains(stderr.String(), "command not found: "+missing) {
		t.Fatalf("stderr = %q, want explicit missing path", stderr.String())
	}
}

func TestRunCommandRejectsDirectoryAndNonExecutableBeforeCreatingSession(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "directory", path: dir, want: "is a directory"},
		{name: "non-executable", path: script, want: "permission denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			app := App{
				Err:       &stderr,
				Preflight: func() error { return nil },
				CreateSession: func() (session.Session, error) {
					t.Fatal("CreateSession should not run for invalid executable")
					return session.Session{}, nil
				},
			}

			code := app.Run([]string{"--", test.path})
			if code != 126 {
				t.Fatalf("Run exit code = %d, want 126", code)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunCommandCleansRuntimeSessionWhenInitialStateUpdateFails(t *testing.T) {
	var stderr bytes.Buffer
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("initial-state-fail")}
	var runtimeSession session.Session

	app := App{
		Err:       &stderr,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			var err error
			runtimeSession, err = manager.Create()
			return runtimeSession, err
		},
		UpdateState: func(session.Session, time.Time, func(*session.State)) error {
			return fmt.Errorf("write options failed")
		},
		RunLayout: func(session.Session, session.Command) error {
			t.Fatal("RunLayout should not start after initial state failure")
			return nil
		},
	}

	code := app.Run([]string{"--mode=quest", "--", "true"})
	if code != 2 {
		t.Fatalf("Run exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "write options failed") {
		t.Fatalf("stderr = %q, want original state update error", stderr.String())
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunCommandCustomLayoutFailureCleansRuntimeSession(t *testing.T) {
	var stderr bytes.Buffer
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("custom-layout-fail")}
	var runtimeSession session.Session

	app := App{
		Err:       &stderr,
		Preflight: func() error { return nil },
		CreateSession: func() (session.Session, error) {
			var err error
			runtimeSession, err = manager.Create()
			return runtimeSession, err
		},
		RunLayout: func(session.Session, session.Command) error {
			return fmt.Errorf("layout failed")
		},
	}

	code := app.Run([]string{"--", "true"})
	if code != 2 {
		t.Fatalf("Run exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "layout failed") {
		t.Fatalf("stderr = %q, want original layout error", stderr.String())
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutStartFailureCleansRuntimeSessionAndCommandSocket(t *testing.T) {
	runtimeSession := createCLITestSession(t, "layout-start-fail")
	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return tmux.Info{}, fmt.Errorf("start layout failed")
		},
		CloseTmux: func(tmux.Info) error {
			t.Fatal("CloseTmux should not run before layout ownership is established")
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "start layout failed") {
		t.Fatalf("runLayout error = %v, want original start error", err)
	}
	assertSessionDirRemoved(t, runtimeSession)
	assertSocketRemoved(t, runtimeSession)
}

func TestRunLayoutStateMetadataFailureClosesOwnedTmuxAndCleansRuntimeSession(t *testing.T) {
	runtimeSession := createCLITestSession(t, "state-metadata-fail")
	ownedInfo := tmux.Info{SocketName: "sidequest-state-metadata-fail", SessionName: "sidequest-state-metadata-fail"}
	var closed []tmux.Info

	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		UpdateState: func(session.Session, time.Time, func(*session.State)) error {
			return fmt.Errorf("store tmux metadata failed")
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "store tmux metadata failed") {
		t.Fatalf("runLayout error = %v, want original metadata error", err)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutStartupFailureDoesNotCloseUnownedTmuxInfo(t *testing.T) {
	runtimeSession := createCLITestSession(t, "unowned-tmux")
	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return tmux.Info{SocketName: "external", SessionName: "external"}, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			return session.CommandStartup{}, fmt.Errorf("command handoff failed")
		},
		CloseTmux: func(tmux.Info) error {
			t.Fatal("CloseTmux should not run for unowned tmux metadata")
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "command handoff failed") {
		t.Fatalf("runLayout error = %v, want original handoff error", err)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutCommandHandoffFailureClosesOwnedTmuxCleansRuntimeSessionAndSocket(t *testing.T) {
	runtimeSession := createCLITestSession(t, "handoff-fail")
	ownedInfo := tmux.Info{SocketName: "sidequest-handoff-fail", SessionName: "sidequest-handoff-fail"}
	var closed []tmux.Info

	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			return session.CommandStartup{}, fmt.Errorf("command handoff failed")
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "command handoff failed") {
		t.Fatalf("runLayout error = %v, want original handoff error", err)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
	assertSocketRemoved(t, runtimeSession)
}

func TestRunLayoutStartFailedStatusClosesOwnedTmuxAndReturnsCommandExitCode(t *testing.T) {
	runtimeSession := createCLITestSession(t, "start-status-fail")
	ownedInfo := tmux.Info{SocketName: "sidequest-start-status-fail", SessionName: "sidequest-start-status-fail"}
	var closed []tmux.Info
	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			return session.CommandStartup{Status: session.CommandStartupStartFailed, Error: "fork/exec ./bad: no such file or directory"}, nil
		},
		TmuxHasSession: func(tmux.Info) bool { return true },
		CapturePane: func(tmux.Info) (string, bool, error) {
			return "", false, nil
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "fork/exec ./bad") {
		t.Fatalf("runLayout error = %v, want start failure", err)
	}
	if got := exitCodeForError(err); got != 127 {
		t.Fatalf("exit code = %d, want 127", got)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutImmediateZeroExitCleansOwnedResourcesWithoutAttach(t *testing.T) {
	runtimeSession := createCLITestSession(t, "startup-zero")
	ownedInfo := tmux.Info{SocketName: "sidequest-startup-zero", SessionName: "sidequest-startup-zero"}
	var closed []tmux.Info
	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			exitCode := 0
			return session.CommandStartup{Status: session.CommandStartupCompleted, ExitCode: &exitCode}, nil
		},
		AttachLayout: func(tmux.Info) error {
			t.Fatal("AttachLayout should not run after immediate command exit")
			return nil
		},
		TmuxHasSession: func(tmux.Info) bool { return true },
		CapturePane: func(tmux.Info) (string, bool, error) {
			return "", false, nil
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	if err := app.runLayout(runtimeSession, session.Command{Executable: "true"}); err != nil {
		t.Fatalf("runLayout returned error: %v", err)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutImmediateNonZeroExitCleansOwnedResourcesAndReturnsExitCode(t *testing.T) {
	runtimeSession := createCLITestSession(t, "startup-exit7")
	ownedInfo := tmux.Info{SocketName: "sidequest-startup-exit7", SessionName: "sidequest-startup-exit7"}
	var closed []tmux.Info
	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			exitCode := 7
			return session.CommandStartup{Status: session.CommandStartupFailed, ExitCode: &exitCode}, nil
		},
		AttachLayout: func(tmux.Info) error {
			t.Fatal("AttachLayout should not run after immediate command exit")
			return nil
		},
		TmuxHasSession: func(tmux.Info) bool { return true },
		CapturePane: func(tmux.Info) (string, bool, error) {
			return "", false, nil
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "status 7") {
		t.Fatalf("runLayout error = %v, want startup exit status", err)
	}
	if got := exitCodeForError(err); got != 7 {
		t.Fatalf("exit code = %d, want 7", got)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutAttachFailureClosesOwnedTmuxAndCleansRuntimeSession(t *testing.T) {
	runtimeSession := createCLITestSession(t, "attach-fail")
	ownedInfo := tmux.Info{SocketName: "sidequest-attach-fail", SessionName: "sidequest-attach-fail"}
	var closed []tmux.Info

	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			return session.CommandStartup{Status: session.CommandStartupStarted}, nil
		},
		AttachLayout: func(tmux.Info) error {
			return fmt.Errorf("attach failed")
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "attach failed") {
		t.Fatalf("runLayout error = %v, want original attach error", err)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutFinalStateReadFailureClosesOwnedTmuxAndCleansRuntimeSession(t *testing.T) {
	runtimeSession := createCLITestSession(t, "final-read-fail")
	ownedInfo := tmux.Info{SocketName: "sidequest-final-read-fail", SessionName: "sidequest-final-read-fail"}
	var closed []tmux.Info

	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			return session.CommandStartup{Status: session.CommandStartupStarted}, nil
		},
		AttachLayout: func(tmux.Info) error {
			return nil
		},
		ReadState: func(session.Session) (session.State, error) {
			return session.State{}, fmt.Errorf("read final state failed")
		},
		CloseTmux: func(info tmux.Info) error {
			closed = append(closed, info)
			return nil
		},
	}

	err := app.runLayout(runtimeSession, session.Command{Executable: "true"})
	if err == nil || !strings.Contains(err.Error(), "read final state failed") {
		t.Fatalf("runLayout error = %v, want original read error", err)
	}
	if got, want := closed, []tmux.Info{ownedInfo}; !equalTmuxInfos(got, want) {
		t.Fatalf("closed tmux = %#v, want %#v", got, want)
	}
	assertSessionDirRemoved(t, runtimeSession)
}

func TestRunLayoutPreservesRunningSessionAfterSuccessfulAttach(t *testing.T) {
	runtimeSession := createCLITestSession(t, "still-running")
	ownedInfo := tmux.Info{SocketName: "sidequest-still-running", SessionName: "sidequest-still-running"}
	app := App{
		StartLayout: func(session.Session, []string, []string) (tmux.Info, error) {
			return ownedInfo, nil
		},
		ServeCommand: func(context.Context, *session.CommandListener, session.Command) (session.CommandStartup, error) {
			return session.CommandStartup{Status: session.CommandStartupStarted}, nil
		},
		AttachLayout: func(tmux.Info) error {
			return nil
		},
		CloseTmux: func(tmux.Info) error {
			t.Fatal("CloseTmux should not run for a detached running session")
			return nil
		},
		CleanupSession: func(session.Session) error {
			t.Fatal("CleanupSession should not run for a detached running session")
			return nil
		},
	}

	if err := app.runLayout(runtimeSession, session.Command{Executable: "true"}); err != nil {
		t.Fatalf("runLayout returned error: %v", err)
	}
	if _, err := session.ReadState(runtimeSession); err != nil {
		t.Fatalf("running session was removed: %v", err)
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

func TestRunGameShellConfiguresProductionRandomSource(t *testing.T) {
	started := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	var captured game.Shell
	app := App{
		Now: func() time.Time { return started },
		RunShell: func(shell game.Shell) error {
			captured = shell
			return nil
		},
	}

	code := app.Run([]string{gameRunnerMode, "/tmp/sidequest-1000/session-1/state.json"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if captured.Random == nil {
		t.Fatal("game shell Random = nil, want production source")
	}
	if captured.ReadState == nil {
		t.Fatal("game shell ReadState = nil")
	}
	if captured.ReadFocus == nil {
		t.Fatal("game shell ReadFocus = nil")
	}
	if captured.ReadCommandPreview == nil {
		t.Fatal("game shell ReadCommandPreview = nil")
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

func TestRunListDoesNotInspectUnownedTmuxMetadata(t *testing.T) {
	var out bytes.Buffer
	checkedTmux := false
	app := App{
		Out: &out,
		Now: time.Now,
		ListSessions: func() ([]session.Record, error) {
			return []session.Record{
				{
					Session: session.Session{ID: "quiet-fox"},
					State: session.State{
						ID:         "quiet-fox",
						Status:     session.StatusCompleted,
						CreatedAt:  time.Now(),
						TmuxSocket: "external-tmux",
					},
				},
			}, nil
		},
		TmuxHasSession: func(tmux.Info) bool {
			checkedTmux = true
			return true
		},
	}

	code := app.Run([]string{"list"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	if checkedTmux {
		t.Fatal("list inspected unowned tmux metadata")
	}
	if !strings.Contains(out.String(), "stale") {
		t.Fatalf("list output = %q, want stale marker", out.String())
	}
}

func TestRunRunsShowsStoredRunMetadataWithoutCommandArguments(t *testing.T) {
	var out bytes.Buffer
	exitCode := 0
	finished := time.Date(2026, 7, 11, 18, 43, 15, 0, time.Local)
	app := App{
		Out: &out,
		ListRuns: func() ([]runhistory.Run, error) {
			return []runhistory.Run{
				{
					Result: runhistory.Result{
						ID:             "brave-otter",
						FinishedAt:     finished,
						DurationMillis: 275000,
						ExitCode:       &exitCode,
						Termination:    session.StatusCompleted,
					},
					OutputPath: "/state/sidequest/runs/brave-otter/output.txt",
				},
			}, nil
		},
	}

	code := app.Run([]string{"runs"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	output := out.String()
	for _, want := range []string{"ID", "FINISHED", "EXIT", "DURATION", "brave-otter", "0", "00:04:35"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runs output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"bash", "secret", "sudo apt"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("runs output exposes command data %q:\n%s", forbidden, output)
		}
	}
}

func TestRunShowDisplaysMetadataAndOutputPath(t *testing.T) {
	var out bytes.Buffer
	exitCode := 1
	started := time.Date(2026, 7, 11, 18, 38, 40, 0, time.Local)
	finished := time.Date(2026, 7, 11, 18, 43, 15, 0, time.Local)
	app := App{
		Out: &out,
		FindRun: func(id string) (runhistory.Run, error) {
			if id != "last" {
				t.Fatalf("id = %q, want last", id)
			}
			return runhistory.Run{
				Result: runhistory.Result{
					ID:              "brave-otter",
					StartedAt:       started,
					FinishedAt:      finished,
					DurationMillis:  275000,
					ExitCode:        &exitCode,
					Termination:     session.StatusFailed,
					OutputTruncated: true,
				},
				OutputPath: "/state/sidequest/runs/brave-otter/output.txt",
			}, nil
		},
	}

	code := app.Run([]string{"show", "last"})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}
	output := out.String()
	for _, want := range []string{"ID: brave-otter", "Exit code: 1", "Termination: failed", "Output truncated: true", "Output: /state/sidequest/runs/brave-otter/output.txt"} {
		if !strings.Contains(output, want) {
			t.Fatalf("show output missing %q:\n%s", want, output)
		}
	}
}

func TestRunOutputAndPurgeDispatchToHistory(t *testing.T) {
	outputID := ""
	purgeID := ""
	app := App{
		OutputRun: func(id string) error {
			outputID = id
			return nil
		},
		PurgeRun: func(id string) error {
			purgeID = id
			return nil
		},
	}

	if code := app.Run([]string{"output", "last"}); code != 0 {
		t.Fatalf("output exit code = %d, want 0", code)
	}
	if code := app.Run([]string{"purge", "brave-otter"}); code != 0 {
		t.Fatalf("purge exit code = %d, want 0", code)
	}
	if outputID != "last" {
		t.Fatalf("outputID = %q, want last", outputID)
	}
	if purgeID != "brave-otter" {
		t.Fatalf("purgeID = %q, want brave-otter", purgeID)
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

func TestPrintReconnectHintForRunningSession(t *testing.T) {
	var out bytes.Buffer
	app := App{Out: &out}

	app.printReconnectHint(
		session.Session{ID: "9d4f5dcd6ad45b93f1f07ebb64d67c9b"},
		session.State{Status: session.StatusRunning},
	)

	want := "Reconnect with: sidequest attach 9d4f5dcd6ad45b93f1f07ebb64d67c9b\n"
	if out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
	}
}

func TestPrintReconnectHintSkipsTerminalSession(t *testing.T) {
	var out bytes.Buffer
	app := App{Out: &out}

	app.printReconnectHint(
		session.Session{ID: "done"},
		session.State{Status: session.StatusCompleted},
	)

	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestCleanupClosedSessionRemovesTerminalOwnedSession(t *testing.T) {
	exitCode := 0
	record := session.Record{
		Session: session.Session{ID: "done", Dir: "/tmp/sidequest-1000/done"},
		State: session.State{
			Status:     session.StatusCompleted,
			ExitCode:   &exitCode,
			TmuxSocket: "sidequest-done",
		},
	}
	closed := ""
	cleaned := ""
	app := App{
		TmuxHasSession: func(tmux.Info) bool { return false },
		CloseTmux: func(info tmux.Info) error {
			closed = info.SocketName
			return nil
		},
		CleanupSession: func(runtimeSession session.Session) error {
			cleaned = runtimeSession.ID
			return nil
		},
	}

	if err := app.cleanupClosedSession(record); err != nil {
		t.Fatalf("cleanupClosedSession returned error: %v", err)
	}
	if closed != "sidequest-done" {
		t.Fatalf("closed = %q, want sidequest-done", closed)
	}
	if cleaned != "done" {
		t.Fatalf("cleaned = %q, want done", cleaned)
	}
}

func TestCleanupClosedSessionCapturesAndStoresBeforeClosingTmux(t *testing.T) {
	exitCode := 0
	record := session.Record{
		Session: session.Session{ID: "done", Dir: "/tmp/sidequest-1000/done"},
		State: session.State{
			Status:     session.StatusCompleted,
			ExitCode:   &exitCode,
			TmuxSocket: "sidequest-done",
		},
	}
	var order []string
	var out bytes.Buffer
	app := App{
		Out:            &out,
		TmuxHasSession: func(tmux.Info) bool { return true },
		CapturePane: func(info tmux.Info) (string, bool, error) {
			order = append(order, "capture")
			return "visible output\n", true, nil
		},
		StoreRun: func(got session.Record, output string, truncated bool) (runhistory.Run, error) {
			order = append(order, "store")
			if got.Session.ID != "done" || output != "visible output\n" || !truncated {
				t.Fatalf("stored = %#v output=%q truncated=%t", got, output, truncated)
			}
			return runhistory.Run{
				Result:     runhistory.Result{ID: "done"},
				OutputPath: "/state/sidequest/runs/done/output.txt",
			}, nil
		},
		CloseTmux: func(tmux.Info) error {
			order = append(order, "close")
			return nil
		},
		CleanupSession: func(session.Session) error {
			order = append(order, "cleanup")
			return nil
		},
	}

	if err := app.cleanupClosedSession(record); err != nil {
		t.Fatalf("cleanupClosedSession returned error: %v", err)
	}
	if got, want := strings.Join(order, ","), "capture,store,close,cleanup"; got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
	output := out.String()
	for _, want := range []string{
		"Saved output: /state/sidequest/runs/done/output.txt",
		"View it with: sidequest output done",
		"Metadata: sidequest show done",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("cleanup output missing %q:\n%s", want, output)
		}
	}
}

func TestCleanupClosedSessionSkipsCaptureAndStoreWhenNoHistory(t *testing.T) {
	exitCode := 0
	record := session.Record{
		Session: session.Session{ID: "secret", Dir: "/tmp/sidequest-1000/secret"},
		State: session.State{
			Status:     session.StatusCompleted,
			ExitCode:   &exitCode,
			TmuxSocket: "sidequest-secret",
			NoHistory:  true,
		},
	}
	var order []string
	var out bytes.Buffer
	app := App{
		Out:            &out,
		TmuxHasSession: func(tmux.Info) bool { return true },
		CapturePane: func(tmux.Info) (string, bool, error) {
			t.Fatal("CapturePane was called in no-history mode")
			return "", false, nil
		},
		StoreRun: func(session.Record, string, bool) (runhistory.Run, error) {
			t.Fatal("StoreRun was called in no-history mode")
			return runhistory.Run{}, nil
		},
		CloseTmux: func(tmux.Info) error {
			order = append(order, "close")
			return nil
		},
		CleanupSession: func(session.Session) error {
			order = append(order, "cleanup")
			return nil
		},
	}

	if err := app.cleanupClosedSession(record); err != nil {
		t.Fatalf("cleanupClosedSession returned error: %v", err)
	}
	if got, want := strings.Join(order, ","), "close,cleanup"; got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
	output := out.String()
	if !strings.Contains(output, "History disabled: no command output saved for run secret") {
		t.Fatalf("cleanup output missing no-history notice:\n%s", output)
	}
	if strings.Contains(output, "Saved output") || strings.Contains(output, "sidequest output secret") || strings.Contains(output, "sidequest show secret") {
		t.Fatalf("cleanup output referenced stored history in no-history mode:\n%s", output)
	}
}

func TestCleanupClosedSessionPreservesRunningSession(t *testing.T) {
	called := false
	app := App{
		CloseTmux: func(tmux.Info) error {
			called = true
			return nil
		},
		CapturePane: func(tmux.Info) (string, bool, error) {
			called = true
			return "", false, nil
		},
		StoreRun: func(session.Record, string, bool) (runhistory.Run, error) {
			called = true
			return runhistory.Run{}, nil
		},
		CleanupSession: func(session.Session) error {
			called = true
			return nil
		},
	}

	err := app.cleanupClosedSession(session.Record{
		Session: session.Session{ID: "running"},
		State:   session.State{Status: session.StatusRunning, TmuxSocket: "sidequest-running"},
	})
	if err != nil {
		t.Fatalf("cleanupClosedSession returned error: %v", err)
	}
	if called {
		t.Fatal("cleanup touched running session")
	}
}

func TestCleanupClosedSessionDoesNotCloseUnownedTmuxSocket(t *testing.T) {
	closed := false
	cleaned := false
	app := App{
		CloseTmux: func(tmux.Info) error {
			closed = true
			return nil
		},
		CleanupSession: func(session.Session) error {
			cleaned = true
			return nil
		},
	}

	err := app.cleanupClosedSession(session.Record{
		Session: session.Session{ID: "done"},
		State:   session.State{Status: session.StatusCompleted, TmuxSocket: "external-session"},
	})
	if err != nil {
		t.Fatalf("cleanupClosedSession returned error: %v", err)
	}
	if closed {
		t.Fatal("cleanup closed unowned tmux socket")
	}
	if !cleaned {
		t.Fatal("cleanup did not remove Sidequest runtime session")
	}
}

func TestCleanupStaleFinishedSessionsRemovesFinishedSessionsWithoutTmux(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID("finished")}
	finished, err := manager.Create()
	if err != nil {
		t.Fatalf("Create finished returned error: %v", err)
	}
	if err := session.UpdateState(finished, time.Now(), func(state *session.State) {
		state.Status = session.StatusCompleted
		state.TmuxSocket = "sidequest-finished"
	}); err != nil {
		t.Fatalf("UpdateState finished returned error: %v", err)
	}

	manager.IDGenerator = fixedID("running")
	running, err := manager.Create()
	if err != nil {
		t.Fatalf("Create running returned error: %v", err)
	}
	if err := session.UpdateState(running, time.Now(), func(state *session.State) {
		state.Status = session.StatusRunning
		state.TmuxSocket = "sidequest-running"
	}); err != nil {
		t.Fatalf("UpdateState running returned error: %v", err)
	}

	app := App{TmuxHasSession: func(tmux.Info) bool { return false }}
	if err := app.cleanupStaleFinishedSessions(manager); err != nil {
		t.Fatalf("cleanupStaleFinishedSessions returned error: %v", err)
	}

	if _, err := session.ReadState(finished); !session.IsNotExist(err) {
		t.Fatalf("finished state error = %v, want not exist", err)
	}
	if _, err := session.ReadState(running); err != nil {
		t.Fatalf("running state was removed: %v", err)
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

func fixedID(id string) func() (string, error) {
	return func() (string, error) {
		return id, nil
	}
}

func createCLITestSession(t *testing.T, id string) session.Session {
	t.Helper()
	base, err := os.MkdirTemp("", "sq-cli-")
	if err != nil {
		t.Fatalf("MkdirTemp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(base)
	})
	manager := session.Manager{BaseDir: base, IDGenerator: fixedID(id)}
	runtimeSession, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	return runtimeSession
}

func assertSessionDirRemoved(t *testing.T, runtimeSession session.Session) {
	t.Helper()
	if runtimeSession.Dir == "" {
		t.Fatal("runtimeSession.Dir is empty")
	}
	if _, err := os.Stat(runtimeSession.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir stat error = %v, want not exist", err)
	}
}

func assertSocketRemoved(t *testing.T, runtimeSession session.Session) {
	t.Helper()
	if _, err := os.Stat(runtimeSession.SocketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket stat error = %v, want not exist", err)
	}
}

func equalTmuxInfos(a, b []tmux.Info) bool {
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
