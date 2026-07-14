package preflight

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateAcceptsSupportedEnvironment(t *testing.T) {
	err := Validate(validEnvironment())
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsUnsupportedPlatform(t *testing.T) {
	env := validEnvironment()
	env.GOOS = "darwin"

	err := Validate(env)
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Validate error = %v, want %v", err, ErrUnsupportedPlatform)
	}
}

func TestValidateRejectsNonInteractiveStdin(t *testing.T) {
	env := validEnvironment()
	env.IsTerminal = func(fd uintptr) bool {
		return fd == env.StdoutFD
	}

	err := Validate(env)
	if !errors.Is(err, ErrNonInteractive) {
		t.Fatalf("Validate error = %v, want %v", err, ErrNonInteractive)
	}

	if !strings.Contains(err.Error(), "stdin=false, stdout=true") {
		t.Fatalf("error does not identify terminal state: %v", err)
	}
}

func TestValidateRejectsNonInteractiveStdout(t *testing.T) {
	env := validEnvironment()
	env.IsTerminal = func(fd uintptr) bool {
		return fd == env.StdinFD
	}

	err := Validate(env)
	if !errors.Is(err, ErrNonInteractive) {
		t.Fatalf("Validate error = %v, want %v", err, ErrNonInteractive)
	}

	if !strings.Contains(err.Error(), "stdin=true, stdout=false") {
		t.Fatalf("error does not identify terminal state: %v", err)
	}
}

func TestValidateRejectsUnavailableTerminalSize(t *testing.T) {
	env := validEnvironment()
	env.TerminalSize = func(uintptr) (Size, error) {
		return Size{}, fmt.Errorf("ioctl failed")
	}

	err := Validate(env)
	if !errors.Is(err, ErrTerminalSize) {
		t.Fatalf("Validate error = %v, want %v", err, ErrTerminalSize)
	}
}

func TestValidateRejectsSmallTerminal(t *testing.T) {
	env := validEnvironment()
	env.TerminalSize = func(uintptr) (Size, error) {
		return Size{Columns: 80, Rows: 21}, nil
	}

	err := Validate(env)
	if !errors.Is(err, ErrTerminalTooSmall) {
		t.Fatalf("Validate error = %v, want %v", err, ErrTerminalTooSmall)
	}

	message := err.Error()
	for _, fragment := range []string{"required at least 80x22", "detected 80x21"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("error %q does not contain %q", message, fragment)
		}
	}
}

func TestValidateRejectsMissingTmux(t *testing.T) {
	env := validEnvironment()
	env.LookPath = func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}

	err := Validate(env)
	if !errors.Is(err, ErrMissingTmux) {
		t.Fatalf("Validate error = %v, want %v", err, ErrMissingTmux)
	}

	if !strings.Contains(err.Error(), "install tmux") {
		t.Fatalf("missing tmux error is not installation-oriented: %v", err)
	}
}

func validEnvironment() Environment {
	return Environment{
		GOOS:     "linux",
		StdinFD:  1,
		StdoutFD: 2,
		LookPath: func(string) (string, error) {
			return "/usr/bin/tmux", nil
		},
		IsTerminal: func(uintptr) bool {
			return true
		},
		TerminalSize: func(uintptr) (Size, error) {
			return Size{Columns: MinimumColumns, Rows: MinimumRows}, nil
		},
	}
}
