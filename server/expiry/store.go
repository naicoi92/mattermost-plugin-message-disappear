// Package expiry maintains the per-post expire index (mpmd_expire) and computes
// expire_at for posts in channels that have a TTL (D1/D5/D7).
package expiry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Entry is a row in the mpmd_expire index.
type Entry struct {
	PostID    string
	ChannelID string
	RootID    string // thread root id (the post's own id for a thread root)
	ExpireAt  int64  // unix milliseconds
	CreatedAt int64  // unix milliseconds
}

// ExpireIndexStore persists expire-index rows (persistence port; DIP for tests).
type ExpireIndexStore interface {
	// Migrate creates the table and indexes (idempotent).
	Migrate(ctx context.Context) error
	// Upsert inserts a row keyed by post_id, replacing channel/root/expire on conflict.
	Upsert(ctx context.Context, e Entry) error
	// UpdateExpireByRoot bumps expire_at for every row in a thread (root + replies).
	UpdateExpireByRoot(ctx context.Context, rootID string, expireAtMs int64) error
	// GetByPostID loads the row for a post, or nil when absent.
	GetByPostID(ctx context.Context, postID string) (*Entry, error)
}

// NewSQLStore wraps a Mattermost master DB handle as an ExpireIndexStore.
func NewSQLStore(db *sql.DB) ExpireIndexStore {
	return &sqlStore{db: db}
}

type sqlStore struct {
	db *sql.DB
}

// DDL is portable across postgres and sqlite (MM v10+ is postgres-focused).
// `id` is intentionally omitted — post_id is the primary key (one row per post).
const ddl = `
CREATE TABLE IF NOT EXISTS mpmd_expire (
    post_id    VARCHAR(26) NOT NULL PRIMARY KEY,
    channel_id VARCHAR(26) NOT NULL,
    root_id    VARCHAR(26) NOT NULL DEFAULT '',
    expire_at  BIGINT NOT NULL,
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mpmd_expire_expire_at ON mpmd_expire (expire_at);
CREATE INDEX IF NOT EXISTS idx_mpmd_expire_root ON mpmd_expire (root_id);
`

func (s *sqlStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, ddl)
	if err != nil {
		return fmt.Errorf("expire: migrate: %w", err)
	}
	return nil
}

const upsertSQL = `
INSERT INTO mpmd_expire (post_id, channel_id, root_id, expire_at, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(post_id) DO UPDATE SET
    channel_id = excluded.channel_id,
    root_id    = excluded.root_id,
    expire_at  = excluded.expire_at;
`

func (s *sqlStore) Upsert(ctx context.Context, e Entry) error {
	_, err := s.db.ExecContext(ctx, upsertSQL, e.PostID, e.ChannelID, e.RootID, e.ExpireAt, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("expire: upsert %q: %w", e.PostID, err)
	}
	return nil
}

const updateByRootSQL = `UPDATE mpmd_expire SET expire_at = ? WHERE root_id = ?;`

func (s *sqlStore) UpdateExpireByRoot(ctx context.Context, rootID string, expireAtMs int64) error {
	_, err := s.db.ExecContext(ctx, updateByRootSQL, expireAtMs, rootID)
	if err != nil {
		return fmt.Errorf("expire: bump thread %q: %w", rootID, err)
	}
	return nil
}

const getByPostSQL = `SELECT post_id, channel_id, root_id, expire_at, created_at FROM mpmd_expire WHERE post_id = ?;`

func (s *sqlStore) GetByPostID(ctx context.Context, postID string) (*Entry, error) {
	row := s.db.QueryRowContext(ctx, getByPostSQL, postID)
	var e Entry
	err := row.Scan(&e.PostID, &e.ChannelID, &e.RootID, &e.ExpireAt, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("expire: get %q: %w", postID, err)
	}
	return &e, nil
}
