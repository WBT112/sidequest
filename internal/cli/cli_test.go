package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
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
	app := App{
		Out: &out,
		Err: &stderr,
		Preflight: func() error {
			return fmt.Errorf("preflight failed")
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
