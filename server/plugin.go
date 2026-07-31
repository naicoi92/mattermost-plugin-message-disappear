package main

import (
	"context"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/api"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/expiry"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/purge"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/sweeper"
	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

const sweeperInterval = 60 * time.Second

// Plugin implements the Mattermost server plugin hooks.
//
// Disappearing-messages lifecycle: TTL set (V2) -> expire index (V3.1) ->
// sweeper + transactional hard purge (V3.2/V4). This wiring activates the TTL
// service, the HTTP/slash API, the expire index, the HA sweeper and the Purger.
type Plugin struct {
	plugin.MattermostPlugin

	client        *pluginapi.Client
	ttlService    *ttl.Service
	apiHandler    *api.Handler
	expireStore   expiry.ExpireIndexStore
	expiryService *expiry.Service
	purger        purge.Purger
	sweeper       *sweeper.Sweeper
	sweeperJob    *cluster.Job
}

// OnActivate wires the TTL service, the HTTP/slash API surface, the /disappear
// command, and — when DB access is available — the expire index + Purger + HA sweeper.
func (p *Plugin) OnActivate() error {
	p.API.LogInfo("Disappearing Messages plugin activated")

	p.client = pluginapi.NewClient(p.API, p.Driver)
	p.ttlService = ttl.NewService(ttl.NewKVStore(p.API), p.API)
	p.apiHandler = api.New(p.ttlService, p.API)

	// Best-effort: a registration failure is logged but does not block activation.
	if err := p.API.RegisterCommand(p.disappearCommand()); err != nil {
		p.API.LogError("failed to register /disappear command", "err", err)
	}

	// The expire index + Purger need the master DB; if unavailable, the plugin
	// degrades gracefully — TTL set/view/off still work, only auto-delete is off.
	if p.Driver != nil {
		if err := p.initExpiry(context.Background()); err != nil {
			p.API.LogError("disappear: expire index disabled", "err", err)
		} else if err := p.initSweeper(); err != nil {
			p.API.LogError("disappear: sweeper disabled", "err", err)
		}
	}
	return nil
}

// initExpiry opens the master DB, migrates the expire-index table, and wires the
// ExpiryService and the transactional Purger. Returns nil on success or an error.
func (p *Plugin) initExpiry(ctx context.Context) error {
	db, err := p.client.Store.GetMasterDB()
	if err != nil {
		return err
	}
	p.expireStore = expiry.NewSQLStore(db)
	if err := p.expireStore.Migrate(ctx); err != nil {
		return err
	}
	p.expiryService = expiry.NewService(p.expireStore, p.ttlService)
	p.purger = purge.NewSQLPurger(db)
	return nil
}

// initSweeper schedules the single-node HA sweeper (cluster.Schedule).
func (p *Plugin) initSweeper() error {
	p.sweeper = sweeper.New(p.expireStore, p.purger, p.API, 500)
	job, err := cluster.Schedule(p.API, "disappear_sweeper", cluster.MakeWaitForRoundedInterval(sweeperInterval), p.sweeper.Run)
	if err != nil {
		return err
	}
	p.sweeperJob = job
	return nil
}

// OnDeactivate stops the sweeper and logs deactivation.
func (p *Plugin) OnDeactivate() error {
	if p.sweeperJob != nil {
		if err := p.sweeperJob.Close(); err != nil {
			p.API.LogError("disappear: sweeper close failed", "err", err)
		}
	}
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
