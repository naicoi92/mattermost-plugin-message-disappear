package main

import (
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/api"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// Plugin implements the Mattermost server plugin hooks.
//
// The disappearing-messages lifecycle (TTL set -> expiry index -> sweeper ->
// transactional hard purge) is built up across V2/V3/V4. This wiring activates
// the TTL service and the HTTP + slash-command API surface.
type Plugin struct {
	plugin.MattermostPlugin

	ttlService *ttl.Service
	apiHandler *api.Handler
}

// OnActivate wires the TTL service, the HTTP/slash API surface and registers
// the /disappear command.
func (p *Plugin) OnActivate() error {
	p.API.LogInfo("Disappearing Messages plugin activated")

	p.ttlService = ttl.NewService(ttl.NewKVStore(p.API), p.API)
	p.apiHandler = api.New(p.ttlService, p.API)

	// Best-effort: a registration failure is logged but does not block activation
	// (the HTTP API still works). MM's RegisterCommand updates an existing command.
	if err := p.API.RegisterCommand(p.disappearCommand()); err != nil {
		p.API.LogError("failed to register /disappear command", "err", err)
	}
	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	p.API.LogInfo("Disappearing Messages plugin deactivated")
	return nil
}

// ServeHTTP routes plugin HTTP requests (/plugins/<id>/ttl...) to the API handler.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.apiHandler.ServeHTTP(w, r)
}

// ExecuteCommand dispatches /disappear set|off|status to the API handler.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	return p.apiHandler.ExecuteCommand(args)
}

// MessageHasBeenPosted is a no-op placeholder; the expiry-indexing logic that
// consumes a channel's TTL lands in V3.1 (ExpiryService, MPMD-20).
func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, _ *model.Post) {}

func (p *Plugin) disappearCommand() *model.Command {
	return &model.Command{
		Trigger:          api.CommandTrigger,
		DisplayName:      "Disappearing Messages",
		Description:      "Manage disappearing messages per channel",
		AutoComplete:     true,
		AutoCompleteDesc: "Set, view or clear the channel's disappearing-message TTL",
		AutoCompleteHint: "[set <duration> | status | off]",
	}
}
