// Package sqlutil holds cross-driver SQL helpers. The plugin persists to the
// Mattermost master DB, whose driver may need placeholders other than "?".
package sqlutil

import (
	"strconv"
	"strings"
)

// IsPostgres reports whether the Mattermost SQL driver name is postgres.
func IsPostgres(driver string) bool {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "pgx":
		return true
	}
	return false
}

// Rebind converts "?" placeholders to the driver's native form. Postgres
// (lib/pq / pgx) requires $1, $2, …; sqlite and mysql keep "?". A bare "?" is
// also the Postgres JSONB key-exists operator, so leaving "?" in a postgres
// query is a hard syntax error (e.g. "syntax error at or near ORDER"), not a
// silent no-op — which is why the expiry sweeper never purged on Postgres.
func Rebind(driver, query string) string {
	if !IsPostgres(driver) {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 16)
	n := 0
	for i := range query {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
