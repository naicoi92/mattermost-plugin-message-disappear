package main

import (
	"context"
	"errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/purge"
)

// errSchemaSkipped signals a fail-safe schema-guard skip (untested MM version).
// It is an error so the sweeper keeps the index rows and retries next tick
// instead of pruning them as if the posts had been purged.
var errSchemaSkipped = errors.New("disappear: purge skipped — MM schema version not in allowlist")

// Compile-time: the Mattermost plugin.API satisfies versionLogger.
var _ versionLogger = (plugin.API)(nil)

// softDeleter soft-deletes a post via MM's DeletePost (EnablePurge=false fallback).
type softDeleter interface {
	DeletePost(postID string) *model.AppError
}

// versionLogger is the plugin-API subset the configPurger needs.
type versionLogger interface {
	GetServerVersion() string
	GetLicense() *model.License
	LogInfo(msg string, keyvals ...any)
	LogError(msg string, keyvals ...any)
}

// configPurger implements purge.Purger and routes each purge to soft-delete, a
// schema-guarded skip, or the transactional hard purge based on the live config.
// It lets EnablePurge / PurgeSchemaAllowlist change at runtime without restarting.
type configPurger struct {
	cfg  *configHolder
	hard purge.Purger
	soft softDeleter
	api  versionLogger
}

func (p *configPurger) Purge(ctx context.Context, postIDs []string) (int, error) {
	if len(postIDs) == 0 {
		return 0, nil
	}
	cfg := p.cfg.get()
	switch purgeDecision(cfg.EnablePurge, p.api.GetLicense() != nil, p.api.GetServerVersion(), cfg.PurgeSchemaAllowlist()) {
	case purgeSoft:
		return p.softDelete(postIDs), nil
	case purgeSkip:
		// Fail-safe: untested MM schema -> touch nothing, alert, and return an error
		// so the sweeper keeps the index rows for retry (returning nil would make the
		// sweeper prune the rows, orphaning the posts from the plugin).
		p.api.LogError("disappear: hard purge skipped — MM schema version not in allowlist (fail-safe)", "version", p.api.GetServerVersion())
		return 0, errSchemaSkipped
	default: // purgeHard
		return p.hard.Purge(ctx, postIDs)
	}
}

// softDelete best-effort soft-deletes each post (EnablePurge off). Posts whose
// DeletePost fails are logged; the index rows are still pruned for the whole batch.
func (p *configPurger) softDelete(postIDs []string) int {
	for _, id := range postIDs {
		if appErr := p.soft.DeletePost(id); appErr != nil {
			p.api.LogError("disappear: soft-delete failed", "post_id", id, "err", appErr)
		}
	}
	return len(postIDs)
}
