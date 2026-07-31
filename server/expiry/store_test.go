package expiry

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // pure-Go sqlite driver for the integration test

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore returns an in-memory sqlite-backed store with the schema migrated.
func newTestStore(t *testing.T) ExpireIndexStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	store := NewSQLStore(db)
	require.NoError(t, store.Migrate(context.Background()))
	return store
}

func TestStoreMigrateIsIdempotent(t *testing.T) {
	// newTestStore already migrates; running again must not error.
	store := newTestStore(t)
	require.NoError(t, store.Migrate(context.Background()))
}

func TestStoreUpsertAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p1", ChannelID: "c1", RootID: "p1", ExpireAt: 1000, CreatedAt: 1}))

	got, err := store.GetByPostID(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(1000), got.ExpireAt)

	absent, err := store.GetByPostID(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, absent)
}

func TestStoreUpsertReplacesOnConflict(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p1", ChannelID: "c1", RootID: "p1", ExpireAt: 1000, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p1", ChannelID: "c1", RootID: "p1", ExpireAt: 2000, CreatedAt: 1}))

	got, err := store.GetByPostID(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(2000), got.ExpireAt, "second upsert wins on post_id conflict")
}

func TestStoreUpdateByRootBumpsWholeThread(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// A thread: root "root" (root_id = its own id) + reply "r1".
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "root", ChannelID: "c1", RootID: "root", ExpireAt: 1000, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "r1", ChannelID: "c1", RootID: "root", ExpireAt: 1000, CreatedAt: 1}))
	// An unrelated post that must NOT be bumped.
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "other", ChannelID: "c1", RootID: "other", ExpireAt: 1000, CreatedAt: 1}))

	require.NoError(t, store.UpdateExpireByRoot(ctx, "root", 9999))

	for _, pid := range []string{"root", "r1"} {
		got, err := store.GetByPostID(ctx, pid)
		require.NoError(t, err)
		assert.Equalf(t, int64(9999), got.ExpireAt, "thread member %s should be bumped", pid)
	}
	other, err := store.GetByPostID(ctx, "other")
	require.NoError(t, err)
	assert.Equal(t, int64(1000), other.ExpireAt, "unrelated post must not be bumped")
}

func TestStoreGetExpiredRespectsTimeAndLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "old", ChannelID: "c", RootID: "old", ExpireAt: 100, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "now", ChannelID: "c", RootID: "now", ExpireAt: 200, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "future", ChannelID: "c", RootID: "future", ExpireAt: 300, CreatedAt: 1}))

	// now=200 -> old + now expired, future not; oldest first.
	got, err := store.GetExpired(ctx, 200, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []string{"old", "now"}, []string{got[0].PostID, got[1].PostID})

	// limit respected.
	limited, err := store.GetExpired(ctx, 300, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, "old", limited[0].PostID)
}

func TestStoreDeleteByPostID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p1", ChannelID: "c", RootID: "p1", ExpireAt: 1, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p2", ChannelID: "c", RootID: "p2", ExpireAt: 1, CreatedAt: 1}))

	require.NoError(t, store.DeleteByPostID(ctx, "p1"))
	gone, err := store.GetByPostID(ctx, "p1")
	require.NoError(t, err)
	assert.Nil(t, gone)
	kept, err := store.GetByPostID(ctx, "p2")
	require.NoError(t, err)
	assert.NotNil(t, kept, "unrelated row preserved")
}

func TestStoreMigrateFailsOnClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close()) // closed handle -> migrate errors

	err = NewSQLStore(db).Migrate(context.Background())
	require.Error(t, err)
}

func TestStoreDeleteByPostIDsBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p1", ChannelID: "c", RootID: "p1", ExpireAt: 1, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p2", ChannelID: "c", RootID: "p2", ExpireAt: 1, CreatedAt: 1}))
	require.NoError(t, store.Upsert(ctx, Entry{PostID: "p3", ChannelID: "c", RootID: "p3", ExpireAt: 1, CreatedAt: 1}))

	require.NoError(t, store.DeleteByPostIDs(ctx, []string{"p1", "p2"}))
	gone1, _ := store.GetByPostID(ctx, "p1")
	gone2, _ := store.GetByPostID(ctx, "p2")
	kept, _ := store.GetByPostID(ctx, "p3")
	assert.Nil(t, gone1)
	assert.Nil(t, gone2)
	assert.NotNil(t, kept, "unrelated row preserved")

	// Empty batch is a no-op (no error).
	require.NoError(t, store.DeleteByPostIDs(ctx, nil))
}
