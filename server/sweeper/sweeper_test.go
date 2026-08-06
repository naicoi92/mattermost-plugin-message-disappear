package sweeper

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// --- fakes ---

type fakeTTLSource struct {
	channels []ttl.ChannelTTL
	err      error
}

func (f fakeTTLSource) Channels(context.Context) ([]ttl.ChannelTTL, error) { return f.channels, f.err }

type fakeFinder struct {
	ids    map[string][]string // channelID -> post ids to return
	err    error
	called map[string]int64 // channelID -> last thresholdMs seen
}

func (f *fakeFinder) AgedThreads(_ context.Context, channelID string, thresholdMs int64, _ int) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.called == nil {
		f.called = map[string]int64{}
	}
	f.called[channelID] = thresholdMs
	return f.ids[channelID], nil
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

type captureLogger struct {
	infos  []string
	errors []string
}

func (l *captureLogger) LogInfo(msg string, _ ...any)  { l.infos = append(l.infos, msg) }
func (l *captureLogger) LogError(msg string, _ ...any) { l.errors = append(l.errors, msg) }

func newSweeper(t *testing.T, ttls TTLSource, finder *fakeFinder, purger *fakePurger) (*Sweeper, *captureLogger) {
	t.Helper()
	log := &captureLogger{}
	return New(ttls, finder, purger, log, 10), log
}

// --- tests ---

func TestRunPurgesAgedPerChannel(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{
		{ChannelID: "c1", TTL: 5 * time.Minute},
		{ChannelID: "c2", TTL: time.Hour},
	}}
	finder := &fakeFinder{ids: map[string][]string{"c1": {"p1", "p2"}, "c2": {"p3"}}}
	purger := &fakePurger{}
	sw, log := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.ElementsMatch(t, []string{"p1", "p2", "p3"}, purger.purged)
	require.Len(t, log.infos, 2, "one info log per purged channel")
	assert.Empty(t, log.errors)
}

func TestRunThresholdIsNowMinusTTL(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{{ChannelID: "c1", TTL: 5 * time.Minute}}}
	finder := &fakeFinder{ids: map[string][]string{"c1": {"p1"}}}
	purger := &fakePurger{}
	log := &captureLogger{}
	const now = int64(1_000_000)
	sw := &Sweeper{ttls: ttls, finder: finder, purger: purger, log: log, batchSize: 10, now: func() int64 { return now }}

	sw.Run()

	// threshold = now - ttl = 1_000_000 - 300_000.
	assert.Equal(t, now-5*time.Minute.Milliseconds(), finder.called["c1"])
}

func TestRunEmptyChannelSkips(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{{ChannelID: "c1", TTL: time.Minute}}}
	finder := &fakeFinder{ids: map[string][]string{"c1": nil}} // nothing aged
	purger := &fakePurger{}
	sw, log := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged)
	assert.Empty(t, log.infos, "no log for an empty channel")
}

func TestRunPurgeFailLogsAndContinues(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{
		{ChannelID: "c1", TTL: time.Minute},
		{ChannelID: "c2", TTL: time.Minute},
	}}
	finder := &fakeFinder{ids: map[string][]string{"c1": {"p1"}, "c2": {"p2"}}}
	purger := &fakePurger{err: errors.New("boom")}
	sw, log := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged, "purge failed, nothing recorded")
	require.Len(t, log.errors, 2, "purge failure logged per channel")
	assert.Empty(t, log.infos)
}

func TestRunFinderFailContinues(t *testing.T) {
	ttls := fakeTTLSource{channels: []ttl.ChannelTTL{
		{ChannelID: "c1", TTL: time.Minute},
		{ChannelID: "c2", TTL: time.Minute},
	}}
	finder := &fakeFinder{err: errors.New("query boom")}
	purger := &fakePurger{}
	sw, log := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged)
	require.Len(t, log.errors, 2, "finder failure logged per channel; sweep continues")
}

func TestRunTTLSourceFailReturns(t *testing.T) {
	ttls := fakeTTLSource{err: errors.New("list boom")}
	finder := &fakeFinder{}
	purger := &fakePurger{}
	sw, log := newSweeper(t, ttls, finder, purger)

	sw.Run()

	assert.Empty(t, purger.purged)
	require.Len(t, log.errors, 1)
}
