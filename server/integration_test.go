package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/expiry"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/purge"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/sweeper"
)

// fixedTTL is an expiry.TTLReader that always reports the same TTL (test seam).
type fixedTTL struct{ d time.Duration }

func (f fixedTTL) GetTTL(context.Context, string) (time.Duration, bool, error) {
	return f.d, true, nil
}

// discardLogger is a no-op sweeper.Logger (test seam).
type discardLogger struct{}

func (discardLogger) LogError(string, ...any) {}

// TestIntegrationExpireSweepPurge drives the full server lifecycle against an
// in-memory sqlite DB: a posted message is indexed, the sweeper hard-purges the
// expired one (full DB footprint), and the not-yet-expired one survives.
func TestIntegrationExpireSweepPurge(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	store := expiry.NewSQLStore(db, "sqlite")
	require.NoError(t, store.Migrate(ctx))
	for _, ddl := range []string{
		`CREATE TABLE posts (id TEXT PRIMARY KEY)`,
		`CREATE TABLE fileinfo (post_id TEXT)`,
		`CREATE TABLE reactions (post_id TEXT)`,
		`CREATE TABLE mentions (post_id TEXT)`,
	} {
		_, e := db.Exec(ddl)
		require.NoError(t, e)
	}

	seed := func(id string) {
		for _, q := range []string{
			`INSERT INTO posts (id) VALUES (?)`,
			`INSERT INTO fileinfo (post_id) VALUES (?)`,
			`INSERT INTO reactions (post_id) VALUES (?)`,
			`INSERT INTO mentions (post_id) VALUES (?)`,
		} {
			_, e := db.Exec(q, id)
			require.NoError(t, e)
		}
	}
	seed("gone") // expires -> purged
	seed("kept") // not expired -> survives

	expirySvc := expiry.NewService(store, fixedTTL{d: time.Hour})
	sw := sweeper.New(store, purge.NewSQLPurger(db), discardLogger{}, 10)

	now := time.Now().UnixMilli()
	hour := time.Hour.Milliseconds()
	// "gone": posted 2h ago + 1h TTL => expired 1h ago.
	require.NoError(t, expirySvc.OnPostCreated(ctx, &model.Post{Id: "gone", ChannelId: "c1", CreateAt: now - 2*hour}))
	// "kept": posted now + 1h TTL => expires in 1h.
	require.NoError(t, expirySvc.OnPostCreated(ctx, &model.Post{Id: "kept", ChannelId: "c1", CreateAt: now}))

	goneRow, err := store.GetByPostID(ctx, "gone")
	require.NoError(t, err)
	require.NotNil(t, goneRow, "expired post indexed")
	keptRow, err := store.GetByPostID(ctx, "kept")
	require.NoError(t, err)
	require.NotNil(t, keptRow, "future post indexed")

	sw.Run()

	count := func(table, col, id string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+col+" = ?", id).Scan(&n))
		return n
	}
	for _, f := range []struct{ table, col string }{{"posts", "id"}, {"fileinfo", "post_id"}, {"reactions", "post_id"}, {"mentions", "post_id"}} {
		assert.Equalf(t, 0, count(f.table, f.col, "gone"), "gone hard-deleted from %s", f.table)
	}
	goneAfter, err := store.GetByPostID(ctx, "gone")
	require.NoError(t, err)
	assert.Nil(t, goneAfter, "expire row pruned after purge")

	assert.Equal(t, 1, count("posts", "id", "kept"), "not-yet-expired post survives")
	keptAfter, err := store.GetByPostID(ctx, "kept")
	require.NoError(t, err)
	assert.NotNil(t, keptAfter, "not-yet-expired row preserved")
}
