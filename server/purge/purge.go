// Package purge performs the transactional hard delete (D10): it removes a post
// and its full data footprint (attachments, reactions, mentions) from the
// Mattermost database in a single all-or-nothing transaction.
//
// This is the highest-risk component (direct DB writes the plugin API does not
// support). The transaction guarantees no partial purge; deletion is idempotent
// (deleting already-removed rows is a no-op).
package purge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/sqlutil"
)

// Purger hard-deletes posts and their related data.
type Purger interface {
	// Purge deletes the given posts and all their related rows in one transaction.
	// It returns the number of post ids requested for deletion (callers must not
	// treat the count as rows-affected). Deleting non-existent ids is a no-op.
	Purge(ctx context.Context, postIDs []string) (int, error)
}

// footprint lists (table, post-id column) pairs purged together with each post.
// posts is keyed by its own id; the rest reference the post id. Column names are
// the Mattermost DB schema (lowercase, no underscore: "postid"), verified against
// the MM v10 schema. There is NO "mentions" table (mentions are derived from the
// message text, so deleting the post removes them); threads.postid is cleaned so
// a hard-deleted thread leaves no ghost thread row.
//
// IMPORTANT (D10 risk): these names are schema-dependent. Verify against the
// target MM version before enabling hard purge (PurgeSchemaAllowlist).
//
// Scope: this deletes the DB ROWS only. The plugin API exposes no file-delete
// capability, so attachment BLOBS (disk/object storage) are NOT removed — the
// fileinfo metadata row is, but the blob becomes orphaned. See README.
var footprint = []struct {
	table string
	col   string
}{
	{"posts", "id"},
	{"fileinfo", "postid"},
	{"reactions", "postid"},
	{"threads", "postid"},
}

// NewSQLPurger wraps a Mattermost master DB handle as a Purger. driver rebinds
// "?" placeholders to the DB's native form (postgres needs $N).
func NewSQLPurger(db *sql.DB, driver string) Purger {
	return &sqlPurger{db: db, driver: driver}
}

type sqlPurger struct {
	db     *sql.DB
	driver string
}

// Purge deletes every footprint row for the given post ids in one transaction.
func (p *sqlPurger) Purge(ctx context.Context, postIDs []string) (int, error) {
	if len(postIDs) == 0 {
		return 0, nil
	}

	placeholders, args := inClause(postIDs)

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("purge: begin tx: %w", err)
	}
	for _, f := range footprint {
		stmt := sqlutil.Rebind(p.driver, "DELETE FROM "+f.table+" WHERE "+f.col+" IN "+placeholders)
		if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("purge: delete %s.%s: %w", f.table, f.col, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("purge: commit: %w", err)
	}
	return len(postIDs), nil
}

// inClause builds "(?, ?, ...)" and the matching args slice for an IN list.
func inClause(ids []string) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return "(" + strings.Join(placeholders, ", ") + ")", args
}
