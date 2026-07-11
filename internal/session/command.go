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

type CommandListener struct {
	listener *net.UnixListener
	path     string
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

func (l *CommandListener) Receive(ctx context.Context) (Command, error) {
	if deadline, ok := ctx.Deadline(); ok {
		if err := l.listener.SetDeadline(deadline); err != nil {
			return Command{}, fmt.Errorf("set command socket deadline: %w", err)
		}
	}

	conn, err := l.listener.AcceptUnix()
	if err != nil {
		return Command{}, fmt.Errorf("accept command handoff: %w", err)
	}
	defer conn.Close()

	var command Command
	if err := json.NewDecoder(conn).Decode(&command); err != nil {
		return Command{}, fmt.Errorf("decode command handoff: %w", err)
	}
	if command.Executable == "" {
		return Command{}, ErrEmptyExecutable
	}

	return command, nil
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

func SendCommand(ctx context.Context, socketPath string, command Command) error {
	if command.Executable == "" {
		return ErrEmptyExecutable
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to command socket: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("set command handoff deadline: %w", err)
		}
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	if err := json.NewEncoder(conn).Encode(command); err != nil {
		return fmt.Errorf("encode command handoff: %w", err)
	}

	return nil
}
