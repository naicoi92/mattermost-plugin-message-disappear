package expiry

import (
	"context"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// TTLReader is the subset of the TTL service the ExpiryService needs (read a
// channel's TTL). *ttl.Service satisfies it.
type TTLReader interface {
	GetTTL(ctx context.Context, channelID string) (time.Duration, bool, error)
}

// Compile-time: the real ttl.Service satisfies TTLReader.
var _ TTLReader = (*ttl.Service)(nil)

// Service computes and maintains expire_at per post/thread.
//
//   - D1: expire_at = post send-time (CreateAt) + channel TTL.
//   - D5: a reply extends the whole thread (root + earlier replies) to the new expiry.
//   - D7: editing a post does NOT change its expiry (OnPostUpdated is a no-op).
type Service struct {
	store ExpireIndexStore
	ttl   TTLReader
	now   func() int64 // injectable clock (unix ms); defaults to time.Now.
}

// NewService wires the ExpiryService with its persistence and TTL ports.
func NewService(store ExpireIndexStore, ttlReader TTLReader) *Service {
	return &Service{store: store, ttl: ttlReader, now: func() int64 { return time.Now().UnixMilli() }}
}

// Migrate creates the expire index table + indexes.
func (s *Service) Migrate(ctx context.Context) error {
	return s.store.Migrate(ctx)
}

// threadRoot returns the thread root id for a post: its RootId, or its own Id
// when the post is itself a thread root (RootId == ""). Storing root_id this way
// lets a single `WHERE root_id = ?` bump an entire thread.
func threadRoot(post *model.Post) string {
	if post.RootId != "" {
		return post.RootId
	}
	return post.Id
}

// OnPostCreated indexes a newly posted message.
func (s *Service) OnPostCreated(ctx context.Context, post *model.Post) error {
	d, hasTTL, err := s.ttl.GetTTL(ctx, post.ChannelId)
	if err != nil {
		return err
	}
	if !hasTTL {
		return nil // channel has no TTL -> nothing to expire (D4 default OFF)
	}

	expireAt := post.CreateAt + d.Milliseconds()
	root := threadRoot(post)

	if err := s.store.Upsert(ctx, Entry{
		PostID:    post.Id,
		ChannelID: post.ChannelId,
		RootID:    root,
		ExpireAt:  expireAt,
		CreatedAt: s.now(),
	}); err != nil {
		return err
	}

	// A reply extends the whole thread (D5): bump root + earlier replies to the new
	// expiry. A standalone post (RootId == "") needs no bump. Ordering is safe: a
	// reply's RootId references an already-committed root, so the root is always
	// indexed before its replies — a forward bump can never regress an earlier reply.
	// (If MM ever delivered hooks out of order, a forward-only GREATEST(expire_at, ?)
	// bump would be needed; not required today.)
	if post.RootId != "" {
		if err := s.store.UpdateExpireByRoot(ctx, root, expireAt); err != nil {
			return err
		}
	}
	return nil
}

// OnPostUpdated is a no-op: editing does NOT extend the TTL (D7).
func (s *Service) OnPostUpdated(_ context.Context, _ *model.Post) error {
	return nil
}
