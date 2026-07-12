package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	statsFileName        = "game-stats.json"
	statsSchemaVersion   = 2
	statsDirectoryPerm   = 0o700
	statsRegularFilePerm = 0o600
	leaderboardLimit     = 5
	maxPlayerNameColumns = 32
)

type LeaderboardEntry struct {
	Score      int    `json:"score"`
	PlayerName string `json:"player_name"`
}

type Stats struct {
	SchemaVersion        int                `json:"schema_version"`
	ClassicTop5          []LeaderboardEntry `json:"classic_top_5"`
	QuestTop5            []LeaderboardEntry `json:"quest_top_5"`
	GamesPlayed          int                `json:"games_played"`
	BestScore            int                `json:"best_score"`
	LongestCombo         int                `json:"longest_combo"`
	FoodCollected        int                `json:"food_collected"`
	GoldenBytesCollected int                `json:"golden_bytes_collected"`
	Achievements         []string           `json:"achievements"`
}

type StatsManager struct {
	BaseDir     string
	CurrentUser func() (*user.User, error)
	Hostname    func() (string, error)
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

func (m StatsManager) QualifyingRank(mode string, score int) int {
	stats := m.load()
	return leaderboardRank(top5ForMode(stats, mode), score)
}

func (m StatsManager) AddLeaderboardScore(mode string, score int, playerName string) (Stats, int, error) {
	stats := m.load()
	entry := LeaderboardEntry{Score: score, PlayerName: NormalizePlayerName(playerName)}
	entries, rank := insertLeaderboardEntry(top5ForMode(stats, mode), entry)
	if rank == 0 {
		return stats, 0, nil
	}
	setTop5ForMode(&stats, mode, entries)
	if err := m.write(stats); err != nil {
		return Stats{}, 0, err
	}
	return stats, rank, nil
}

func (m StatsManager) Leaderboard(mode string) []LeaderboardEntry {
	stats := m.load()
	return append([]LeaderboardEntry(nil), top5ForMode(stats, mode)...)
}

func (m StatsManager) DefaultPlayerName() string {
	currentUser := user.Current
	if m.CurrentUser != nil {
		currentUser = m.CurrentUser
	}
	hostname := os.Hostname
	if m.Hostname != nil {
		hostname = m.Hostname
	}

	username := "YOU"
	if u, err := currentUser(); err == nil && u != nil {
		if name := strings.TrimSpace(u.Username); name != "" {
			username = name
			if slash := strings.LastIndexAny(username, `\/`); slash >= 0 && slash < len(username)-1 {
				username = username[slash+1:]
			}
		}
	}
	if username = NormalizePlayerName(username); username == "" {
		username = "YOU"
	}

	host, err := hostname()
	if err != nil {
		return username
	}
	host = NormalizePlayerName(host)
	if host == "" {
		return username
	}
	return NormalizePlayerName(username + "@" + host)
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
	if err := json.Unmarshal(data, &stats); err != nil {
		return Stats{SchemaVersion: statsSchemaVersion}
	}
	return migrateStats(stats)
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

func migrateStats(stats Stats) Stats {
	switch stats.SchemaVersion {
	case statsSchemaVersion:
	case 0, 1:
		if stats.BestScore > 0 && len(stats.QuestTop5) == 0 {
			stats.QuestTop5 = []LeaderboardEntry{{Score: stats.BestScore, PlayerName: "YOU"}}
		}
	default:
		return Stats{SchemaVersion: statsSchemaVersion}
	}
	stats.SchemaVersion = statsSchemaVersion
	stats.ClassicTop5 = sanitizeLeaderboard(stats.ClassicTop5)
	stats.QuestTop5 = sanitizeLeaderboard(stats.QuestTop5)
	stats.Achievements = mergeAchievements(stats.Achievements)
	return stats
}

func sanitizeLeaderboard(entries []LeaderboardEntry) []LeaderboardEntry {
	result := make([]LeaderboardEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Score < 0 {
			continue
		}
		entry.PlayerName = NormalizePlayerName(entry.PlayerName)
		result = append(result, entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	if len(result) > leaderboardLimit {
		result = result[:leaderboardLimit]
	}
	return result
}

func top5ForMode(stats Stats, mode string) []LeaderboardEntry {
	if gameMode(mode) == GameModeQuest {
		return stats.QuestTop5
	}
	return stats.ClassicTop5
}

func setTop5ForMode(stats *Stats, mode string, entries []LeaderboardEntry) {
	if gameMode(mode) == GameModeQuest {
		stats.QuestTop5 = entries
		return
	}
	stats.ClassicTop5 = entries
}

func leaderboardRank(entries []LeaderboardEntry, score int) int {
	if score < 0 {
		return 0
	}
	if len(entries) < leaderboardLimit {
		return insertionIndex(entries, score) + 1
	}
	if score < entries[leaderboardLimit-1].Score {
		return 0
	}
	return insertionIndex(entries, score) + 1
}

func insertLeaderboardEntry(entries []LeaderboardEntry, entry LeaderboardEntry) ([]LeaderboardEntry, int) {
	entries = sanitizeLeaderboard(entries)
	rank := leaderboardRank(entries, entry.Score)
	if rank == 0 {
		return entries, 0
	}
	index := rank - 1
	entries = append(entries, LeaderboardEntry{})
	copy(entries[index+1:], entries[index:])
	entries[index] = entry
	if len(entries) > leaderboardLimit {
		entries = entries[:leaderboardLimit]
	}
	return entries, rank
}

func insertionIndex(entries []LeaderboardEntry, score int) int {
	for index, entry := range entries {
		if score > entry.Score {
			return index
		}
	}
	return len(entries)
}

func NormalizePlayerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "YOU"
	}
	cleaned := make([]rune, 0, len(name))
	width := 0
	for _, r := range name {
		if r == '\u001b' || unicode.IsControl(r) {
			continue
		}
		runeWidth := runeDisplayWidth(r)
		if width+runeWidth > maxPlayerNameColumns {
			break
		}
		cleaned = append(cleaned, r)
		width += runeWidth
	}
	result := strings.TrimSpace(string(cleaned))
	if result == "" {
		return "YOU"
	}
	return result
}

func runeDisplayWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if r >= 0x1100 {
		return 2
	}
	return 1
}
