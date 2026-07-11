package commandexec

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/WBT112/sidequest/internal/session"
)

type Executor struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Now    func() time.Time
}

func DefaultExecutor() Executor {
	return Executor{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Now:    time.Now,
	}
}

func (e Executor) Run(runtimeSession session.Session, command session.Command) error {
	if command.Executable == "" {
		return session.ErrEmptyExecutable
	}

	startedAt := e.now().UTC()
	if err := session.UpdateState(runtimeSession, startedAt, func(state *session.State) {
		state.Status = session.StatusRunning
		state.StartedAt = &startedAt
		state.FinishedAt = nil
		state.DurationMillis = nil
		state.ExitCode = nil
		state.ExitSignal = ""
		state.StartError = ""
	}); err != nil {
		return err
	}

	process := exec.Command(command.Executable, command.Arguments...)
	process.Stdin = e.Stdin
	process.Stdout = e.Stdout
	process.Stderr = e.Stderr

	if err := process.Start(); err != nil {
		finishedAt := e.now().UTC()
		durationMillis := finishedAt.Sub(startedAt).Milliseconds()
		if durationMillis < 0 {
			durationMillis = 0
		}
		_ = session.UpdateState(runtimeSession, finishedAt, func(state *session.State) {
			state.Status = session.StatusStartFailed
			state.FinishedAt = &finishedAt
			state.DurationMillis = &durationMillis
			state.StartError = err.Error()
		})
		return err
	}

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-signals:
			case <-done:
				return
			}
		}
	}()

	waitErr := process.Wait()
	close(done)

	finishedAt := e.now().UTC()
	durationMillis := finishedAt.Sub(startedAt).Milliseconds()
	if durationMillis < 0 {
		durationMillis = 0
	}
	result := resultFromWaitError(waitErr)

	if err := session.UpdateState(runtimeSession, finishedAt, func(state *session.State) {
		state.Status = result.status
		state.FinishedAt = &finishedAt
		state.DurationMillis = &durationMillis
		state.ExitCode = result.exitCode
		state.ExitSignal = result.exitSignal
	}); err != nil {
		return err
	}

	return waitErr
}

func ExitCodeForError(err error) int {
	if err == nil {
		return 0
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return 127
	}
	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return 1
	}
	if waitStatus.Signaled() {
		return 128 + int(waitStatus.Signal())
	}
	if waitStatus.Exited() {
		return waitStatus.ExitStatus()
	}
	return 1
}

func (e Executor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

type result struct {
	status     string
	exitCode   *int
	exitSignal string
}

func resultFromWaitError(err error) result {
	if err == nil {
		exitCode := 0
		return result{status: session.StatusCompleted, exitCode: &exitCode}
	}

	var waitStatus syscall.WaitStatus
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			waitStatus = status
		}
	}

	if ok && waitStatus.Signaled() {
		return result{status: session.StatusInterrupted, exitSignal: waitStatus.Signal().String()}
	}
	if ok && waitStatus.Exited() {
		exitCode := waitStatus.ExitStatus()
		return result{status: session.StatusFailed, exitCode: &exitCode}
	}

	return result{status: session.StatusFailed, exitSignal: fmt.Sprintf("unknown: %v", err)}
}
