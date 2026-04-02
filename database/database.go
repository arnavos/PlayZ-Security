package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "modernc.org/sqlite"
)

type GuildData map[string]interface{}

type SQLiteDB struct {
	Conn     *sql.DB
	Mu       *sync.RWMutex
	Cache    map[string]cacheEntry
	CacheTTL time.Duration
}

type cacheEntry struct {
	Data      GuildData
	ExpiresAt time.Time
}

func cloneGuildData(in GuildData) GuildData {
	out := make(GuildData, len(in))
	for k, v := range in {
		switch typed := v.(type) {
		case []string:
			copied := make([]string, len(typed))
			copy(copied, typed)
			out[k] = copied
		default:
			out[k] = v
		}
	}
	return out
}

func (db *SQLiteDB) setCache(guildID string, data GuildData) {
	db.Mu.Lock()
	defer db.Mu.Unlock()
	if db.Cache == nil {
		db.Cache = make(map[string]cacheEntry)
	}
	db.Cache[guildID] = cacheEntry{
		Data:      cloneGuildData(data),
		ExpiresAt: time.Now().Add(db.CacheTTL),
	}
}

func (db *SQLiteDB) getCache(guildID string) (GuildData, bool) {
	db.Mu.RLock()
	defer db.Mu.RUnlock()
	entry, ok := db.Cache[guildID]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return cloneGuildData(entry.Data), true
}

func (db *SQLiteDB) invalidateCache(guildID string) {
	db.Mu.Lock()
	defer db.Mu.Unlock()
	delete(db.Cache, guildID)
}

func (db *SQLiteDB) ensureGuildDefaults(guildID string, defaults map[string]interface{}) error {
	for key, value := range defaults {
		if _, err := db.upsertSetting(guildID, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (db *SQLiteDB) upsertSetting(guildID, key string, value interface{}) (bool, error) {
	valType := "string"
	val := ""

	switch typed := value.(type) {
	case bool:
		valType = "bool"
		if typed {
			val = "1"
		} else {
			val = "0"
		}
	case int:
		valType = "int"
		val = fmt.Sprint(typed)
	case string:
		val = typed
	default:
		blob, err := json.Marshal(typed)
		if err != nil {
			return false, err
		}
		valType = "json"
		val = string(blob)
	}

	_, err := db.Conn.Exec(`
		INSERT INTO guild_settings(guild_id, key, value, value_type)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(guild_id, key) DO UPDATE SET
			value=excluded.value,
			value_type=excluded.value_type
	`, guildID, key, val, valType)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *SQLiteDB) CreateGuild(_ *discordgo.User, guild *discordgo.Guild) {
	if guild == nil {
		return
	}

	if _, err := db.FindData(guild.ID); err == nil {
		return
	}

	defaults := map[string]interface{}{
		"antinuke-enabled":    false,
		"anti-invite":         "off",
		"anti-everyone-mention": true,
		"anti-here-mention":     true,
		"anti-ban":            true,
		"anti-bot":            true,
		"anti-kick":           true,
		"anti-prune":          true,
		"anti-guild-update":   true,
		"anti-name-change":    true,
		"anti-widget-spam":    true,
		"anti-member-role":    true,
		"anti-role-create":    true,
		"anti-role-delete":    true,
		"anti-role-update":    true,
		"anti-vanity-steal":   true,
		"anti-channel-create": true,
		"anti-channel-delete": true,
		"anti-channel-update": true,
		"anti-webhook-create": true,
		"guild-id":            guild.ID,
		"guild-name":          guild.Name,
		"log-channel":         "nil",
		"moderation-type":     "ban",
		"prefix":              ">",
		"offense-threshold":   "1",
		"vanity-url":          guild.VanityURLCode,
	}

	if err := db.ensureGuildDefaults(guild.ID, defaults); err != nil {
		return
	}

	db.invalidateCache(guild.ID)

}

func (db *SQLiteDB) DeleteGuild(guildID string) bool {
	if guildID == "" {
		return false
	}

	if _, err := db.Conn.Exec(`DELETE FROM guild_settings WHERE guild_id = ?`, guildID); err != nil {
		return false
	}
	if _, err := db.Conn.Exec(`DELETE FROM whitelists WHERE guild_id = ?`, guildID); err != nil {
		return false
	}
	db.invalidateCache(guildID)
	return true
}

func (db *SQLiteDB) FindData(guildID string) (GuildData, error) {
	if cached, ok := db.getCache(guildID); ok {
		return cached, nil
	}

	rows, err := db.Conn.Query(`SELECT key, value, value_type FROM guild_settings WHERE guild_id = ?`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := GuildData{}
	count := 0
	for rows.Next() {
		count++
		var key, value, valueType string
		if err := rows.Scan(&key, &value, &valueType); err != nil {
			return nil, err
		}

		switch valueType {
		case "bool":
			data[key] = value == "1"
		case "int":
			data[key] = value
		case "json":
			var out interface{}
			if err := json.Unmarshal([]byte(value), &out); err != nil {
				data[key] = value
			} else {
				data[key] = out
			}
		default:
			data[key] = value
		}
	}

	if count == 0 {
		return nil, errors.New("guild data not found")
	}

	data["users"] = db.getWhitelist(guildID, "users")
	data["whitelisted-roles"] = db.getWhitelist(guildID, "whitelisted-roles")
	data["whitelisted-invite-channels"] = db.getWhitelist(guildID, "whitelisted-invite-channels")
	data["whitelisted-webhook-channels"] = db.getWhitelist(guildID, "whitelisted-webhook-channels")

	db.setCache(guildID, data)
	return data, nil
}

func (db *SQLiteDB) IsWhitelisted(guildID, typeKey, id string, optionalMember *discordgo.Member) bool {
	ids := db.getWhitelist(guildID, typeKey)
	for _, whitelistedID := range ids {
		if whitelistedID == id {
			return true
		}
	}

	if optionalMember == nil {
		return false
	}

	roles := db.getWhitelist(guildID, "whitelisted-roles")
	for _, roleID := range roles {
		for _, memberRoleID := range optionalMember.Roles {
			if roleID == memberRoleID {
				return true
			}
		}
	}

	return false
}

func (db *SQLiteDB) SetData(_ string, guildID, index, value string) (bool, error) {
	ok, err := db.upsertSetting(guildID, index, value)
	if err == nil {
		db.invalidateCache(guildID)
	}
	return ok, err
}

func (db *SQLiteDB) SetToggle(_ string, guildID, index string, value bool) (bool, error) {
	ok, err := db.upsertSetting(guildID, index, value)
	if err == nil {
		db.invalidateCache(guildID)
	}
	return ok, err
}

func (db *SQLiteDB) addWhitelist(guildID, valueKey, id string) error {
	_, err := db.Conn.Exec(`
		INSERT INTO whitelists(guild_id, list_key, entry_id)
		VALUES(?, ?, ?)
		ON CONFLICT(guild_id, list_key, entry_id) DO NOTHING
	`, guildID, valueKey, id)
	return err
}

func (db *SQLiteDB) removeWhitelist(guildID, valueKey, id string) error {
	_, err := db.Conn.Exec(`DELETE FROM whitelists WHERE guild_id = ? AND list_key = ? AND entry_id = ?`, guildID, valueKey, id)
	return err
}

func (db *SQLiteDB) getWhitelist(guildID, valueKey string) []string {
	rows, err := db.Conn.Query(`SELECT entry_id FROM whitelists WHERE guild_id = ? AND list_key = ?`, guildID, valueKey)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (db *SQLiteDB) SetWhitelistData(guildID, id, index, valueKey string) (bool, error) {
	if strings.TrimSpace(guildID) == "" || strings.TrimSpace(id) == "" || strings.TrimSpace(valueKey) == "" {
		return false, fmt.Errorf("invalid whitelist data")
	}

	switch index {
	case "$push":
		if err := db.addWhitelist(guildID, valueKey, id); err != nil {
			return false, err
		}
	case "$pull":
		if err := db.removeWhitelist(guildID, valueKey, id); err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("unsupported operation: %s", index)
	}

	db.invalidateCache(guildID)
	return true, nil
}

func (db *SQLiteDB) ClearWhitelist(guildID string) (int64, error) {
	result, err := db.Conn.Exec(`DELETE FROM whitelists WHERE guild_id = ?`, guildID)
	if err != nil {
		return 0, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	db.invalidateCache(guildID)
	return affected, nil
}

func SetupDB() SQLiteDB {
	if err := os.MkdirAll("data", 0o755); err != nil {
		panic(err)
	}

	dbPath := filepath.Join("data", "playz-security.db")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	// SQLite + concurrent goroutines: keep a single shared connection to avoid lock churn.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	if _, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS guild_settings (
			guild_id TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			value_type TEXT NOT NULL,
			PRIMARY KEY (guild_id, key)
		);
		CREATE TABLE IF NOT EXISTS whitelists (
			guild_id TEXT NOT NULL,
			list_key TEXT NOT NULL,
			entry_id TEXT NOT NULL,
			PRIMARY KEY (guild_id, list_key, entry_id)
		);
	`); err != nil {
		panic(err)
	}

	// Pragmas tuned for bot workloads (frequent reads + small writes).
	if _, err = conn.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		panic(err)
	}

	return SQLiteDB{
		Conn:     conn,
		Mu:       &sync.RWMutex{},
		Cache:    make(map[string]cacheEntry),
		CacheTTL: 3 * time.Second,
	}
}

var Database = SetupDB()
