package sweeper

import (
	"context"
	"errors"
	"testing"

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

func (f *fakeStore) DeleteByPostIDs(_ context.Context, postIDs []string) error {
	f.pruned = append(f.pruned, postIDs...)
	return f.delErr
}

type fakePurger struct {
	calls [][]string
	err   error
}

func (p *fakePurger) Purge(_ context.Context, postIDs []string) (int, error) {
	p.calls = append(p.calls, append([]string(nil), postIDs...))
	return len(postIDs), p.err
}

type captureLogger struct {
	errors []string
}

func (l *captureLogger) LogError(msg string, _ ...any) {
	l.errors = append(l.errors, msg)
}

func newSweeper(store *fakeStore, purger *fakePurger) (*Sweeper, *captureLogger) {
	log := &captureLogger{}
	return New(store, purger, log, 10), log
}

// --- tests ---

func TestRunPurgesBatchAndPrunes(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}, {PostID: "p2"}}}
	purger := &fakePurger{}
	sw, log := newSweeper(store, purger)

	sw.Run()

	require.Len(t, purger.calls, 1)
	assert.Equal(t, []string{"p1", "p2"}, purger.calls[0], "whole batch purged in one call")
	assert.Equal(t, []string{"p1", "p2"}, store.pruned, "both rows pruned")
	assert.Empty(t, log.errors)
}

func TestRunNoExpiredIsNoOp(t *testing.T) {
	store := &fakeStore{}
	purger := &fakePurger{}
	sw, log := newSweeper(store, purger)

	sw.Run()

	assert.Empty(t, purger.calls)
	assert.Empty(t, store.pruned)
	assert.Empty(t, log.errors)
}

func TestRunPurgeErrorKeepsRowsForRetry(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}}}
	purger := &fakePurger{err: errors.New("tx failed")}
	sw, log := newSweeper(store, purger)

	sw.Run()

	require.Len(t, purger.calls, 1)
	assert.Empty(t, store.pruned, "rows kept so the batch is retried next tick (no partial purge)")
	require.Len(t, log.errors, 1)
}

func TestRunQueryErrorLogsAndStops(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}}, getErr: errors.New("db down")}
	purger := &fakePurger{}
	sw, log := newSweeper(store, purger)

	sw.Run()

	assert.Empty(t, purger.calls, "no purge when the query fails")
	require.Len(t, log.errors, 1)
}

func TestRunPruneErrorIsLogged(t *testing.T) {
	store := &fakeStore{expired: []expiry.Entry{{PostID: "p1"}}, delErr: errors.New("prune failed")}
	purger := &fakePurger{}
	sw, log := newSweeper(store, purger)

	sw.Run()

	require.Len(t, purger.calls, 1, "posts still purged")
	require.Len(t, log.errors, 1, "prune failure is logged")
}

func TestNewDefaultsBatchSize(t *testing.T) {
	sw := New(&fakeStore{}, &fakePurger{}, &captureLogger{}, 0)
	assert.Equal(t, defaultBatchSize, sw.batchSize)
}
