package purge

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // pure-Go sqlite for the integration test

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupFootprintDB creates the MM-like footprint tables in an in-memory sqlite DB.
func setupFootprintDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for _, ddl := range []string{
		`CREATE TABLE posts (id TEXT PRIMARY KEY)`,
		`CREATE TABLE fileinfo (post_id TEXT)`,
		`CREATE TABLE reactions (post_id TEXT)`,
		`CREATE TABLE mentions (post_id TEXT)`,
	} {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}
	return db
}

// seed inserts a post + one related row in each footprint table for the given id.
func seed(t *testing.T, db *sql.DB, postID string) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO posts (id) VALUES (?)`,
		`INSERT INTO fileinfo (post_id) VALUES (?)`,
		`INSERT INTO reactions (post_id) VALUES (?)`,
		`INSERT INTO mentions (post_id) VALUES (?)`,
	} {
		_, err := db.Exec(q, postID)
		require.NoError(t, err)
	}
}

func count(t *testing.T, db *sql.DB, table, col, postID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?`, postID).Scan(&n))
	return n
}

func TestPurgeEmptyIsNoOp(t *testing.T) {
	db := setupFootprintDB(t)
	n, err := NewSQLPurger(db).Purge(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestPurgeDeletesFullFootprint(t *testing.T) {
	db := setupFootprintDB(t)
	seed(t, db, "p1")
	seed(t, db, "p2")
	seed(t, db, "p3") // must survive

	n, err := NewSQLPurger(db).Purge(context.Background(), []string{"p1", "p2"})
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	for _, f := range footprint {
		assert.Equalf(t, 0, count(t, db, f.table, f.col, "p1"), "p1 should be gone from %s", f.table)
		assert.Equalf(t, 0, count(t, db, f.table, f.col, "p2"), "p2 should be gone from %s", f.table)
		assert.Equalf(t, 1, count(t, db, f.table, f.col, "p3"), "p3 should survive in %s", f.table)
	}
}

func TestPurgeIsIdempotent(t *testing.T) {
	db := setupFootprintDB(t)
	seed(t, db, "p1")

	_, err := NewSQLPurger(db).Purge(context.Background(), []string{"p1"})
	require.NoError(t, err)
	// Purging again (already deleted) is a no-op, no error.
	n, err := NewSQLPurger(db).Purge(context.Background(), []string{"p1"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, count(t, db, "posts", "id", "p1"))
}

// TestPurgeRollbackOnError appends a non-existent footprint table so a mid-tx
// DELETE fails; the whole transaction must roll back (no partial delete).
func TestPurgeRollbackOnError(t *testing.T) {
	db := setupFootprintDB(t)
	seed(t, db, "p1")

	orig := footprint
	footprint = append(footprint, struct {
		table string
		col   string
	}{"nope_table", "post_id"})
	t.Cleanup(func() { footprint = orig })

	_, err := NewSQLPurger(db).Purge(context.Background(), []string{"p1"})
	require.Error(t, err)
	// Rollback preserved every footprint row (atomicity, no partial purge).
	for _, f := range footprint[:len(footprint)-1] {
		assert.Equalf(t, 1, count(t, db, f.table, f.col, "p1"), "%s row must survive rollback", f.table)
	}
}
