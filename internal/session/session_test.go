package session

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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
		errc <- SendCommand(ctx, session.SocketPath, command)
	}()

	if _, err := listener.Receive(ctx); err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
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
	go func() {
		errc <- SendCommand(ctx, session.SocketPath, want)
	}()

	got, err := listener.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}

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
