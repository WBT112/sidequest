package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	StatusCreated           = "created"
	StatusRunning           = "running"
	StatusCompleted         = "completed"
	StatusFailed            = "failed"
	StatusInterrupted       = "interrupted"
	StatusStartFailed       = "start_failed"
	DefaultStateFileName    = "state.json"
	DefaultCommandSocket    = "command.sock"
	runtimeDirectoryPerm    = 0o700
	regularStateFilePerm    = 0o600
	sessionIDRandomByteSize = 16
	maxSessionIDAttempts    = 16
)

type Session struct {
	ID         string
	Dir        string
	StatePath  string
	SocketPath string
}

type Record struct {
	Session Session
	State   State
}

type State struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMillis *int64     `json:"duration_millis,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	ExitSignal     string     `json:"exit_signal,omitempty"`
	StartError     string     `json:"start_error,omitempty"`
	TmuxSocket     string     `json:"tmux_socket,omitempty"`
}

type Manager struct {
	BaseDir     string
	UID         int
	IDGenerator func() (string, error)
	Now         func() time.Time
}

func DefaultManager() Manager {
	return Manager{
		BaseDir: RuntimeBaseDir(os.Getenv("XDG_RUNTIME_DIR"), os.Getuid()),
		UID:     os.Getuid(),
	}
}

func RuntimeBaseDir(xdgRuntimeDir string, uid int) string {
	if xdgRuntimeDir != "" {
		return filepath.Join(xdgRuntimeDir, "sidequest")
	}
	return filepath.Join(os.TempDir(), "sidequest-"+strconv.Itoa(uid))
}

func (m Manager) Create() (Session, error) {
	if m.BaseDir == "" {
		m.BaseDir = RuntimeBaseDir(os.Getenv("XDG_RUNTIME_DIR"), m.uid())
	}
	if m.IDGenerator == nil {
		m.IDGenerator = randomID
	}
	if m.Now == nil {
		m.Now = time.Now
	}

	if err := ensurePrivateDir(m.BaseDir); err != nil {
		return Session{}, err
	}

	for attempt := 0; attempt < maxSessionIDAttempts; attempt++ {
		id, err := m.IDGenerator()
		if err != nil {
			return Session{}, fmt.Errorf("create session id: %w", err)
		}
		if !validSessionID(id) {
			return Session{}, fmt.Errorf("invalid session id %q", id)
		}

		session := newSession(m.BaseDir, id)
		if err := os.Mkdir(session.Dir, runtimeDirectoryPerm); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return Session{}, fmt.Errorf("create session directory: %w", err)
		}
		if err := os.Chmod(session.Dir, runtimeDirectoryPerm); err != nil {
			_ = os.RemoveAll(session.Dir)
			return Session{}, fmt.Errorf("secure session directory: %w", err)
		}

		now := m.Now().UTC()
		state := State{
			ID:        id,
			Status:    StatusCreated,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := WriteState(session, state); err != nil {
			_ = os.RemoveAll(session.Dir)
			return Session{}, err
		}

		return session, nil
	}

	return Session{}, fmt.Errorf("create unique session id: exhausted %d attempts", maxSessionIDAttempts)
}

func (m Manager) List() ([]Record, error) {
	if m.BaseDir == "" {
		m.BaseDir = RuntimeBaseDir(os.Getenv("XDG_RUNTIME_DIR"), m.uid())
	}

	entries, err := os.ReadDir(m.BaseDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read runtime directory: %w", err)
	}

	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runtimeSession := newSession(m.BaseDir, entry.Name())
		state, err := ReadState(runtimeSession)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if state.ID == "" {
			state.ID = runtimeSession.ID
		}
		if state.ID != runtimeSession.ID {
			continue
		}

		records = append(records, Record{Session: runtimeSession, State: state})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].State.CreatedAt.Before(records[j].State.CreatedAt)
	})

	return records, nil
}

func (m Manager) Find(id string) (Record, error) {
	if m.BaseDir == "" {
		m.BaseDir = RuntimeBaseDir(os.Getenv("XDG_RUNTIME_DIR"), m.uid())
	}
	if !validSessionID(id) {
		return Record{}, fmt.Errorf("invalid session id %q", id)
	}

	runtimeSession := newSession(m.BaseDir, id)
	state, err := ReadState(runtimeSession)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, fmt.Errorf("unknown session %q: no Sidequest metadata found", id)
	}
	if err != nil {
		return Record{}, err
	}
	if state.ID == "" {
		state.ID = id
	}
	if state.ID != id {
		return Record{}, fmt.Errorf("session %q metadata does not match requested id", id)
	}

	return Record{Session: runtimeSession, State: state}, nil
}

func WriteState(session Session, state State) error {
	if state.ID == "" {
		state.ID = session.ID
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	data = append(data, '\n')

	file, err := os.OpenFile(session.StatePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, regularStateFilePerm)
	if err != nil {
		return fmt.Errorf("open session state: %w", err)
	}

	writeErr := func() error {
		if _, err := file.Write(data); err != nil {
			return err
		}
		if err := file.Chmod(regularStateFilePerm); err != nil {
			return err
		}
		return file.Close()
	}()
	if writeErr != nil {
		_ = file.Close()
		return fmt.Errorf("write session state: %w", writeErr)
	}

	return nil
}

func ReadState(session Session) (State, error) {
	data, err := os.ReadFile(session.StatePath)
	if err != nil {
		return State{}, fmt.Errorf("read session state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode session state: %w", err)
	}

	return state, nil
}

func UpdateState(session Session, now time.Time, update func(*State)) error {
	state, err := ReadState(session)
	if err != nil {
		return err
	}

	update(&state)
	state.UpdatedAt = now.UTC()

	return WriteState(session, state)
}

func FromSocketPath(socketPath string) Session {
	dir := filepath.Dir(socketPath)
	id := filepath.Base(dir)
	return Session{
		ID:         id,
		Dir:        dir,
		StatePath:  filepath.Join(dir, DefaultStateFileName),
		SocketPath: socketPath,
	}
}

func Cleanup(session Session) error {
	if session.Dir == "" {
		return nil
	}
	return os.RemoveAll(session.Dir)
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, runtimeDirectoryPerm); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	if err := os.Chmod(path, runtimeDirectoryPerm); err != nil {
		return fmt.Errorf("secure runtime directory: %w", err)
	}
	return nil
}

func newSession(baseDir string, id string) Session {
	return Session{
		ID:         id,
		Dir:        filepath.Join(baseDir, id),
		StatePath:  filepath.Join(baseDir, id, DefaultStateFileName),
		SocketPath: filepath.Join(baseDir, id, DefaultCommandSocket),
	}
}

func randomID() (string, error) {
	var bytes [sessionIDRandomByteSize]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func validSessionID(id string) bool {
	if id == "" || strings.Contains(id, ".") {
		return false
	}
	for _, char := range id {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		if char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func (m Manager) uid() int {
	if m.UID > 0 {
		return m.UID
	}
	return os.Getuid()
}

func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
