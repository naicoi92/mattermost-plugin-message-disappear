package expiry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeStore struct {
	rows    map[string]Entry
	upserts []Entry
	bumps   []threadBump
}

type threadBump struct {
	rootID string
	at     int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]Entry{}}
}

func (f *fakeStore) Migrate(context.Context) error { return nil }

func (f *fakeStore) Upsert(_ context.Context, e Entry) error {
	f.upserts = append(f.upserts, e)
	f.rows[e.PostID] = e
	return nil
}

func (f *fakeStore) UpdateExpireByRoot(_ context.Context, rootID string, at int64) error {
	f.bumps = append(f.bumps, threadBump{rootID: rootID, at: at})
	for k, v := range f.rows {
		if v.RootID == rootID {
			v.ExpireAt = at
			f.rows[k] = v
		}
	}
	return nil
}

func (f *fakeStore) GetByPostID(_ context.Context, postID string) (*Entry, error) {
	e, ok := f.rows[postID]
	if !ok {
		return nil, nil
	}
	return &e, nil
}

func (f *fakeStore) GetExpired(context.Context, int64, int) ([]Entry, error) { return nil, nil }
func (f *fakeStore) DeleteByPostID(context.Context, string) error            { return nil }
func (f *fakeStore) DeleteByPostIDs(context.Context, []string) error         { return nil }

type fakeTTL struct {
	d   time.Duration
	ok  bool
	err error
}

func (f fakeTTL) GetTTL(context.Context, string) (time.Duration, bool, error) {
	return f.d, f.ok, f.err
}

// --- service logic (D1/D5/D7) ---

func TestOnPostCreatedNoTTLIndexesNothing(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeTTL{ok: false})

	require.NoError(t, svc.OnPostCreated(context.Background(), &model.Post{Id: "p1", ChannelId: "c1", CreateAt: 1000}))
	assert.Empty(t, store.upserts, "channel without a TTL must not be indexed (D4)")
}

func TestOnPostCreatedStandaloneIndexesRow(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeTTL{d: time.Hour, ok: true})

	require.NoError(t, svc.OnPostCreated(context.Background(), &model.Post{Id: "p1", ChannelId: "c1", CreateAt: 1000}))

	require.Len(t, store.upserts, 1)
	assert.Empty(t, store.bumps, "a standalone post must not bump a thread")
	e := store.upserts[0]
	assert.Equal(t, "p1", e.PostID)
	assert.Equal(t, "p1", e.RootID, "root_id is the post's own id for a thread root")
	assert.Equal(t, 1000+time.Hour.Milliseconds(), e.ExpireAt)
}

func TestOnPostCreatedReplyBumpsThread(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeTTL{d: time.Hour, ok: true})

	require.NoError(t, svc.OnPostCreated(context.Background(), &model.Post{Id: "reply1", ChannelId: "c1", CreateAt: 5000, RootId: "root"}))

	require.Len(t, store.upserts, 1)
	assert.Equal(t, "root", store.upserts[0].RootID)
	require.Len(t, store.bumps, 1)
	assert.Equal(t, "root", store.bumps[0].rootID)
	assert.Equal(t, 5000+time.Hour.Milliseconds(), store.bumps[0].at, "thread bumped to reply send-time + TTL (D5)")
}

func TestOnPostCreatedPropagatesTTLReaderError(t *testing.T) {
	svc := NewService(newFakeStore(), fakeTTL{err: errors.New("boom")})
	err := svc.OnPostCreated(context.Background(), &model.Post{Id: "p1", ChannelId: "c1"})
	require.Error(t, err)
}

func TestOnPostUpdatedIsNoOp(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeTTL{ok: true})

	require.NoError(t, svc.OnPostUpdated(context.Background(), &model.Post{Id: "p1", ChannelId: "c1", UpdateAt: 9999}))
	assert.Empty(t, store.upserts, "editing must not change expiry (D7)")
	assert.Empty(t, store.bumps)
}
