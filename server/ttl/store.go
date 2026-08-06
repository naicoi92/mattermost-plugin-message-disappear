// Package ttl implements the per-channel TTL configuration domain: SQL-backed
// persistence (mpmd_ttl), the TTLService (permission-checked set/get/clear per
// D2/D4) and the allowed TTL presets and validation bounds.
package ttl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/sqlutil"
)

// TTLSetting is the per-channel TTL configuration persisted in the plugin's
// SQL table (mpmd_ttl): {duration_s, set_by, set_at}.
type TTLSetting struct {
	DurationSeconds int64
	SetBy           string
	SetAt           int64 // unix milliseconds
}

// TTLSettingStore persists per-channel TTL settings (persistence port; DIP for
// testability — the service depends on this interface, not the SQL store).
type TTLSettingStore interface {
	// Migrate creates the TTL table if absent. Idempotent.
	Migrate(ctx context.Context) error
	// Get returns the channel's TTL, or (nil, nil) when no TTL is set (default OFF, D4).
	Get(channelID string) (*TTLSetting, error)
	// Set upserts the channel's TTL (atomic; last committed write wins).
	Set(channelID string, setting TTLSetting) error
	// Clear removes the channel's TTL (default OFF).
	Clear(channelID string) error
}

// NewSQLStore wraps a Mattermost master DB handle as a TTLSettingStore. The TTL
// settings live in the same master DB as the expire index (mpmd_expire) and the
// purge targets, so there is a single persistence backend and no plugin-KV
// dependency (KV proved unreliable on Mattermost 10.x: "connection is shut
// down" during reload/deactivate).
func NewSQLStore(db *sql.DB, driver string) TTLSettingStore {
	return &sqlStore{db: db, driver: driver}
}

type sqlStore struct {
	db     *sql.DB
	driver string
}

// ttlDDL is portable across postgres and sqlite (MM v10+ is postgres-focused).
const ttlDDL = `
CREATE TABLE IF NOT EXISTS mpmd_ttl (
    channel_id VARCHAR(26) NOT NULL PRIMARY KEY,
    duration_s BIGINT NOT NULL,
    set_by     VARCHAR(26) NOT NULL,
    set_at     BIGINT NOT NULL
);
`

func (s *sqlStore) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, ttlDDL); err != nil {
		return fmt.Errorf("ttl: migrate: %w", err)
	}
	return nil
}

const getTTLSQL = `SELECT duration_s, set_by, set_at FROM mpmd_ttl WHERE channel_id = ?;`

// Get returns the channel's TTL, or (nil, nil) when unset (default OFF, D4).
func (s *sqlStore) Get(channelID string) (*TTLSetting, error) {
	row := s.db.QueryRow(sqlutil.Rebind(s.driver, getTTLSQL), channelID)
	var set TTLSetting
	err := row.Scan(&set.DurationSeconds, &set.SetBy, &set.SetAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ttl: get %q: %w", channelID, err)
	}
	return &set, nil
}

const upsertTTLSQL = `
INSERT INTO mpmd_ttl (channel_id, duration_s, set_by, set_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(channel_id) DO UPDATE SET
    duration_s = excluded.duration_s,
    set_by     = excluded.set_by,
    set_at     = excluded.set_at;
`

// Set upserts the TTL setting. UPSERT is atomic; concurrent sets serialise and
// the last committed value wins (no torn writes), replacing the former KV
// compare-and-set retry loop.
func (s *sqlStore) Set(channelID string, setting TTLSetting) error {
	if _, err := s.db.Exec(sqlutil.Rebind(s.driver, upsertTTLSQL), channelID, setting.DurationSeconds, setting.SetBy, setting.SetAt); err != nil {
		return fmt.Errorf("ttl: set %q: %w", channelID, err)
	}
	return nil
}

const clearTTLSQL = `DELETE FROM mpmd_ttl WHERE channel_id = ?;`

// Clear removes the TTL setting (default OFF).
func (s *sqlStore) Clear(channelID string) error {
	if _, err := s.db.Exec(sqlutil.Rebind(s.driver, clearTTLSQL), channelID); err != nil {
		return fmt.Errorf("ttl: clear %q: %w", channelID, err)
	}
	return nil
}
