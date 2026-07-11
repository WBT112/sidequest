package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	statsFileName        = "game-stats.json"
	statsSchemaVersion   = 1
	statsDirectoryPerm   = 0o700
	statsRegularFilePerm = 0o600
)

type Stats struct {
	SchemaVersion        int      `json:"schema_version"`
	GamesPlayed          int      `json:"games_played"`
	BestScore            int      `json:"best_score"`
	LongestCombo         int      `json:"longest_combo"`
	FoodCollected        int      `json:"food_collected"`
	GoldenBytesCollected int      `json:"golden_bytes_collected"`
	Achievements         []string `json:"achievements"`
}

type StatsManager struct {
	BaseDir string
}

func DefaultStatsManager() StatsManager {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			if userHome, err := os.UserHomeDir(); err == nil {
				home = userHome
			}
		}
		base = filepath.Join(home, ".local", "state")
	}
	return StatsManager{BaseDir: filepath.Join(base, "sidequest")}
}

func (m StatsManager) UpdateQuest(result ScoreBreakdown, quest *QuestState) (Stats, error) {
	if quest == nil || !quest.Enabled() {
		return Stats{}, nil
	}
	stats := m.load()
	stats.SchemaVersion = statsSchemaVersion
	stats.GamesPlayed++
	if result.FinalScore > stats.BestScore {
		stats.BestScore = result.FinalScore
	}
	if quest.MaxCombo > stats.LongestCombo {
		stats.LongestCombo = quest.MaxCombo
	}
	stats.FoodCollected += quest.NormalFood
	stats.GoldenBytesCollected += quest.GoldenCollected
	stats.Achievements = mergeAchievements(stats.Achievements, achievementsFor(result, quest)...)
	if err := m.write(stats); err != nil {
		return Stats{}, err
	}
	return stats, nil
}

func (m StatsManager) path() string {
	baseDir := m.BaseDir
	if baseDir == "" {
		baseDir = DefaultStatsManager().BaseDir
	}
	return filepath.Join(baseDir, statsFileName)
}

func (m StatsManager) load() Stats {
	data, err := os.ReadFile(m.path())
	if err != nil {
		return Stats{SchemaVersion: statsSchemaVersion}
	}
	var stats Stats
	if err := json.Unmarshal(data, &stats); err != nil || stats.SchemaVersion != statsSchemaVersion {
		return Stats{SchemaVersion: statsSchemaVersion}
	}
	return stats
}

func (m StatsManager) write(stats Stats) error {
	path := m.path()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, statsDirectoryPerm); err != nil {
		return fmt.Errorf("create stats directory: %w", err)
	}
	if err := os.Chmod(dir, statsDirectoryPerm); err != nil {
		return fmt.Errorf("secure stats directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to replace symlink stats file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, statsRegularFilePerm)
	if err != nil {
		return err
	}
	writeErr := func() error {
		if _, err := tmp.Write(data); err != nil {
			return err
		}
		if err := tmp.Chmod(statsRegularFilePerm); err != nil {
			return err
		}
		return tmp.Close()
	}()
	if writeErr != nil {
		_ = tmp.Close()
		_ = os.Remove(path + ".tmp")
		return writeErr
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		_ = os.Remove(path + ".tmp")
		return err
	}
	return os.Chmod(path, statsRegularFilePerm)
}

func achievementsFor(result ScoreBreakdown, quest *QuestState) []string {
	achievements := []string{"hello_world"}
	if result.SurvivalAwarded {
		achievements = append(achievements, "zero_downtime")
	}
	if quest.MaxCombo >= 5 {
		achievements = append(achievements, "combo_5")
	}
	if quest.GoldenCollected > 0 {
		achievements = append(achievements, "golden_byte")
	}
	if quest.MaxHeat >= 6 {
		achievements = append(achievements, "heat_6")
	}
	return achievements
}

func mergeAchievements(existing []string, added ...string) []string {
	set := make(map[string]bool, len(existing)+len(added))
	for _, id := range existing {
		if id != "" {
			set[id] = true
		}
	}
	for _, id := range added {
		if id != "" {
			set[id] = true
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		if strings.TrimSpace(id) != "" {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}
