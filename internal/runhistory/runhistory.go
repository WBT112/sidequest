package runhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/WBT112/sidequest/internal/session"
)

const (
	DefaultResultFileName = "result.json"
	DefaultOutputFileName = "output.txt"
	DefaultRetentionLimit = 100
	privateDirectoryPerm  = 0o700
	privateFilePerm       = 0o600
)

type Manager struct {
	BaseDir        string
	UID            int
	RetentionLimit int
	Pager          string
	Out            io.Writer
}

type Result struct {
	ID              string    `json:"id"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	DurationMillis  int64     `json:"duration_ms"`
	ExitCode        *int      `json:"exit_code,omitempty"`
	Termination     string    `json:"termination"`
	OutputTruncated bool      `json:"output_truncated"`
}

type Run struct {
	Result     Result
	Dir        string
	OutputPath string
	ResultPath string
}

func DefaultManager() Manager {
	home := os.Getenv("HOME")
	if home == "" {
		if userHome, err := os.UserHomeDir(); err == nil {
			home = userHome
		}
	}
	return Manager{
		BaseDir: StateBaseDir(os.Getenv("XDG_STATE_HOME"), home),
		UID:     os.Getuid(),
		Pager:   os.Getenv("PAGER"),
		Out:     os.Stdout,
	}
}

func StateBaseDir(xdgStateHome string, home string) string {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "sidequest")
	}
	return filepath.Join(home, ".local", "state", "sidequest")
}

func (m Manager) Store(record session.Record, output string, outputTruncated bool) (Run, error) {
	if record.Session.ID == "" {
		return Run{}, fmt.Errorf("missing run id")
	}
	if !validRunID(record.Session.ID) {
		return Run{}, fmt.Errorf("invalid run id %q", record.Session.ID)
	}
	result := resultFromRecord(record, outputTruncated)
	run := m.run(record.Session.ID)
	if err := ensureHistoryDirs(run.Dir, m.uid()); err != nil {
		return Run{}, err
	}
	if err := writePrivateFile(run.OutputPath, []byte(output)); err != nil {
		return Run{}, fmt.Errorf("write run output: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return Run{}, fmt.Errorf("encode run result: %w", err)
	}
	data = append(data, '\n')
	if err := writePrivateFile(run.ResultPath, data); err != nil {
		return Run{}, fmt.Errorf("write run result: %w", err)
	}
	run.Result = result
	if err := m.EnforceRetention(); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (m Manager) List() ([]Run, error) {
	runsDir := m.runsDir()
	entries, err := os.ReadDir(runsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read runs directory: %w", err)
	}

	runs := make([]Run, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validRunID(entry.Name()) {
			continue
		}
		run := m.run(entry.Name())
		result, err := readResult(run.ResultPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if result.ID != entry.Name() {
			continue
		}
		run.Result = result
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].Result.FinishedAt.After(runs[j].Result.FinishedAt)
	})
	return runs, nil
}

func (m Manager) Find(id string) (Run, error) {
	if id == "last" {
		return m.Last()
	}
	if !validRunID(id) {
		return Run{}, fmt.Errorf("invalid run id %q", id)
	}
	run := m.run(id)
	result, err := readResult(run.ResultPath)
	if errors.Is(err, os.ErrNotExist) {
		return Run{}, fmt.Errorf("unknown run %q", id)
	}
	if err != nil {
		return Run{}, err
	}
	if result.ID != id {
		return Run{}, fmt.Errorf("run %q metadata does not match requested id", id)
	}
	run.Result = result
	return run, nil
}

func (m Manager) Last() (Run, error) {
	runs, err := m.List()
	if err != nil {
		return Run{}, err
	}
	if len(runs) == 0 {
		return Run{}, fmt.Errorf("no stored runs")
	}
	return runs[0], nil
}

func (m Manager) Output(id string) error {
	run, err := m.Find(id)
	if err != nil {
		return err
	}
	if err := validateFile(run.OutputPath, m.uid()); err != nil {
		return err
	}
	pager := m.Pager
	if pager != "" {
		command := exec.Command(pager, run.OutputPath)
		command.Stdin = os.Stdin
		command.Stdout = m.outputWriter()
		command.Stderr = os.Stderr
		if err := command.Run(); err == nil {
			return nil
		}
	}
	data, err := os.ReadFile(run.OutputPath)
	if err != nil {
		return fmt.Errorf("read run output: %w", err)
	}
	_, err = m.outputWriter().Write(data)
	return err
}

func (m Manager) Purge(id string) error {
	if id == "last" {
		run, err := m.Last()
		if err != nil {
			return err
		}
		id = run.Result.ID
	}
	if !validRunID(id) {
		return fmt.Errorf("invalid run id %q", id)
	}
	run := m.run(id)
	if err := validateRunDir(run.Dir, m.uid(), m.runsDir()); err != nil {
		return err
	}
	return os.RemoveAll(run.Dir)
}

func (m Manager) EnforceRetention() error {
	limit := m.RetentionLimit
	if limit == 0 {
		limit = DefaultRetentionLimit
	}
	if limit < 0 {
		return nil
	}
	runs, err := m.List()
	if err != nil {
		return err
	}
	if len(runs) <= limit {
		return nil
	}
	for _, run := range runs[limit:] {
		if err := validateRunDir(run.Dir, m.uid(), m.runsDir()); err != nil {
			return err
		}
		if err := os.RemoveAll(run.Dir); err != nil {
			return fmt.Errorf("remove old run %q: %w", run.Result.ID, err)
		}
	}
	return nil
}

func (m Manager) run(id string) Run {
	dir := filepath.Join(m.runsDir(), id)
	return Run{
		Dir:        dir,
		OutputPath: filepath.Join(dir, DefaultOutputFileName),
		ResultPath: filepath.Join(dir, DefaultResultFileName),
	}
}

func (m Manager) runsDir() string {
	baseDir := m.BaseDir
	if baseDir == "" {
		baseDir = DefaultManager().BaseDir
	}
	return filepath.Join(baseDir, "runs")
}

func (m Manager) outputWriter() io.Writer {
	if m.Out != nil {
		return m.Out
	}
	return io.Discard
}

func (m Manager) uid() int {
	if m.UID > 0 {
		return m.UID
	}
	return os.Getuid()
}

func resultFromRecord(record session.Record, outputTruncated bool) Result {
	startedAt := record.State.CreatedAt
	if record.State.StartedAt != nil {
		startedAt = *record.State.StartedAt
	}
	finishedAt := record.State.UpdatedAt
	if record.State.FinishedAt != nil {
		finishedAt = *record.State.FinishedAt
	}
	durationMillis := finishedAt.Sub(startedAt).Milliseconds()
	if record.State.DurationMillis != nil {
		durationMillis = *record.State.DurationMillis
	}
	if durationMillis < 0 {
		durationMillis = 0
	}
	return Result{
		ID:              record.Session.ID,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		DurationMillis:  durationMillis,
		ExitCode:        record.State.ExitCode,
		Termination:     record.State.Status,
		OutputTruncated: outputTruncated,
	}
}

func ensureHistoryDirs(runDir string, uid int) error {
	runsDir := filepath.Dir(runDir)
	baseDir := filepath.Dir(runsDir)
	if err := ensureParentDirs(baseDir); err != nil {
		return err
	}
	for _, dir := range []string{baseDir, runsDir, runDir} {
		if err := ensurePrivateDir(dir, uid); err != nil {
			return err
		}
	}
	return nil
}

func ensureParentDirs(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent := filepath.Dir(absolute)
	volume := filepath.VolumeName(parent)
	rest := strings.TrimPrefix(parent, volume)
	rest = strings.TrimPrefix(rest, string(os.PathSeparator))
	current := volume
	if strings.HasPrefix(parent, string(os.PathSeparator)) {
		current += string(os.PathSeparator)
	}
	if rest == "" {
		return nil
	}
	for _, part := range strings.Split(rest, string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, privateDirectoryPerm); err != nil {
				return fmt.Errorf("create state parent directory %q: %w", current, err)
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q is a symlink", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", current)
		}
	}
	return nil
}

func ensurePrivateDir(path string, uid int) error {
	if err := os.Mkdir(path, privateDirectoryPerm); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create private directory %q: %w", path, err)
	}
	if err := validateDir(path, uid); err != nil {
		return err
	}
	return os.Chmod(path, privateDirectoryPerm)
}

func writePrivateFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateFilePerm)
	if err != nil {
		return err
	}
	writeErr := func() error {
		if _, err := file.Write(data); err != nil {
			return err
		}
		if err := file.Chmod(privateFilePerm); err != nil {
			return err
		}
		return file.Close()
	}()
	if writeErr != nil {
		_ = file.Close()
		return writeErr
	}
	return nil
}

func readResult(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, fmt.Errorf("decode run result: %w", err)
	}
	return result, nil
}

func validateRunDir(path string, uid int, runsDir string) error {
	cleanRunsDir, err := filepath.Abs(runsDir)
	if err != nil {
		return err
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(cleanRunsDir, cleanPath)
	if err != nil {
		return err
	}
	if relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || relative == ".." || filepath.IsAbs(relative) {
		return fmt.Errorf("run path %q is outside Sidequest state root", path)
	}
	if filepath.Base(cleanPath) != relative {
		return fmt.Errorf("run path %q is not a direct Sidequest run directory", path)
	}
	if err := validateDir(cleanRunsDir, uid); err != nil {
		return err
	}
	return validateDir(cleanPath, uid)
}

func validateDir(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", path)
	}
	if err := validateOwner(info, uid); err != nil {
		return fmt.Errorf("validate directory owner: %w", err)
	}
	return nil
}

func validateFile(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}
	if err := validateOwner(info, uid); err != nil {
		return fmt.Errorf("validate file owner: %w", err)
	}
	return nil
}

func validateOwner(info os.FileInfo, uid int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unsupported file metadata")
	}
	if int(stat.Uid) != uid {
		return fmt.Errorf("owned by uid %d, want %d", stat.Uid, uid)
	}
	return nil
}

func validRunID(id string) bool {
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

func FormatDuration(milliseconds int64) string {
	if milliseconds < 0 {
		milliseconds = 0
	}
	duration := time.Duration(milliseconds) * time.Millisecond
	totalSeconds := int(duration.Round(time.Second).Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func FormatExitCode(exitCode *int) string {
	if exitCode == nil {
		return "-"
	}
	return strconv.Itoa(*exitCode)
}
