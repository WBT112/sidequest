package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

var ErrEmptyExecutable = errors.New("empty command executable")

type Command struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
}

type CommandStartupStatus string

const (
	CommandStartupStarted     CommandStartupStatus = "started"
	CommandStartupStartFailed CommandStartupStatus = "start_failed"
	CommandStartupCompleted   CommandStartupStatus = StatusCompleted
	CommandStartupFailed      CommandStartupStatus = StatusFailed
	CommandStartupInterrupted CommandStartupStatus = StatusInterrupted
)

type CommandStartup struct {
	Status     CommandStartupStatus `json:"status"`
	Error      string               `json:"error,omitempty"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
	ExitSignal string               `json:"exit_signal,omitempty"`
}

type CommandListener struct {
	listener *net.UnixListener
	path     string
}

type CommandExchange struct {
	conn *net.UnixConn
}

func ListenCommand(session Session) (*CommandListener, error) {
	if err := os.Remove(session.SocketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale command socket: %w", err)
	}

	addr := net.UnixAddr{Name: session.SocketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", &addr)
	if err != nil {
		return nil, fmt.Errorf("listen on command socket: %w", err)
	}

	if err := os.Chmod(session.SocketPath, 0o600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("secure command socket: %w", err)
	}

	return &CommandListener{listener: listener, path: session.SocketPath}, nil
}

func (l *CommandListener) Serve(ctx context.Context, command Command) (CommandStartup, error) {
	if command.Executable == "" {
		return CommandStartup{}, ErrEmptyExecutable
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := l.listener.SetDeadline(deadline); err != nil {
			return CommandStartup{}, fmt.Errorf("set command socket deadline: %w", err)
		}
	}

	conn, err := l.listener.AcceptUnix()
	if err != nil {
		return CommandStartup{}, fmt.Errorf("accept command handoff: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return CommandStartup{}, fmt.Errorf("set command handoff deadline: %w", err)
		}
	}
	if err := json.NewEncoder(conn).Encode(command); err != nil {
		return CommandStartup{}, fmt.Errorf("encode command handoff: %w", err)
	}

	var startup CommandStartup
	if err := json.NewDecoder(conn).Decode(&startup); err != nil {
		return CommandStartup{}, fmt.Errorf("decode command startup status: %w", err)
	}
	if startup.Status == "" {
		return CommandStartup{}, fmt.Errorf("empty command startup status")
	}

	return startup, nil
}

func ReceiveCommand(ctx context.Context, socketPath string) (Command, error) {
	command, exchange, err := ReceiveCommandExchange(ctx, socketPath)
	if exchange != nil {
		_ = exchange.ReportStartup(CommandStartup{Status: CommandStartupStarted})
		_ = exchange.Close()
	}
	return command, err
}

func ReceiveCommandExchange(ctx context.Context, socketPath string) (Command, *CommandExchange, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return Command{}, nil, fmt.Errorf("connect to command socket: %w", err)
	}
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return Command{}, nil, fmt.Errorf("command socket is not a unix connection")
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := unixConn.SetDeadline(deadline); err != nil {
			unixConn.Close()
			return Command{}, nil, fmt.Errorf("set command handoff deadline: %w", err)
		}
	} else {
		_ = unixConn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	var command Command
	if err := json.NewDecoder(unixConn).Decode(&command); err != nil {
		unixConn.Close()
		return Command{}, nil, fmt.Errorf("decode command handoff: %w", err)
	}
	if command.Executable == "" {
		unixConn.Close()
		return Command{}, nil, ErrEmptyExecutable
	}

	return command, &CommandExchange{conn: unixConn}, nil
}

func (e *CommandExchange) ReportStartup(startup CommandStartup) error {
	if e == nil || e.conn == nil {
		return nil
	}
	if startup.Status == "" {
		startup.Status = CommandStartupStarted
	}
	if err := json.NewEncoder(e.conn).Encode(startup); err != nil {
		return fmt.Errorf("encode command startup status: %w", err)
	}
	return nil
}

func (e *CommandExchange) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

func (l *CommandListener) Close() error {
	listenerErr := l.listener.Close()
	removeErr := os.Remove(l.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	if listenerErr != nil {
		return listenerErr
	}
	return removeErr
}
