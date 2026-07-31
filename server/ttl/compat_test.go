package ttl_test

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// Compile-time proof that the Mattermost plugin.API satisfies both ports the
// TTL service depends on. Fails to compile if the MM API method signatures
// drift away from what the store/service expect.
var (
	_ ttl.KV                = (plugin.API)(nil)
	_ ttl.PermissionChecker = (plugin.API)(nil)
)

func TestPortsCompatibleWithMattermostAPI(t *testing.T) {
	t.Parallel()
}
