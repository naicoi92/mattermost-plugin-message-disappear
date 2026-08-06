package sweeper

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/retention"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// --- fakes ---

type fakeTTLSource struct {
	channels []ttl.ChannelTTL
	err      error
}

func (f fakeTTLSource) Channels(context.Context) ([]ttl.ChannelTTL, error) { return f.channels, f.err }

type fakeFinder struct {
	aged   map[string][]retention.AgedPost
	err    error
	called map[string]int64 // channelID -> last thresholdMs seen
}

func (f *fakeFinder) AgedThreads(_ context.Context, channelID string, thresholdMs int64, _ int) ([]retention.AgedPost, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.called == nil {
		f.called = map[string]int64{}
	}
	f.called[channelID] = thresholdMs
	return f.aged[channelID], nil
}

type fakePurger struct {
	purged []string
	err    error
}

func (f *fakePurger) Purge(_ context.Context, ids []string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.purged = append(f.purged, ids...)
	return len(ids), nil
}

// captureAPI satisfies sweeper.API: records log lines and WebSocket events.
type captureAPI struct {
	infos, errors []string
	events        []string // published event names
}

func (a *captureAPI) LogInfo(msg string, _ ...any)  { a.infos = append(a.infos, msg) }
func (a *captureAPI) LogError(msg string, _ ...any) { a.errors = append(a.errors, msg) }
func (a *captureAPI) PublishWebSocketEvent(event string, _ map[string]any, _ *model.WebsocketBroadcast) {
	a.events = append(a.events, event)
}

func newSweeper(t *testing.T, ttls TTLSource, finder *fakeFinder, purger *fakePurger) (*Sweeper, *captureAPI) {
	t.Helper()
	api := &captureAPI{}
	return New(ttls, finder, purger, api, 10), api
}

// --- tests ---

func TestRunPurgesAgedPerChannelAndNotifies(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{
		{ChannelID: "c1", TTL: 5 * time.Minute},
		{ChannelID: "c2", TTL: time.Hour},
	}}
	finder := &fakeFinder{aged: map[string][]retention.AgedPost{
		"c1": {{PostID: "p1"}, {PostID: "p2"}},
		"c2": {{PostID: "p3"}},
	}}
	purger := &fakePurger{}
	sw, api := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.ElementsMatch(t, []string{"p1", "p2", "p3"}, purger.purged)
	require.Len(t, api.infos, 2, "one info log per purged channel")
	// one post_deleted WebSocket event per purged post (so the webapp clears them).
	require.Len(t, api.events, 3, "a post_deleted event per purged post")
	for _, e := range api.events {
		assert.Equal(t, "post_deleted", e)
	}
	assert.Empty(t, api.errors)
}

func TestRunThresholdIsNowMinusTTL(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{{ChannelID: "c1", TTL: 5 * time.Minute}}}
	finder := &fakeFinder{aged: map[string][]retention.AgedPost{"c1": {{PostID: "p1"}}}}
	purger := &fakePurger{}
	const now = int64(1_000_000)
	sw := &Sweeper{ttls: ttls, finder: finder, purger: purger, api: &captureAPI{}, batchSize: 10, now: func() int64 { return now }}

	sw.Run()

	// threshold = now - ttl = 1_000_000 - 300_000.
	assert.Equal(t, now-5*time.Minute.Milliseconds(), finder.called["c1"])
}

func TestRunEmptyChannelSkips(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{{ChannelID: "c1", TTL: time.Minute}}}
	finder := &fakeFinder{aged: map[string][]retention.AgedPost{"c1": nil}} // nothing aged
	purger := &fakePurger{}
	sw, api := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged)
	assert.Empty(t, api.infos, "no log/event for an empty channel")
	assert.Empty(t, api.events)
}

func TestRunPurgeFailLogsAndContinues(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{
		{ChannelID: "c1", TTL: time.Minute},
		{ChannelID: "c2", TTL: time.Minute},
	}}
	finder := &fakeFinder{aged: map[string][]retention.AgedPost{"c1": {{PostID: "p1"}}, "c2": {{PostID: "p2"}}}}
	purger := &fakePurger{err: errors.New("boom")}
	sw, api := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged, "purge failed, nothing recorded")
	require.Len(t, api.errors, 2, "purge failure logged per channel")
	assert.Empty(t, api.events, "no post_deleted emitted when purge failed")
	assert.Empty(t, api.infos)
}

func TestRunFinderFailContinues(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{
		{ChannelID: "c1", TTL: time.Minute},
		{ChannelID: "c2", TTL: time.Minute},
	}}
	finder := &fakeFinder{err: errors.New("query boom")}
	purger := &fakePurger{}
	sw, api := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged)
	require.Len(t, api.errors, 2, "finder failure logged per channel; sweep continues")
}

func TestRunTTLSourceFailReturns(t *testing.T) {
	ttls := fakeTTLSource{err: errors.New("list boom")}
	finder := &fakeFinder{}
	purger := &fakePurger{}
	sw, api := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged)
	require.Len(t, api.errors, 1)
}
