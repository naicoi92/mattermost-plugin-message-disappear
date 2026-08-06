package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"

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
// service, the HTTP/slash API, the expire index, the background sweeper and the Purger.
type Plugin struct {
	plugin.MattermostPlugin

	client *pluginapi.Client
	// db + sqlDriver are injected by tests; production opens the master DB via
	// pluginapi in OnActivate and reads the SQL driver from the server config.
	// sqlDriver selects the placeholder style ("?" vs the postgres "$N" form).
	db        *sql.DB
	sqlDriver string
	cfg           configHolder
	ttlService    *ttl.Service
	apiHandler    *api.Handler
	expireStore   expiry.ExpireIndexStore
	expiryService *expiry.Service
	purger        purge.Purger
	sweeper       *sweeper.Sweeper
	sweeperCancel context.CancelFunc
}

// OnActivate wires the TTL service, the HTTP/slash API surface, the /disappear
// command, and — backed by the master DB — the expire index, Purger and sweeper.
func (p *Plugin) OnActivate() error {
	p.API.LogInfo("Disappearing Messages plugin activated")
	p.loadConfig()

	p.client = pluginapi.NewClient(p.API, p.Driver)

	// The TTL store, expire index and Purger all persist in the Mattermost
	// master DB (a single handle, shared). Plugin KV is intentionally not used:
	// on Mattermost 10.x the plugin RPC shuts down during reload/deactivate and
	// every KVSetWithOptions fails with "connection is shut down". Tests inject
	// p.db; production opens it via pluginapi.
	db := p.db
	if db == nil {
		var err error
		db, err = p.client.Store.GetMasterDB()
		if err != nil {
			return fmt.Errorf("disappear: master DB unavailable: %w", err)
		}
	}
	driver := p.sqlDriver
	if driver == "" {
		driver = p.masterDriver()
	}
	if err := p.wirePersistence(context.Background(), db, driver); err != nil {
		return err
	}

	// Best-effort: a registration failure is logged but does not block activation.
	if err := p.API.RegisterCommand(p.disappearCommand()); err != nil {
		p.API.LogError("failed to register /disappear command", "err", err)
	}
	return nil
}

// wirePersistence migrates the TTL and expire-index tables and wires the TTL
// service, expire index, transactional Purger, and (against a real server) the
// background sweeper onto a single master DB handle. driver rebinds "?" to the
// DB's native placeholder form.
func (p *Plugin) wirePersistence(ctx context.Context, db *sql.DB, driver string) error {
	ttlStore := ttl.NewSQLStore(db, driver)
	if err := ttlStore.Migrate(ctx); err != nil {
		return fmt.Errorf("disappear: TTL store migrate: %w", err)
	}
	p.ttlService = ttl.NewService(ttlStore, p.API)
	p.apiHandler = api.New(p.ttlService, p.API)

	p.expireStore = expiry.NewSQLStore(db, driver)
	if err := p.expireStore.Migrate(ctx); err != nil {
		return fmt.Errorf("disappear: expire index migrate: %w", err)
	}
	p.expiryService = expiry.NewService(p.expireStore, p.ttlService)
	p.purger = purge.NewSQLPurger(db)

	// The sweeper runs only against a real server (Driver set). Tests inject a
	// DB but have no Driver, so they don't spawn the background goroutine.
	if p.Driver != nil {
		p.initSweeper()
	}
	return nil
}

// masterDriver returns the Mattermost SQL driver name, defaulting to "postgres"
// (the MM v10 default and the target deployment) so "?" placeholders are
// rebound to "$N" even if the config read fails.
func (p *Plugin) masterDriver() string {
	if cfg := p.API.GetConfig(); cfg != nil && cfg.SqlSettings.DriverName != nil {
		return *cfg.SqlSettings.DriverName
	}
	return "postgres"
}

// initSweeper starts the background sweeper on a fixed interval.
//
// The sweeper runs on its own goroutine driven by a context that OnDeactivate
// cancels, so shutdown is prompt and never blocks on a dead RPC. Purge is
// idempotent (deleting already-removed rows is a no-op), so on a multi-node
// cluster every node may sweep concurrently without corruption — the worst case
// is redundant delete attempts.
//
// This intentionally replaces cluster.Schedule: its KV-mutex pinger (Lock over
// context.Background) spins forever once the plugin's RPC connection is shut
// down, hanging OnDeactivate and flooding the log with "connection is shut down".
func (p *Plugin) initSweeper() {
	// Route purges through the config switch (hard purge gated by the schema-version
	// allowlist; soft-delete fallback when EnablePurge is off or the schema is untested).
	swPurger := &configPurger{cfg: &p.cfg, hard: p.purger, soft: p.API, api: p.API}
	p.sweeper = sweeper.New(p.expireStore, swPurger, p.API, 500)

	ctx, cancel := context.WithCancel(context.Background())
	p.sweeperCancel = cancel
	go p.runSweeper(ctx)
}

// runSweeper drains the backlog once on activation, then sweeps on every
// interval until ctx is cancelled by OnDeactivate.
func (p *Plugin) runSweeper(ctx context.Context) {
	p.sweeper.Run() // sweep the backlog promptly on activation

	ticker := time.NewTicker(sweeperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sweeper.Run()
		}
	}
}

// OnDeactivate cancels the sweeper (its goroutine exits within one tick) and
// logs deactivation. Unlike the former cluster.Job, this never blocks on RPC.
func (p *Plugin) OnDeactivate() error {
	if p.sweeperCancel != nil {
		p.sweeperCancel()
	}
	p.API.LogInfo("Disappearing Messages plugin deactivated")
	return nil
}

// loadConfig reads the System-Console configuration into the live config holder.
func (p *Plugin) loadConfig() {
	var c configuration
	if err := p.API.LoadPluginConfiguration(&c); err != nil {
		p.API.LogError("disappear: failed to load config, using defaults", "err", err)
		return
	}
	p.cfg.set(c)
}

// OnConfigurationChange reloads configuration when the System Console changes it.
func (p *Plugin) OnConfigurationChange() error {
	p.loadConfig()
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
