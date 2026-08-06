// Package sweeper periodically purges posts whose age has passed the channel's
// TTL. The plugin drives it on a fixed ticker (see plugin.initSweeper).
//
// Posts are the single source of truth (no separate expire index): on each tick
// the sweeper asks the TTL store which channels have a TTL, then asks the
// retention finder for the threads in each channel whose newest message is older
// than the TTL, and purges them. Saved messages are protected by the finder.
package sweeper

import (
	"context"
	"time"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// TTLSource lists channels that have a TTL. ttl.TTLSettingStore satisfies it.
type TTLSource interface {
	Channels(ctx context.Context) ([]ttl.ChannelTTL, error)
}

// PostFinder returns the post ids to purge for a channel past an age threshold.
// retention.Finder satisfies it.
type PostFinder interface {
	AgedThreads(ctx context.Context, channelID string, thresholdMs int64, limit int) ([]string, error)
}

// Purger hard-deletes posts and their related data (idempotent; deleting
// already-removed rows is a no-op).
type Purger interface {
	Purge(ctx context.Context, postIDs []string) (int, error)
}

// Logger is the subset of the plugin API used for logging.
type Logger interface {
	LogInfo(msg string, keyvals ...any)
	LogError(msg string, keyvals ...any)
}

const defaultBatchSize = 500

// Sweeper purges aged posts per channel on each tick. Purge is idempotent, so
// nodes sweeping concurrently are safe (worst case: redundant delete attempts).
type Sweeper struct {
	ttls      TTLSource
	finder    PostFinder
	purger    Purger
	log       Logger
	batchSize int
	now       func() int64
}

// New creates a sweeper (batchSize defaults to 500 when <= 0).
func New(ttls TTLSource, finder PostFinder, purger Purger, log Logger, batchSize int) *Sweeper {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &Sweeper{
		ttls:      ttls,
		finder:    finder,
		purger:    purger,
		log:       log,
		batchSize: batchSize,
		now:       func() int64 { return time.Now().UnixMilli() },
	}
}

// Run executes one sweep tick: for every channel with a TTL, find threads whose
// newest message is older than the TTL and purge them (whole threads; saved
// messages are protected by the finder). A purge failure leaves the posts in
// place — the next tick finds and retries them.
func (s *Sweeper) Run() {
	ctx := context.Background()
	channels, err := s.ttls.Channels(ctx)
	if err != nil {
		s.log.LogError("disappear: sweeper list TTLs failed", "err", err)
		return
	}
	now := s.now()
	for _, c := range channels {
		threshold := now - c.TTL.Milliseconds()
		ids, err := s.finder.AgedThreads(ctx, c.ChannelID, threshold, s.batchSize)
		if err != nil {
			s.log.LogError("disappear: sweeper query failed", "channel_id", c.ChannelID, "err", err)
			continue
		}
		if len(ids) == 0 {
			continue
		}
		if _, err := s.purger.Purge(ctx, ids); err != nil {
			s.log.LogError("disappear: purge failed; will retry next tick", "channel_id", c.ChannelID, "n", len(ids), "err", err)
			continue
		}
		s.log.LogInfo("disappear: sweeper purged posts", "channel_id", c.ChannelID, "n", len(ids))
	}
}
