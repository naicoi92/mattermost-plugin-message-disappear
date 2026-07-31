// Package sweeper periodically purges posts whose expire_at has passed and prunes
// their index rows. HA is provided by cluster.Schedule (single-node).
package sweeper

import (
	"context"
	"time"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/expiry"
)

// ExpireStore is the subset of the expire index the sweeper consumes.
// expiry.ExpireIndexStore satisfies it.
type ExpireStore interface {
	GetExpired(ctx context.Context, nowMs int64, limit int) ([]expiry.Entry, error)
	DeleteByPostIDs(ctx context.Context, postIDs []string) error
}

// Purger hard-deletes posts and their related data in one transaction.
// purge.Purger satisfies it.
type Purger interface {
	Purge(ctx context.Context, postIDs []string) (int, error)
}

// Logger is the subset of the plugin API used for logging.
type Logger interface {
	LogError(msg string, keyvals ...any)
}

const defaultBatchSize = 500

// Sweeper purges expired posts (D10) and prunes their index rows on each tick.
// HA: cluster.Schedule runs Run on a single cluster node, so purges are not duplicated.
type Sweeper struct {
	store     ExpireStore
	purger    Purger
	log       Logger
	batchSize int
	now       func() int64
}

// New creates a sweeper with the given batch size (defaults to 500 when <= 0).
func New(store ExpireStore, purger Purger, log Logger, batchSize int) *Sweeper {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &Sweeper{
		store:     store,
		purger:    purger,
		log:       log,
		batchSize: batchSize,
		now:       func() int64 { return time.Now().UnixMilli() },
	}
}

// Run executes one sweep tick: purge expired posts and prune their index rows.
//   - purge succeeds -> prune the index rows (stale orphans are pruned too).
//   - purge fails -> keep all rows so the batch is retried next tick (no partial).
func (s *Sweeper) Run() {
	ctx := context.Background()
	entries, err := s.store.GetExpired(ctx, s.now(), s.batchSize)
	if err != nil {
		s.log.LogError("disappear: sweeper query failed", "err", err)
		return
	}
	if len(entries) == 0 {
		return
	}

	postIDs := make([]string, len(entries))
	for i, e := range entries {
		postIDs[i] = e.PostID
	}

	if _, err := s.purger.Purge(ctx, postIDs); err != nil {
		s.log.LogError("disappear: purge failed; will retry next tick", "n", len(postIDs), "err", err)
		return
	}
	if err := s.store.DeleteByPostIDs(ctx, postIDs); err != nil {
		s.log.LogError("disappear: prune expire rows failed", "err", err)
	}
}
