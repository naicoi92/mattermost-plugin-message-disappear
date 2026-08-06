// Package sweeper periodically purges posts whose age has passed the channel's
// TTL. The plugin drives it on a fixed ticker (see plugin.initSweeper).
//
// Posts are the single source of truth (no separate expire index): on each tick
// the sweeper asks the TTL store which channels have a TTL, then asks the
// retention finder for the threads in each channel whose newest message is older
// than the TTL, purges them, and publishes a post_deleted WebSocket event per
// purged post so the webapp drops them from the live view (a direct DB delete
// does not emit MM's own event). Saved messages are protected by the finder.
package sweeper

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/retention"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// TTLSource lists channels that have a TTL. ttl.TTLSettingStore satisfies it.
type TTLSource interface {
	Channels(ctx context.Context) ([]ttl.ChannelTTL, error)
}

// PostFinder returns the posts to purge for a channel past an age threshold.
// retention.Finder satisfies it.
type PostFinder interface {
	AgedThreads(ctx context.Context, channelID string, thresholdMs int64, limit int) ([]retention.AgedPost, error)
}

// Purger hard-deletes posts and their related data (idempotent; deleting
// already-removed rows is a no-op).
type Purger interface {
	Purge(ctx context.Context, postIDs []string) (int, error)
}

// Broadcaster publishes WebSocket events (so the webapp clears purged posts live).
type Broadcaster interface {
	PublishWebSocketEvent(event string, payload map[string]any, broadcast *model.WebsocketBroadcast)
}

// Logger is the subset of the plugin API used for logging.
type Logger interface {
	LogInfo(msg string, keyvals ...any)
	LogError(msg string, keyvals ...any)
}

// API is the plugin-API subset the sweeper needs: logging + WebSocket broadcast.
type API interface {
	Logger
	Broadcaster
}

const defaultBatchSize = 500

// Sweeper purges aged posts per channel on each tick. Purge is idempotent, so
// nodes sweeping concurrently are safe (worst case: redundant delete attempts).
type Sweeper struct {
	ttls      TTLSource
	finder    PostFinder
	purger    Purger
	api       API
	batchSize int
	now       func() int64
}

// New creates a sweeper (batchSize defaults to 500 when <= 0).
func New(ttls TTLSource, finder PostFinder, purger Purger, api API, batchSize int) *Sweeper {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &Sweeper{
		ttls:      ttls,
		finder:    finder,
		purger:    purger,
		api:       api,
		batchSize: batchSize,
		now:       func() int64 { return time.Now().UnixMilli() },
	}
}

// Run executes one sweep tick: for every channel with a TTL, find threads whose
// newest message is older than the TTL, purge them (whole threads; saved
// messages are protected by the finder), and notify the webapp. A purge failure
// leaves the posts in place — the next tick finds and retries them.
func (s *Sweeper) Run() {
	ctx := context.Background()
	channels, err := s.ttls.Channels(ctx)
	if err != nil {
		s.api.LogError("disappear: sweeper list TTLs failed", "err", err)
		return
	}
	now := s.now()
	for _, c := range channels {
		threshold := now - c.TTL.Milliseconds()
		aged, err := s.finder.AgedThreads(ctx, c.ChannelID, threshold, s.batchSize)
		if err != nil {
			s.api.LogError("disappear: sweeper query failed", "channel_id", c.ChannelID, "err", err)
			continue
		}
		if len(aged) == 0 {
			continue
		}
		ids := make([]string, len(aged))
		for i, a := range aged {
			ids[i] = a.PostID
		}
		if _, err := s.purger.Purge(ctx, ids); err != nil {
			s.api.LogError("disappear: purge failed; will retry next tick", "channel_id", c.ChannelID, "n", len(ids), "err", err)
			continue
		}
		// A direct DB delete does not emit MM's post_deleted event; publish one per
		// post so the channel/thread views drop them without a client reload.
		s.notifyDeleted(c.ChannelID, aged, now)
		s.api.LogInfo("disappear: sweeper purged posts", "channel_id", c.ChannelID, "n", len(ids))
	}
}

// notifyDeleted publishes a post_deleted WebSocket event per purged post. The
// payload mirrors MM's own DeletePost event (data.post = post JSON) so the webapp
// removes the post from the channel and thread views.
func (s *Sweeper) notifyDeleted(channelID string, aged []retention.AgedPost, nowMs int64) {
	for _, a := range aged {
		postJSON, err := json.Marshal(model.Post{Id: a.PostID, ChannelId: channelID, RootId: a.RootID, DeleteAt: nowMs})
		if err != nil {
			continue
		}
		s.api.PublishWebSocketEvent(string(model.WebsocketEventPostDeleted),
			map[string]any{"post": string(postJSON)},
			&model.WebsocketBroadcast{ChannelId: channelID})
	}
}
