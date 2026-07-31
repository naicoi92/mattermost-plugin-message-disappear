package main

import (
	"context"
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/api"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/expiry"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// Plugin implements the Mattermost server plugin hooks.
//
// Disappearing-messages lifecycle: TTL set (V2) -> expire index (V3.1) ->
// sweeper + transactional hard purge (V3.2/V4). This wiring activates the TTL
// service, the HTTP/slash API, and the expire index.
type Plugin struct {
	plugin.MattermostPlugin

	client        *pluginapi.Client
	ttlService    *ttl.Service
	apiHandler    *api.Handler
	expiryService *expiry.Service
}

// OnActivate wires the TTL service, the HTTP/slash API surface, the /disappear
// command, and — when DB access is available — the expire index.
func (p *Plugin) OnActivate() error {
	p.API.LogInfo("Disappearing Messages plugin activated")

	p.client = pluginapi.NewClient(p.API, p.Driver)
	p.ttlService = ttl.NewService(ttl.NewKVStore(p.API), p.API)
	p.apiHandler = api.New(p.ttlService, p.API)

	// Best-effort: a registration failure is logged but does not block activation
	// (the HTTP API still works). MM's RegisterCommand updates an existing command.
	if err := p.API.RegisterCommand(p.disappearCommand()); err != nil {
		p.API.LogError("failed to register /disappear command", "err", err)
	}

	// The expire index needs the master DB; if unavailable (e.g. no driver), the
	// plugin degrades gracefully — TTL set/view/off still work, only auto-delete is off.
	if p.Driver != nil {
		if err := p.initExpiry(context.Background()); err != nil {
			p.API.LogError("disappear: expire index disabled", "err", err)
		}
	}
	return nil
}

// initExpiry opens the master DB, migrates the expire-index table, and wires the
// ExpiryService. Returns nil on success (expiry enabled) or an error (disabled).
func (p *Plugin) initExpiry(ctx context.Context) error {
	db, err := p.client.Store.GetMasterDB()
	if err != nil {
		return err
	}
	store := expiry.NewSQLStore(db)
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	p.expiryService = expiry.NewService(store, p.ttlService)
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

// MessageHasBeenPosted indexes the post's expiry (D1/D5) when the channel has a TTL.
// Editing does NOT reset the TTL (D7) — there is intentionally no expiry update on edit.
func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, post *model.Post) {
	if p.expiryService == nil || post == nil || post.Id == "" {
		return
	}
	if err := p.expiryService.OnPostCreated(context.Background(), post); err != nil {
		p.API.LogError("disappear: failed to index post expiry", "post_id", post.Id, "err", err)
	}
}

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
