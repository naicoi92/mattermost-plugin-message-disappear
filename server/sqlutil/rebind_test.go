package sqlutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRebindSqliteLeavesQuestionMarks(t *testing.T) {
	in := "SELECT a FROM t WHERE id = ? AND x <= ? ORDER BY a;"
	assert.Equal(t, in, Rebind("sqlite", in))
	assert.Equal(t, in, Rebind("mysql", in))
	assert.Equal(t, in, Rebind("", in))
}

func TestRebindPostgresUsesDollarPlaceholders(t *testing.T) {
	in := "SELECT a FROM t WHERE id = ? AND x <= ? ORDER BY a;"
	want := "SELECT a FROM t WHERE id = $1 AND x <= $2 ORDER BY a;"
	assert.Equal(t, want, Rebind("postgres", in))
	assert.Equal(t, want, Rebind("pgx", in))
	// "pgx" / "Postgres " with whitespace/case are still recognised.
	assert.Equal(t, want, Rebind("  Postgres ", in))
}

// Reproduces the production failure: "?" is the Postgres JSONB operator, so an
// un-rebound "WHERE expire_at <= ? ORDER BY" parses as a syntax error at ORDER.
// After rebind the placeholder is gone.
func TestRebindFixesExpireSweeperQuery(t *testing.T) {
	in := "SELECT post_id, channel_id, expire_at FROM mpmd_expire WHERE expire_at <= ? ORDER BY expire_at LIMIT ?;"
	want := "SELECT post_id, channel_id, expire_at FROM mpmd_expire WHERE expire_at <= $1 ORDER BY expire_at LIMIT $2;"
	assert.Equal(t, want, Rebind("postgres", in))
}

func TestRebindInClause(t *testing.T) {
	assert.Equal(t, "DELETE FROM t WHERE id IN ($1, $2, $3)", Rebind("postgres", "DELETE FROM t WHERE id IN (?, ?, ?)"))
}

func TestIsPostgres(t *testing.T) {
	assert.True(t, IsPostgres("postgres"))
	assert.True(t, IsPostgres("pgx"))
	assert.True(t, IsPostgres(" Postgres "))
	assert.False(t, IsPostgres("mysql"))
	assert.False(t, IsPostgres("sqlite"))
	assert.False(t, IsPostgres(""))
}
