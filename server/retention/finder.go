// Package retention finds messages that should be purged by a disappearing-TTL
// policy. It is the sweeper's source of post ids: given a channel and an age
// threshold (now - ttl), it returns the posts of every thread whose newest
// message is older than the threshold — the whole thread as a unit, so a root
// is never hard-deleted before its replies (no dangling rootid).
//
// Saved messages are protected: a thread containing any post a user has saved
// (Mattermost stores saved posts in the `preferences` table, category
// "flagged_post", name = post id) is never returned, so the whole thread stays.
package retention

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/sqlutil"
)

// Finder queries the Mattermost master DB for threads that have aged past a TTL.
type Finder struct {
	db     *sql.DB
	driver string
}

// NewFinder wraps a Mattermost master DB handle. driver rebinds "?" to the DB's
// native placeholder form (postgres needs $N).
func NewFinder(db *sql.DB, driver string) *Finder {
	return &Finder{db: db, driver: driver}
}

// agedThreadsSQL returns the post ids of threads in a channel whose newest
// message is older than the age threshold, EXCLUDING threads that contain a
// saved (flagged) post. Columns (posts.id/createat/channelid/rootid/deleteat,
// preferences.category/name) are the Mattermost DB schema.
//
// Thread grouping: rid = COALESCE(NULLIF(rootid,”), id) — a root post uses its
// own id, every reply uses the root id, so the whole thread shares one rid and
// MAX(createat) is the thread's newest message. Mattermost threads are flat
// (reply-to-reply still points at the root), so this groups at any depth.
const agedThreadsSQL = `
SELECT p.id
FROM posts p
JOIN (
    SELECT COALESCE(NULLIF(rootid, ''), id) AS rid, MAX(createat) AS mc
    FROM posts
    WHERE channelid = ? AND deleteat = 0
    GROUP BY rid
) g ON COALESCE(NULLIF(p.rootid, ''), p.id) = g.rid
WHERE p.channelid = ? AND p.deleteat = 0
  AND g.mc < ?
  AND g.rid NOT IN (
      SELECT COALESCE(NULLIF(fp.rootid, ''), fp.id)
      FROM posts fp
      WHERE fp.id IN (SELECT name FROM preferences WHERE category = 'flagged_post')
  )
LIMIT ?
`

// AgedThreads returns up to limit post ids in channelID whose thread's newest
// message is older than thresholdMs (i.e. the whole thread has aged past the
// TTL). Threads containing a saved post are protected and never returned.
func (f *Finder) AgedThreads(ctx context.Context, channelID string, thresholdMs int64, limit int) ([]string, error) {
	rows, err := f.db.QueryContext(ctx, sqlutil.Rebind(f.driver, agedThreadsSQL), channelID, channelID, thresholdMs, limit)
	if err != nil {
		return nil, fmt.Errorf("retention: aged threads %q: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("retention: scan %q: %w", channelID, err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
