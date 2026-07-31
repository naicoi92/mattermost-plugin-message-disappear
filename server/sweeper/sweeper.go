// Package sweeper periodically soft-deletes posts whose expire_at has passed and
// prunes their index rows. HA is provided by cluster.Schedule (single-node).
package sweeper

import (
	"context"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/expiry"
)

// ExpireStore is the subset of the expire index the sweeper consumes.
// expiry.ExpireIndexStore satisfies it.
type ExpireStore interface {
	GetExpired(ctx context.Context, nowMs int64, limit int) ([]expiry.Entry, error)
	DeleteByPostID(ctx context.Context, postID string) error
}

// PostDeleter soft-deletes posts (MM DeletePost) and looks them up to detect
// stale rows (post already removed by user/native retention).
type PostDeleter interface {
	GetPost(postID string) (*model.Post, *model.AppError)
	DeletePost(postID string) *model.AppError
}

// Logger is the subset of the plugin API used for logging.
type Logger interface {
	LogError(msg string, keyvals ...any)
}

const defaultBatchSize = 500

// Sweeper deletes expired posts and prunes their index rows on each tick.
type Sweeper struct {
	store     ExpireStore
	posts     PostDeleter
	log       Logger
	batchSize int
	now       func() int64
}

// New creates a sweeper with the given batch size (defaults to 500 when <= 0).
func New(store ExpireStore, posts PostDeleter, log Logger, batchSize int) *Sweeper {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &Sweeper{
		store:     store,
		posts:     posts,
		log:       log,
		batchSize: batchSize,
		now:       func() int64 { return time.Now().UnixMilli() },
	}
}

// Run executes one sweep tick: delete expired posts and prune their index rows.
func (s *Sweeper) Run() {
	ctx := context.Background()
	entries, err := s.store.GetExpired(ctx, s.now(), s.batchSize)
	if err != nil {
		s.log.LogError("disappear: sweeper query failed", "err", err)
		return
	}
	for _, e := range entries {
		s.sweepOne(ctx, e.PostID)
	}
}

// sweepOne deletes one post (soft) and prunes its index row.
//   - post already gone (stale, 404): prune the row, no error.
//   - delete transiently fails, or the lookup itself fails: keep the row to retry.
func (s *Sweeper) sweepOne(ctx context.Context, postID string) {
	if appErr := s.posts.DeletePost(postID); appErr != nil {
		// A confirmed 404 means the post was removed by the user/native retention
		// -> prune the stale row. Any other GetPost outcome (post still present, or
		// the lookup itself transiently failed) keeps the row so the post is retried.
		if _, getErr := s.posts.GetPost(postID); getErr != nil && getErr.StatusCode == http.StatusNotFound {
			s.prune(ctx, postID)
			return
		}
		s.log.LogError("disappear: delete failed; will retry next tick", "post_id", postID, "err", appErr)
		return
	}
	s.prune(ctx, postID)
}

func (s *Sweeper) prune(ctx context.Context, postID string) {
	if err := s.store.DeleteByPostID(ctx, postID); err != nil {
		s.log.LogError("disappear: prune expire row failed", "post_id", postID, "err", err)
	}
}
