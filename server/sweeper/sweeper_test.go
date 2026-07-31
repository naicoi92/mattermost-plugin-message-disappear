package sweeper

import (
	"context"
	"errors"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/expiry"
)

// --- fakes ---

type fakeStore struct {
	expired []expiry.Entry
	pruned  []string
	getErr  error
	delErr  error
}

func (f *fakeStore) GetExpired(_ context.Context, _ int64, _ int) ([]expiry.Entry, error) {
	out := f.expired
	f.expired = nil
	return out, f.getErr
}

func (f *fakeStore) DeleteByPostID(_ context.Context, postID string) error {
	f.pruned = append(f.pruned, postID)
	return f.delErr
}

type fakePosts struct {
	existing  map[string]bool            // postID -> exists
	deleted   []string                   // DeletePost call order
	deleteErr map[string]*model.AppError // postID -> error returned by DeletePost
}

func (f *fakePosts) GetPost(id string) (*model.Post, *model.AppError) {
	if f.existing[id] {
		return &model.Post{Id: id}, nil
	}
	return nil, &model.AppError{}
}

func (f *fakePosts) DeletePost(id string) *model.AppError {
	f.deleted = append(f.deleted, id)
	return f.deleteErr[id]
}

type captureLogger struct {
	errors []string
}

func (l *captureLogger) LogError(msg string, _ ...any) {
	l.errors = append(l.errors, msg)
}

func newSweeper(store *fakeStore, posts *fakePosts) (*Sweeper, *captureLogger) {
	log := &captureLogger{}
	return New(store, posts, log, 10), log
}

// --- tests ---

func TestRunDeletesExpiredAndPrunes(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}, {PostID: "p2"}}}
	posts := &fakePosts{existing: map[string]bool{"p1": true, "p2": true}, deleteErr: map[string]*model.AppError{}}
	sw, log := newSweeper(store, posts)

	sw.Run()

	assert.Equal(t, []string{"p1", "p2"}, posts.deleted, "both expired posts soft-deleted")
	assert.Equal(t, []string{"p1", "p2"}, store.pruned, "both rows pruned")
	assert.Empty(t, log.errors)
}

func TestRunStalePostPrunesWithoutError(t *testing.T) {
	// p3 already removed by the user -> DeletePost errors, GetPost not found.
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p3"}}}
	posts := &fakePosts{existing: map[string]bool{}, deleteErr: map[string]*model.AppError{"p3": {}}}
	sw, log := newSweeper(store, posts)

	sw.Run()

	assert.Contains(t, posts.deleted, "p3")
	assert.Equal(t, []string{"p3"}, store.pruned, "stale row pruned")
	assert.Empty(t, log.errors, "stale handling is not an error")
}

func TestRunTransientDeleteKeepsRowForRetry(t *testing.T) {
	// p4 still exists but DeletePost transiently fails -> row kept, logged.
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p4"}}}
	posts := &fakePosts{existing: map[string]bool{"p4": true}, deleteErr: map[string]*model.AppError{"p4": {}}}
	sw, log := newSweeper(store, posts)

	sw.Run()

	assert.Contains(t, posts.deleted, "p4")
	assert.Empty(t, store.pruned, "row kept so the post is retried next tick")
	require.Len(t, log.errors, 1)
}

func TestRunNoExpiredIsNoOp(t *testing.T) {
	store := &fakeStore{}
	posts := &fakePosts{existing: map[string]bool{}, deleteErr: map[string]*model.AppError{}}
	sw, log := newSweeper(store, posts)

	sw.Run()

	assert.Empty(t, posts.deleted)
	assert.Empty(t, store.pruned)
	assert.Empty(t, log.errors)
}

func TestRunQueryErrorLogsAndStops(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}}, getErr: errors.New("db down")}
	posts := &fakePosts{existing: map[string]bool{"p1": true}, deleteErr: map[string]*model.AppError{}}
	sw, log := newSweeper(store, posts)

	sw.Run()

	assert.Empty(t, posts.deleted, "no deletion when the query fails")
	require.Len(t, log.errors, 1)
}

func TestPruneErrorIsLogged(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}}, delErr: errors.New("delete row failed")}
	posts := &fakePosts{existing: map[string]bool{"p1": true}, deleteErr: map[string]*model.AppError{}}
	sw, log := newSweeper(store, posts)

	sw.Run()

	assert.Contains(t, posts.deleted, "p1", "post still deleted")
	require.Len(t, log.errors, 1, "prune failure is logged")
}

func TestNewDefaultsBatchSize(t *testing.T) {
	sw := New(&fakeStore{}, &fakePosts{existing: map[string]bool{}, deleteErr: map[string]*model.AppError{}}, &captureLogger{}, 0)
	assert.Equal(t, defaultBatchSize, sw.batchSize)
}
