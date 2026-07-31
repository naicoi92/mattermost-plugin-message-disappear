package main

import (
	"strings"
	"sync"
)

// configuration is the plugin's external configuration (doc 05 §4), loaded from
// the Mattermost System Console via LoadPluginConfiguration.
type configuration struct {
	// EnablePurge toggles the transactional hard DB purge. When false the sweeper
	// falls back to MM's soft-delete (graceful degradation, safe on unknown schemas).
	EnablePurge bool `json:"enablepurge,omitempty"`
	// PurgeSchemaAllowlistRaw is a comma-separated list of tested Mattermost server
	// version prefixes (e.g. "10.,11."). The hard purge only runs on a version that
	// matches one of these prefixes (schema-version guard, D10 risk mitigation).
	PurgeSchemaAllowlistRaw string `json:"purgeschemaallowlist,omitempty"`
}

// PurgeSchemaAllowlist parses the raw comma-separated allowlist into trimmed entries.
func (c *configuration) PurgeSchemaAllowlist() []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, e := range strings.Split(c.PurgeSchemaAllowlistRaw, ",") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// configHolder guards the active configuration (plugins are concurrent).
type configHolder struct {
	mu   sync.RWMutex
	conf configuration
}

func (h *configHolder) get() configuration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conf
}

func (h *configHolder) set(c configuration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conf = c
}

// versionAllowed reports whether serverVersion matches an allowlist entry by prefix.
// An empty allowlist means "no tested version" -> fail-safe (not allowed).
func versionAllowed(serverVersion string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return false
	}
	for _, allowed := range allowlist {
		if strings.HasPrefix(serverVersion, allowed) {
			return true
		}
	}
	return false
}

// purgeDecision picks the sweeper's deletion mode for the current config + server
// edition/version: "soft" (EnablePurge off, OR Enterprise for legal-hold safety),
// "skip" (untested schema -> fail-safe, no data touched), or "hard" (transactional
// DB purge).
//
// On Enterprise the plugin's direct DB DELETE would bypass legal-hold (which MM
// enforces at the API/store layer, not the DB). The plugin API exposes no way to
// query legal-hold, so on a licensed (Enterprise) server the sweeper falls back to
// soft-delete so MM's DeletePost honours it (D11: do not bypass compliance).
func purgeDecision(enablePurge, isEnterprise bool, serverVersion string, allowlist []string) string {
	if !enablePurge || isEnterprise {
		return "soft"
	}
	if !versionAllowed(serverVersion, allowlist) {
		return "skip"
	}
	return "hard"
}
