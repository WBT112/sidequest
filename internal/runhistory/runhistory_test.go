package runhistory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WBT112/sidequest/internal/session"
)

func TestStoreWritesOutputResultAndPrivatePermissions(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state", "sidequest")
	exitCode := 7
	started := time.Date(2026, 7, 11, 18, 38, 40, 0, time.UTC)
	finished := started.Add(3 * time.Second)
	durationMillis := int64(3000)
	manager := Manager{BaseDir: base, UID: os.Getuid(), RetentionLimit: -1}

	run, err := manager.Store(session.Record{
		Session: session.Session{ID: "brave-otter"},
		State: session.State{
			ID:             "brave-otter",
			Status:         session.StatusFailed,
			CreatedAt:      started.Add(-time.Minute),
			StartedAt:      &started,
			FinishedAt:     &finished,
			DurationMillis: &durationMillis,
			ExitCode:       &exitCode,
		},
	}, "visible output\n", true)
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	assertPerm(t, base, 0o700)
	assertPerm(t, filepath.Join(base, "runs"), 0o700)
	assertPerm(t, run.Dir, 0o700)
	assertPerm(t, run.OutputPath, 0o600)
	assertPerm(t, run.ResultPath, 0o600)

	output, err := os.ReadFile(run.OutputPath)
	if err != nil {
		t.Fatalf("ReadFile output returned error: %v", err)
	}
	if string(output) != "visible output\n" {
		t.Fatalf("output = %q", output)
	}

	data, err := os.ReadFile(run.ResultPath)
	if err != nil {
		t.Fatalf("ReadFile result returned error: %v", err)
	}
	if strings.Contains(string(data), "bash") || strings.Contains(string(data), "secret") {
		t.Fatalf("result contains command-like data:\n%s", data)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal result returned error: %v", err)
	}
	if result.ID != "brave-otter" || result.Termination != session.StatusFailed {
		t.Fatalf("result = %#v", result)
	}
	if result.ExitCode == nil || *result.ExitCode != exitCode {
		t.Fatalf("exit code = %#v, want %d", result.ExitCode, exitCode)
	}
	if !result.OutputTruncated {
		t.Fatal("OutputTruncated = false, want true")
	}
}

func TestListFindLastAndOutput(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state", "sidequest")
	manager := Manager{BaseDir: base, UID: os.Getuid(), RetentionLimit: -1}
	storeRun(t, manager, "older", time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC), "old\n")
	storeRun(t, manager, "newer", time.Date(2026, 7, 11, 19, 0, 0, 0, time.UTC), "new\n")

	runs, err := manager.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if got, want := []string{runs[0].Result.ID, runs[1].Result.ID}, []string{"newer", "older"}; got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("runs = %#v, want %#v", got, want)
	}

	last, err := manager.Find("last")
	if err != nil {
		t.Fatalf("Find last returned error: %v", err)
	}
	if last.Result.ID != "newer" {
		t.Fatalf("last = %q, want newer", last.Result.ID)
	}

	var out strings.Builder
	manager.Out = &out
	if err := manager.Output("last"); err != nil {
		t.Fatalf("Output last returned error: %v", err)
	}
	if out.String() != "new\n" {
		t.Fatalf("output = %q, want new", out.String())
	}
}

func TestPurgeRemovesOnlyValidatedRunDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state", "sidequest")
	manager := Manager{BaseDir: base, UID: os.Getuid(), RetentionLimit: -1}
	run := storeRun(t, manager, "gone", time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC), "bye\n")

	if err := manager.Purge("gone"); err != nil {
		t.Fatalf("Purge returned error: %v", err)
	}
	if _, err := os.Stat(run.Dir); !os.IsNotExist(err) {
		t.Fatalf("purged run stat error = %v, want not exist", err)
	}

	if err := manager.Purge("../outside"); err == nil {
		t.Fatal("Purge accepted path traversal id")
	}
}

func TestRetentionKeepsNewestRuns(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state", "sidequest")
	manager := Manager{BaseDir: base, UID: os.Getuid(), RetentionLimit: 2}
	storeRun(t, manager, "one", time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC), "one\n")
	storeRun(t, manager, "two", time.Date(2026, 7, 11, 19, 0, 0, 0, time.UTC), "two\n")
	storeRun(t, manager, "three", time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC), "three\n")

	runs, err := manager.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].Result.ID != "three" || runs[1].Result.ID != "two" {
		t.Fatalf("runs = %#v, want three,two", []string{runs[0].Result.ID, runs[1].Result.ID})
	}
	if _, err := os.Stat(filepath.Join(base, "runs", "one")); !os.IsNotExist(err) {
		t.Fatalf("old run stat error = %v, want not exist", err)
	}
}

func TestDefaultRetentionKeepsNewestHundredRuns(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state", "sidequest")
	manager := Manager{BaseDir: base, UID: os.Getuid()}
	start := time.Date(2026, 7, 11, 18, 0, 0, 0, time.UTC)
	for index := 0; index < DefaultRetentionLimit+1; index++ {
		id := "run-" + strings.Repeat("a", index/26) + string(rune('a'+index%26))
		storeRun(t, manager, id, start.Add(time.Duration(index)*time.Minute), id+"\n")
	}

	runs, err := manager.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(runs) != DefaultRetentionLimit {
		t.Fatalf("len(runs) = %d, want %d", len(runs), DefaultRetentionLimit)
	}
	if _, err := os.Stat(filepath.Join(base, "runs", "run-a")); !os.IsNotExist(err) {
		t.Fatalf("oldest run stat error = %v, want not exist", err)
	}
}

func TestStoreRejectsSymlinkStateRoot(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir target returned error: %v", err)
	}
	link := filepath.Join(tmp, "state", "sidequest")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatalf("MkdirAll state returned error: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}

	manager := Manager{BaseDir: link, UID: os.Getuid(), RetentionLimit: -1}
	_, err := manager.Store(session.Record{
		Session: session.Session{ID: "blocked"},
		State:   session.State{ID: "blocked", Status: session.StatusCompleted, CreatedAt: time.Now()},
	}, "", false)
	if err == nil {
		t.Fatal("Store accepted symlink state root")
	}
}

func storeRun(t *testing.T, manager Manager, id string, finished time.Time, output string) Run {
	t.Helper()
	started := finished.Add(-time.Second)
	durationMillis := int64(1000)
	exitCode := 0
	run, err := manager.Store(session.Record{
		Session: session.Session{ID: id},
		State: session.State{
			ID:             id,
			Status:         session.StatusCompleted,
			CreatedAt:      started,
			StartedAt:      &started,
			FinishedAt:     &finished,
			DurationMillis: &durationMillis,
			ExitCode:       &exitCode,
		},
	}, output, false)
	if err != nil {
		t.Fatalf("Store %q returned error: %v", id, err)
	}
	return run
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat %s returned error: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s perm = %#o, want %#o", path, got, want)
	}
}
