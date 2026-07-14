package session

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRuntimeBaseDirUsesXDGWhenAvailable(t *testing.T) {
	got := RuntimeBaseDir("/run/user/1000", 1000)
	want := filepath.Join("/run/user/1000", "sidequest")
	if got != want {
		t.Fatalf("RuntimeBaseDir = %q, want %q", got, want)
	}
}

func TestRuntimeBaseDirFallsBackToUserSpecificTmpDir(t *testing.T) {
	got := RuntimeBaseDir("", 1000)
	want := filepath.Join(os.TempDir(), "sidequest-1000")
	if got != want {
		t.Fatalf("RuntimeBaseDir = %q, want %q", got, want)
	}
}

func TestCreateSessionPermissionsAndState(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sidequest")
	now := time.Date(2026, 7, 11, 10, 30, 0, 0, time.UTC)
	manager := Manager{
		BaseDir:     base,
		IDGenerator: fixedID("session-1"),
		Now:         func() time.Time { return now },
	}

	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	assertMode(t, base, 0o700)
	assertMode(t, session.Dir, 0o700)
	assertMode(t, session.StatePath, 0o600)

	data, err := os.ReadFile(session.StatePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}

	for _, fragment := range []string{`"id": "session-1"`, `"status": "created"`, `"created_at":`} {
		if !bytes.Contains(data, []byte(fragment)) {
			t.Fatalf("state file does not contain %q:\n%s", fragment, data)
		}
	}
}

func TestCreateRejectsInvalidGeneratedSessionID(t *testing.T) {
	manager := Manager{
		BaseDir:     filepath.Join(t.TempDir(), "sidequest"),
		IDGenerator: fixedID("../not-owned"),
	}

	_, err := manager.Create()
	if err == nil {
		t.Fatal("Create succeeded, want invalid session id error")
	}
	if !strings.Contains(err.Error(), "invalid session id") {
		t.Fatalf("Create error = %v, want invalid session id", err)
	}
}

func TestCommandIsNeverWrittenToRuntimeFiles(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("secret-test")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	command := Command{
		Executable: "printf-secret",
		Arguments:  []string{"token=secret", "|", ">", "*.go"},
	}
	listener, err := ListenCommand(session)
	if err != nil {
		t.Fatalf("ListenCommand returned error: %v", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		_, err := ReceiveCommand(ctx, session.SocketPath)
		errc <- err
	}()

	if _, err := listener.Serve(ctx, command); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("ReceiveCommand returned error: %v", err)
	}

	assertRuntimeFilesDoNotContain(t, session.Dir, "printf-secret", "token=secret", "*.go")
}

func TestCommandHandoffPreservesArgumentArray(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("handoff")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	listener, err := ListenCommand(session)
	if err != nil {
		t.Fatalf("ListenCommand returned error: %v", err)
	}
	defer listener.Close()

	want := Command{
		Executable: "printf",
		Arguments:  []string{"%s\n", "|", ">", "*.go", "$HOME"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errc := make(chan error, 1)
	gotc := make(chan Command, 1)
	go func() {
		got, err := ReceiveCommand(ctx, session.SocketPath)
		if err == nil {
			gotc <- got
		}
		errc <- err
	}()

	if _, err := listener.Serve(ctx, want); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("ReceiveCommand returned error: %v", err)
	}
	got := <-gotc

	if got.Executable != want.Executable {
		t.Fatalf("Executable = %q, want %q", got.Executable, want.Executable)
	}
	if !equalStrings(got.Arguments, want.Arguments) {
		t.Fatalf("Arguments = %#v, want %#v", got.Arguments, want.Arguments)
	}
}

func TestListenCommandRemovesStaleSocketPath(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("stale")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := os.WriteFile(session.SocketPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale socket placeholder: %v", err)
	}

	listener, err := ListenCommand(session)
	if err != nil {
		t.Fatalf("ListenCommand returned error: %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(session.SocketPath)
	if err != nil {
		t.Fatalf("stat socket path: %v", err)
	}
	if info.Mode().Type()&os.ModeSocket == 0 {
		t.Fatalf("socket path mode = %v, want Unix socket", info.Mode())
	}
}

func TestCreateDoesNotRemoveExistingSessionOnIDCollision(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := Manager{BaseDir: base, IDGenerator: fixedID("reused")}
	first, err := manager.Create()
	if err != nil {
		t.Fatalf("first Create returned error: %v", err)
	}
	stalePath := filepath.Join(first.Dir, "stale.tmp")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	_, err = manager.Create()
	if err == nil {
		t.Fatal("second Create succeeded, want ID collision error")
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("existing session file was removed: %v", err)
	}
}

func TestCleanupRemovesSessionRuntimeDirectory(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("cleanup")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := Cleanup(session); err != nil {
		t.Fatalf("Cleanup returned error: %v", err)
	}
	if _, err := os.Stat(session.Dir); !IsNotExist(err) {
		t.Fatalf("session dir stat error = %v, want not exist", err)
	}
}

func TestCleanupFinishedRemovesOnlyTerminalRecordsSelectedAsStale(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := Manager{BaseDir: base, IDGenerator: fixedID("done")}
	done, err := manager.Create()
	if err != nil {
		t.Fatalf("Create done returned error: %v", err)
	}
	if err := UpdateState(done, time.Now(), func(state *State) {
		state.Status = StatusCompleted
	}); err != nil {
		t.Fatalf("UpdateState done returned error: %v", err)
	}

	manager.IDGenerator = fixedID("running")
	running, err := manager.Create()
	if err != nil {
		t.Fatalf("Create running returned error: %v", err)
	}
	if err := UpdateState(running, time.Now(), func(state *State) {
		state.Status = StatusRunning
	}); err != nil {
		t.Fatalf("UpdateState running returned error: %v", err)
	}

	removed, err := manager.CleanupFinished(func(record Record) bool {
		return record.Session.ID == "done"
	})
	if err != nil {
		t.Fatalf("CleanupFinished returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(done.Dir); !IsNotExist(err) {
		t.Fatalf("done dir stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(running.Dir); err != nil {
		t.Fatalf("running dir was removed: %v", err)
	}
}

func TestUpdateStatePersistsTmuxSocketWithoutCommand(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("state-update")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	now := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	err = UpdateState(session, now, func(state *State) {
		state.TmuxSocket = "sidequest-state-update"
	})
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	state, err := ReadState(session)
	if err != nil {
		t.Fatalf("ReadState returned error: %v", err)
	}
	if state.TmuxSocket != "sidequest-state-update" {
		t.Fatalf("TmuxSocket = %q, want %q", state.TmuxSocket, "sidequest-state-update")
	}

	assertRuntimeFilesDoNotContain(t, session.Dir, "bash", "sleep 30", "exit 7")
}

func TestWriteStateUsesAtomicReplacementAndPrivatePermissions(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("atomic-state")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := WriteState(session, State{ID: session.ID, Status: StatusRunning}); err != nil {
		t.Fatalf("WriteState returned error: %v", err)
	}

	state, err := ReadState(session)
	if err != nil {
		t.Fatalf("ReadState returned error: %v", err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("Status = %q, want %q", state.Status, StatusRunning)
	}
	assertMode(t, session.StatePath, regularStateFilePerm)
	assertNoStateTempFiles(t, session.Dir)
}

func TestWriteStateRemovesTemporaryFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	session := Session{
		ID:        "bad-state",
		Dir:       dir,
		StatePath: filepath.Join(dir, "missing", DefaultStateFileName),
	}

	err := WriteState(session, State{Status: StatusRunning})
	if err == nil {
		t.Fatal("WriteState succeeded, want error")
	}
	assertNoStateTempFiles(t, dir)
}

func TestConcurrentReadStateNeverObservesPartialWrite(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("concurrent-state")}
	session, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	stop := make(chan struct{})
	errc := make(chan error, 1)
	var readers sync.WaitGroup
	for reader := 0; reader < 4; reader++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := ReadState(session); err != nil {
					select {
					case errc <- err:
					default:
					}
					return
				}
			}
		}()
	}

	for index := 0; index < 500; index++ {
		err := UpdateState(session, time.Now(), func(state *State) {
			state.Status = StatusRunning
			state.TmuxSocket = strings.Repeat("x", 1024)
			if index%2 == 0 {
				state.Status = StatusCompleted
				state.TmuxSocket = strings.Repeat("y", 1024)
			}
		})
		if err != nil {
			close(stop)
			readers.Wait()
			t.Fatalf("UpdateState returned error: %v", err)
		}
		select {
		case err := <-errc:
			close(stop)
			readers.Wait()
			t.Fatalf("ReadState observed partial write: %v", err)
		default:
		}
	}

	close(stop)
	readers.Wait()
	select {
	case err := <-errc:
		t.Fatalf("ReadState observed partial write: %v", err)
	default:
	}
	assertNoStateTempFiles(t, session.Dir)
}

func TestListReadsOnlySessionMetadata(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sidequest")
	manager := Manager{
		BaseDir:     base,
		IDGenerator: fixedID("brave-otter"),
		Now: func() time.Time {
			return time.Date(2026, 7, 11, 16, 40, 12, 0, time.UTC)
		},
	}
	created, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "not-a-session.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatalf("write unrelated runtime file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(base, "missing-state"), 0o700); err != nil {
		t.Fatalf("create missing-state dir: %v", err)
	}

	records, err := manager.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("List returned %d records, want 1: %#v", len(records), records)
	}
	if records[0].Session.ID != created.ID || records[0].State.ID != created.ID {
		t.Fatalf("record = %#v, want session %q", records[0], created.ID)
	}
}

func TestFindReturnsUnknownSessionError(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}

	_, err := manager.Find("missing")
	if err == nil {
		t.Fatal("Find succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unknown session") {
		t.Fatalf("Find error = %v, want unknown session", err)
	}
}

func TestFindReturnsExistingSession(t *testing.T) {
	manager := Manager{BaseDir: filepath.Join(t.TempDir(), "sidequest"), IDGenerator: fixedID("quiet-fox")}
	created, err := manager.Create()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	record, err := manager.Find("quiet-fox")
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if record.Session.ID != created.ID || record.State.ID != created.ID {
		t.Fatalf("record = %#v, want session %q", record, created.ID)
	}
}

func fixedID(id string) func() (string, error) {
	return func() (string, error) {
		return id, nil
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}

func assertNoStateTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".state-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary state file was not removed: %s", filepath.Join(dir, entry.Name()))
		}
	}
}

func assertRuntimeFilesDoNotContain(t *testing.T, root string, values ...string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range values {
			if strings.Contains(string(data), value) {
				t.Fatalf("runtime file %s contains command value %q:\n%s", path, value, data)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk runtime files: %v", err)
	}
}

func equalStrings(a, b []string) bool {
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
