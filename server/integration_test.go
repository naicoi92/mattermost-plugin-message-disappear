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

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/purge"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/retention"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/sweeper"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// discardLogger is a no-op sweeper.API (test seam).
type discardLogger struct{}

func (discardLogger) LogInfo(string, ...any)                                                  {}
func (discardLogger) LogError(string, ...any)                                                 {}
func (discardLogger) PublishWebSocketEvent(string, map[string]any, *model.WebsocketBroadcast) {}

// TestIntegrationSweepPurgeFromPosts drives the full sweeper lifecycle against
// an in-memory sqlite DB whose schema mimics Mattermost: posts older than the
// channel TTL are hard-purged with their full footprint, a not-yet-aged post
// survives, and a saved message is protected.
func TestIntegrationSweepPurgeFromPosts(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	ttlStore := ttl.NewSQLStore(db, "sqlite")
	require.NoError(t, ttlStore.Migrate(ctx))
	for _, ddl := range []string{
		`CREATE TABLE posts (id TEXT, createat INTEGER, channelid TEXT, rootid TEXT, deleteat INTEGER)`,
		`CREATE TABLE fileinfo (postid TEXT)`,
		`CREATE TABLE reactions (postid TEXT)`,
		`CREATE TABLE threads (postid TEXT)`,
		`CREATE TABLE preferences (userid TEXT, category TEXT, name TEXT, value TEXT)`,
	} {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}

	// Channel c1 has a 1h TTL.
	require.NoError(t, ttlStore.Set("c1", ttl.TTLSetting{DurationSeconds: 3600, SetBy: "u", SetAt: 1}))

	now := time.Now().UnixMilli()
	hour := time.Hour.Milliseconds()
	insertPost := func(id string, createAt int64) {
		_, e := db.Exec(`INSERT INTO posts (id, createat, channelid, rootid, deleteat) VALUES (?, ?, 'c1', '', 0)`, id, createAt)
		require.NoError(t, e)
		for _, tab := range []string{"fileinfo", "reactions", "threads"} {
			_, e := db.Exec(`INSERT INTO `+tab+` (postid) VALUES (?)`, id)
			require.NoError(t, e)
		}
	}
	insertPost("gone", now-2*hour)  // posted 2h ago -> aged past the 1h TTL
	insertPost("kept", now)         // posted now -> not yet aged
	insertPost("saved", now-2*hour) // aged, but a user saved it -> protected
	_, err = db.Exec(`INSERT INTO preferences (userid, category, name, value) VALUES ('u', 'flagged_post', 'saved', '')`)
	require.NoError(t, err)

	finder := retention.NewFinder(db, "sqlite")
	purger := purge.NewSQLPurger(db, "sqlite")
	sweeper.New(ttlStore, finder, purger, discardLogger{}, 10).Run()

	count := func(table, col, id string) int {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?`, id).Scan(&n))
		return n
	}
	for _, f := range []struct{ table, col string }{{"posts", "id"}, {"fileinfo", "postid"}, {"reactions", "postid"}, {"threads", "postid"}} {
		assert.Equalf(t, 0, count(f.table, f.col, "gone"), "aged post hard-deleted from %s", f.table)
	}
	assert.Equal(t, 1, count("posts", "id", "kept"), "not-yet-aged post survives")
	assert.Equal(t, 1, count("posts", "id", "saved"), "saved message is protected from purge")
}
