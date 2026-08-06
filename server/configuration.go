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

// purgeMode is the sweeper's deletion decision. Typed (not bare strings) so a
// typo in the dispatch switch can't silently fall through to the hard path
// (which would bypass Enterprise legal-hold, D11).
type purgeMode string

const (
	purgeSoft purgeMode = "soft" // soft-delete (EnablePurge off, Enterprise legal-hold, or untested schema)
	purgeHard purgeMode = "hard" // transactional DB purge
)

// purgeDecision picks the sweeper's deletion mode for the current config + server
// edition/version.
//
// Hard purge runs only when ALL hold: EnablePurge is on, the server is not
// Enterprise-licensed (a direct DB DELETE would bypass legal-hold, which MM
// enforces at the API/store layer — D11), and the server version matches the
// schema allowlist (D10 risk mitigation: the hard-purge footprint uses hardcoded
// table/column names verified only for allowlisted versions).
//
// Otherwise the sweeper soft-deletes via MM's DeletePost. Soft-delete is
// schema-agnostic and honours legal-hold, so it is always safe — including the
// previously-skipped "untested schema" case. Skipping made "enable purge" a
// silent no-op (messages never cleaned), which is worse than the default; the
// safe floor is now soft-delete, never "do nothing".
func purgeDecision(enablePurge, isEnterprise bool, serverVersion string, allowlist []string) purgeMode {
	if enablePurge && !isEnterprise && versionAllowed(serverVersion, allowlist) {
		return purgeHard
	}
	return purgeSoft
}
