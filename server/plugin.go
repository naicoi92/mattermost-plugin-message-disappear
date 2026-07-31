package main

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the Mattermost server plugin hooks.
//
// The disappearing-messages lifecycle (TTL set -> expiry index -> sweeper ->
// transactional hard purge) is added in V2/V3/V4. This skeleton wires activation
// logging and a no-op post hook so plugin load and event wiring are verifiable
// on a real server.
type Plugin struct {
	plugin.MattermostPlugin
}

// OnActivate is invoked when the plugin is activated. Logging here proves the
// plugin binary loaded and registered with the Mattermost server.
func (p *Plugin) OnActivate() error {
	p.API.LogInfo("Disappearing Messages plugin activated")
	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	p.API.LogInfo("Disappearing Messages plugin deactivated")
	return nil
}

// MessageHasBeenPosted is a no-op placeholder proving the plugin receives post
// events. The real expiry-indexing logic lands in V3.1 (ExpiryService, MPMD-20)
// and replaces this implementation.
func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, _ *model.Post) {
}
