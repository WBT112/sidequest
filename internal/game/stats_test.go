package game

import (
	"encoding/json"
	"os"
	"os/user"
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

func TestDefaultPlayerNameUsesLocalUserAndHostname(t *testing.T) {
	manager := StatsManager{
		CurrentUser: func() (*user.User, error) {
			return &user.User{Username: "admin"}, nil
		},
		Hostname: func() (string, error) {
			return "workstation", nil
		},
	}

	if got := manager.DefaultPlayerName(); got != "admin@workstation" {
		t.Fatalf("DefaultPlayerName = %q, want admin@workstation", got)
	}
}

func TestDefaultPlayerNameFallsBackWithoutHostname(t *testing.T) {
	manager := StatsManager{
		CurrentUser: func() (*user.User, error) {
			return &user.User{Username: "alex"}, nil
		},
		Hostname: func() (string, error) {
			return "", os.ErrInvalid
		},
	}

	if got := manager.DefaultPlayerName(); got != "alex" {
		t.Fatalf("DefaultPlayerName = %q, want alex", got)
	}
}

func TestDefaultPlayerNameFallsBackToYou(t *testing.T) {
	manager := StatsManager{
		CurrentUser: func() (*user.User, error) {
			return nil, os.ErrInvalid
		},
		Hostname: func() (string, error) {
			return "", os.ErrInvalid
		},
	}

	if got := manager.DefaultPlayerName(); got != "YOU" {
		t.Fatalf("DefaultPlayerName = %q, want YOU", got)
	}
}

func TestLeaderboardInsertionAndModeSeparation(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}

	if _, rank, err := manager.AddLeaderboardScore(GameModeClassic, 100, "classic"); err != nil || rank != 1 {
		t.Fatalf("classic AddLeaderboardScore rank=%d err=%v", rank, err)
	}
	if _, rank, err := manager.AddLeaderboardScore(GameModeQuest, 200, "quest"); err != nil || rank != 1 {
		t.Fatalf("quest AddLeaderboardScore rank=%d err=%v", rank, err)
	}

	classic := manager.Leaderboard(GameModeClassic)
	quest := manager.Leaderboard(GameModeQuest)
	if len(classic) != 1 || classic[0].PlayerName != "classic" {
		t.Fatalf("classic leaderboard = %#v", classic)
	}
	if len(quest) != 1 || quest[0].PlayerName != "quest" {
		t.Fatalf("quest leaderboard = %#v", quest)
	}
}

func TestLeaderboardCapsAtFiveAndKeepsDuplicateScores(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	for index, score := range []int{500, 400, 300, 200, 100} {
		if _, _, err := manager.AddLeaderboardScore(GameModeClassic, score, string(rune('a'+index))); err != nil {
			t.Fatalf("AddLeaderboardScore returned error: %v", err)
		}
	}

	if rank := manager.QualifyingRank(GameModeClassic, 99); rank != 0 {
		t.Fatalf("rank for below fifth = %d, want 0", rank)
	}
	if _, rank, err := manager.AddLeaderboardScore(GameModeClassic, 300, "dup"); err != nil || rank != 4 {
		t.Fatalf("duplicate AddLeaderboardScore rank=%d err=%v", rank, err)
	}
	entries := manager.Leaderboard(GameModeClassic)
	if len(entries) != 5 {
		t.Fatalf("leaderboard length = %d, want 5", len(entries))
	}
	if entries[2].Score != 300 || entries[3].PlayerName != "dup" {
		t.Fatalf("duplicate insertion order = %#v", entries)
	}
}

func TestLeaderboardKeepsCutoffTieVisible(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	for index, score := range []int{500, 400, 300, 200, 100} {
		if _, _, err := manager.AddLeaderboardScore(GameModeClassic, score, string(rune('a'+index))); err != nil {
			t.Fatalf("AddLeaderboardScore returned error: %v", err)
		}
	}

	if rank := manager.QualifyingRank(GameModeClassic, 100); rank != 5 {
		t.Fatalf("QualifyingRank cutoff tie = %d, want 5", rank)
	}
	if _, rank, err := manager.AddLeaderboardScore(GameModeClassic, 100, "cutoff"); err != nil || rank != 5 {
		t.Fatalf("cutoff tie AddLeaderboardScore rank=%d err=%v", rank, err)
	}
	entries := manager.Leaderboard(GameModeClassic)
	if len(entries) != 5 {
		t.Fatalf("leaderboard length = %d, want 5", len(entries))
	}
	if entries[4] != (LeaderboardEntry{Score: 100, PlayerName: "cutoff"}) {
		t.Fatalf("fifth entry = %#v, want saved cutoff tie", entries[4])
	}
}

func TestLeaderboardCutoffTieCanBeRepeated(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	for index, score := range []int{500, 400, 300, 200, 100} {
		if _, _, err := manager.AddLeaderboardScore(GameModeQuest, score, string(rune('a'+index))); err != nil {
			t.Fatalf("AddLeaderboardScore returned error: %v", err)
		}
	}

	for _, name := range []string{"first", "second"} {
		if _, rank, err := manager.AddLeaderboardScore(GameModeQuest, 100, name); err != nil || rank != 5 {
			t.Fatalf("cutoff tie %q rank=%d err=%v", name, rank, err)
		}
	}
	entries := manager.Leaderboard(GameModeQuest)
	if len(entries) != 5 {
		t.Fatalf("leaderboard length = %d, want 5", len(entries))
	}
	if entries[4] != (LeaderboardEntry{Score: 100, PlayerName: "second"}) {
		t.Fatalf("fifth entry = %#v, want latest cutoff tie", entries[4])
	}
	if rank := manager.QualifyingRank(GameModeQuest, 99); rank != 0 {
		t.Fatalf("QualifyingRank below cutoff = %d, want 0", rank)
	}
}

func TestLeaderboardKeepsStableEqualScoresAboveCutoff(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	for index, score := range []int{500, 400, 300, 200, 100} {
		if _, _, err := manager.AddLeaderboardScore(GameModeClassic, score, string(rune('a'+index))); err != nil {
			t.Fatalf("AddLeaderboardScore returned error: %v", err)
		}
	}

	if _, rank, err := manager.AddLeaderboardScore(GameModeClassic, 300, "equal"); err != nil || rank != 4 {
		t.Fatalf("equal score rank=%d err=%v", rank, err)
	}
	entries := manager.Leaderboard(GameModeClassic)
	if entries[2] != (LeaderboardEntry{Score: 300, PlayerName: "c"}) || entries[3] != (LeaderboardEntry{Score: 300, PlayerName: "equal"}) {
		t.Fatalf("equal score ordering = %#v", entries)
	}
}

func TestStatsMigratesBestScoreToQuestTop5(t *testing.T) {
	manager := StatsManager{BaseDir: filepath.Join(t.TempDir(), "sidequest")}
	if err := os.MkdirAll(manager.BaseDir, 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	old := `{"schema_version":1,"best_score":1234,"games_played":2}`
	if err := os.WriteFile(filepath.Join(manager.BaseDir, statsFileName), []byte(old), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	entries := manager.Leaderboard(GameModeQuest)
	if len(entries) != 1 || entries[0].Score != 1234 || entries[0].PlayerName != "YOU" {
		t.Fatalf("migrated quest leaderboard = %#v", entries)
	}
}

func TestNormalizePlayerNameRejectsControlsAndTruncates(t *testing.T) {
	got := NormalizePlayerName("  abc\t\u001bdef" + strings.Repeat("x", 40))
	if strings.ContainsAny(got, "\t\u001b") {
		t.Fatalf("normalized name contains controls: %q", got)
	}
	if textDisplayWidth(got) > maxPlayerNameColumns {
		t.Fatalf("normalized width = %d, want <= %d", textDisplayWidth(got), maxPlayerNameColumns)
	}
	if empty := NormalizePlayerName("\n\t"); empty != "YOU" {
		t.Fatalf("empty normalized name = %q, want YOU", empty)
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
