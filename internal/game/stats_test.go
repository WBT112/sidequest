package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatsUpdateWritesPrivateCommandFreeData(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	quest := NewQuestState(GameModeQuest, time.Now(), fixedRandom(0), 8, 8)
	quest.MaxCombo = 5
	quest.NormalFood = 12
	quest.GoldenCollected = 1
	quest.MaxHeat = 6

	stats, err := manager.UpdateQuest(ScoreBreakdown{FinalScore: 7720, SurvivalAwarded: true}, quest)
	if err != nil {
		t.Fatalf("UpdateQuest returned error: %v", err)
	}
	if stats.GamesPlayed != 1 || stats.BestScore != 7720 || stats.LongestCombo != 5 {
		t.Fatalf("stats = %#v", stats)
	}
	data, err := os.ReadFile(filepath.Join(manager.BaseDir, statsFileName))
	if err != nil {
		t.Fatalf("ReadFile stats returned error: %v", err)
	}
	for _, forbidden := range []string{"command", "arguments", "output", "hostname", "working"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("stats contain forbidden field %q:\n%s", forbidden, data)
		}
	}
	var decoded Stats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal stats returned error: %v", err)
	}
	assertFilePerm(t, manager.BaseDir, 0o700)
	assertFilePerm(t, filepath.Join(manager.BaseDir, statsFileName), 0o600)
}

func TestStatsCorruptFileFailsSafely(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	if err := os.MkdirAll(manager.BaseDir, 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manager.BaseDir, statsFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	quest := NewQuestState(GameModeQuest, time.Now(), fixedRandom(0), 8, 8)

	stats, err := manager.UpdateQuest(ScoreBreakdown{FinalScore: 10}, quest)
	if err != nil {
		t.Fatalf("UpdateQuest returned error: %v", err)
	}
	if stats.GamesPlayed != 1 {
		t.Fatalf("GamesPlayed = %d, want 1", stats.GamesPlayed)
	}
}

func TestStatsRejectsSymlinkFile(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	if err := os.MkdirAll(manager.BaseDir, 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile target returned error: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(manager.BaseDir, statsFileName)); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	quest := NewQuestState(GameModeQuest, time.Now(), fixedRandom(0), 8, 8)

	if _, err := manager.UpdateQuest(ScoreBreakdown{FinalScore: 10}, quest); err == nil {
		t.Fatal("UpdateQuest accepted symlink stats file")
	}
}

func assertFilePerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat %s returned error: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s perm = %#o, want %#o", path, got, want)
	}
}
