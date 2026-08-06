package retention

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // pure-Go sqlite driver

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFinderDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	// Mimic the Mattermost schema (lowercase columns).
	_, err = db.Exec(`CREATE TABLE posts (id TEXT, createat INTEGER, channelid TEXT, rootid TEXT, deleteat INTEGER)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE preferences (userid TEXT, category TEXT, name TEXT, value TEXT)`)
	require.NoError(t, err)
	return db
}

func insertPost(t *testing.T, db *sql.DB, id, ch, root string, createAt int64, deleted bool) {
	t.Helper()
	d := 0
	if deleted {
		d = 1
	}
	_, err := db.Exec(`INSERT INTO posts (id, createat, channelid, rootid, deleteat) VALUES (?, ?, ?, ?, ?)`, id, createAt, ch, root, d)
	require.NoError(t, err)
}

func flag(t *testing.T, db *sql.DB, postID string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO preferences (userid, category, name, value) VALUES ('u', 'flagged_post', ?, '')`, postID)
	require.NoError(t, err)
}

// postIDs extracts the ids from aged posts for concise assertions.
func postIDs(ts []AgedPost) []string {
	out := make([]string, len(ts))
	for i, a := range ts {
		out[i] = a.PostID
	}
	return out
}

// A whole thread is returned once its newest message is older than the threshold;
// the root is not returned alone (thread-as-unit, no orphans).
func TestAgedThreadsWholeThreadUnit(t *testing.T) {
	db := newFinderDB(t)
	f := NewFinder(db, "sqlite")
	const now = int64(1_000_000_000)
	const ttl = int64(5 * 60 * 1000)

	// Thread: root 10m old, reply 1m old. Newest = reply (1m). Threshold = now-ttl = now-5m.
	// reply createat = now-1m >= threshold -> NOT aged -> whole thread kept.
	insertPost(t, db, "r1", "c1", "", now-10*60*1000, false)
	insertPost(t, db, "a1", "c1", "r1", now-60*1000, false)
	// Standalone 10m old -> aged.
	insertPost(t, db, "solo", "c1", "", now-10*60*1000, false)

	got, err := f.AgedThreads(context.Background(), "c1", now-ttl, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"solo"}, postIDs(got), "thread with a new reply is kept; only the aged standalone is returned")

	// Once the reply also ages past the threshold, the whole thread is returned together.
	got, err = f.AgedThreads(context.Background(), "c1", (now+5*60*1000)-ttl, 100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"r1", "a1", "solo"}, postIDs(got), "once the thread's newest ages out, root+reply are returned together")
}

func TestAgedThreadsChannelIsolationAndDeleted(t *testing.T) {
	db := newFinderDB(t)
	f := NewFinder(db, "sqlite")
	const now = int64(1_000_000_000)
	const ttl = int64(5 * 60 * 1000)

	insertPost(t, db, "c2post", "c2", "", now-10*60*1000, false) // other channel
	insertPost(t, db, "del", "c1", "", now-10*60*1000, true)     // soft-deleted
	insertPost(t, db, "live", "c1", "", now-10*60*1000, false)   // aged, live

	got, err := f.AgedThreads(context.Background(), "c1", now-ttl, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"live"}, postIDs(got), "other channel + soft-deleted are excluded")
}

// Boundary: a post exactly at the threshold age is NOT returned (strict <).
func TestAgedThreadsBoundaryKept(t *testing.T) {
	db := newFinderDB(t)
	f := NewFinder(db, "sqlite")
	const createAt = int64(1_000_000_000)
	insertPost(t, db, "p", "c1", "", createAt, false)

	// threshold == createAt -> createAt < threshold is false -> kept.
	got, err := f.AgedThreads(context.Background(), "c1", createAt, 100)
	require.NoError(t, err)
	assert.Empty(t, got)
	// threshold just past createAt -> returned.
	got, err = f.AgedThreads(context.Background(), "c1", createAt+1, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"p"}, postIDs(got))
}

// A saved message protects its whole thread: none of its posts are returned.
func TestAgedThreadsSavedProtectsThread(t *testing.T) {
	db := newFinderDB(t)
	f := NewFinder(db, "sqlite")
	const now = int64(1_000_000_000)
	const ttl = int64(5 * 60 * 1000)

	// Thread fully aged, but the root is saved by some user.
	insertPost(t, db, "r1", "c1", "", now-10*60*1000, false)
	insertPost(t, db, "a1", "c1", "r1", now-10*60*1000, false)
	flag(t, db, "r1")
	// Aged standalone, not saved.
	insertPost(t, db, "solo", "c1", "", now-10*60*1000, false)

	got, err := f.AgedThreads(context.Background(), "c1", now-ttl, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"solo"}, postIDs(got), "the saved thread is protected; only the unsaved standalone is returned")
}

// Saving a reply protects the whole thread (root + reply), since the thread is
// one purge unit.
func TestAgedThreadsSavedReplyProtectsRoot(t *testing.T) {
	db := newFinderDB(t)
	f := NewFinder(db, "sqlite")
	const now = int64(1_000_000_000)
	const ttl = int64(5 * 60 * 1000)

	insertPost(t, db, "r1", "c1", "", now-10*60*1000, false)
	insertPost(t, db, "a1", "c1", "r1", now-10*60*1000, false)
	flag(t, db, "a1") // the reply is saved

	got, err := f.AgedThreads(context.Background(), "c1", now-ttl, 100)
	require.NoError(t, err)
	assert.Empty(t, got, "saving a reply protects the whole thread, root included")
}

// The root id is returned alongside the post id so callers can emit a correct
// post_deleted event (root id is "" for a thread root, the root id for a reply).
func TestAgedThreadsReturnsRootID(t *testing.T) {
	db := newFinderDB(t)
	f := NewFinder(db, "sqlite")
	const now = int64(1_000_000_000)

	insertPost(t, db, "root", "c1", "", now-10*60*1000, false)
	insertPost(t, db, "reply", "c1", "root", now-10*60*1000, false)

	got, err := f.AgedThreads(context.Background(), "c1", now, 100)
	require.NoError(t, err)
	byID := map[string]string{}
	for _, a := range got {
		byID[a.PostID] = a.RootID
	}
	assert.Equal(t, "", byID["root"], "root post has empty root id")
	assert.Equal(t, "root", byID["reply"], "reply carries its root id")
}
