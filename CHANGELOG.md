# Changelog

## Unreleased

### Fixed

- **Disappearing messages were never purged when `EnablePurge` was on with an empty/unmatched `PurgeSchemaAllowlist`.** The schema guard returned a fail-safe *skip*, making "enable purge" a silent no-op (messages never cleaned) — worse than the default. The guard now falls back to **soft-delete** (MM `DeletePost`), which is schema-agnostic and always safe. Hard purge still runs when the version is allowlisted; to get hard (no-trace) deletion set `PurgeSchemaAllowlist` to your server version prefix (e.g. `10.`).
- **`RPC call to KVSetWithOptions API failed: connection is shut down` spam + `OnDeactivate` hang.** The sweeper ran via `cluster.Schedule`, whose KV-mutex pinger (`Lock` over an uncancellable context) spun forever once the plugin's RPC connection shut down — flooding the log and blocking deactivation until Mattermost force-killed the plugin. Replaced with a `time.Ticker` goroutine driven by a context cancelled in `OnDeactivate`: clean, prompt shutdown, no KV spam. Trade-off: drops cluster-wide single-execution (purge is idempotent, so concurrent nodes sweeping is safe — worst case redundant deletes).
- **Setting a TTL failed with "ttl: channel not found … nil channel, no app error" on Mattermost 10.x.** `plugin.API.GetChannel` returns `(nil, nil)` for an otherwise-valid team channel on 10.x, which the D2 permission check relied on — so every TTL set (slash command and channel-header button alike) was rejected. `checkCanManage` now authorises team-channel admins via the channel-scoped permission (`HasPermissionToChannel`, no channel object needed) and consults `GetChannel` only to detect DM/Group DMs. Team channels are fixed; DM/GM support degrades when `GetChannel` is unavailable.

### Changed

- **`make deploy` / `make deploy-linux-amd64` now stamp the short commit hash onto the build.** Every deployed plugin carries `version+<shorthash>` (semver build metadata, e.g. `1.1.0+018952b`) in its manifest — visible in the Mattermost plugin page — and the bundle is named `…-1.1.0+018952b.tar.gz`, with a deploy log line `Deploying … (commit …)`. The source `plugin.json` and generated manifests stay clean; only the deployed artifact is tagged. Wires up the `BuildHashShort` infra that was gathered but unused.

## 1.1.0

Minor release: new channel-header UX (TTL status + quick-select) and the canonical plugin-id rename, plus build/deploy tooling.

### Added

- **Channel-header TTL status + quick-select dropdown** ([MPMD-31], #13): the header button now shows live status (⏱ + duration when a TTL is set, muted ⏱ when off) with a hover tooltip (duration + set_by + set_at); the channel-header menu offers quick presets (`5m`/`1h`/`8h`/`1d`/`1w`) + Off + Custom. Shares a `useChannelTTL` hook with the post badge. Webapp-only, no server change.
- `make deploy-linux-amd64` (+ `server-linux-amd64`, `dist-linux-amd64`): cross-compile only the linux/amd64 server binary and bundle it — suited for Mattermost on k8s (linux/amd64) (#14).

### Changed

- **Plugin id renamed** to `com.mattermost.plugin-message-disappear` (mattermost-plugin convention `com.mattermost.plugin-*`): plugin.json, server `PluginID`, webapp manifest, README (#12). ⚠️ Upgrading from an install using the previous id registers as a new plugin — remove the old one first.
- Shared upload logic (`pluginctl` / `curl` + `MM_ADMIN_TOKEN` / manual) extracted into a reusable `upload` target; both `deploy` and `deploy-linux-amd64` call it (#14).

### Fixed

- `make help` hid any target whose name contains digits (`amd64`/`arm64`): target regex `[a-zA-Z_-]+` → `[a-zA-Z0-9_-]+`; help column widened `16` → `20` (#14).

## 1.0.0

First release: disappearing messages for Mattermost (Team + Enterprise editions).

### Features

- **Per-channel TTL** (plugin KV + presets `5m`/`1h`/`8h`/`1d`/`1w` + custom `1m`–`1y`), default OFF, with D2 permission (system/channel admin; DM/Group = any participant).
- **HTTP API** (`POST`/`GET`/`DELETE` `/ttl`) + **`/disappear set|off|status`** slash command.
- **Webapp**: channel-header button + TTL selector modal (D8 E2EE warning) + post badge (⏱); `ttl_changed` WebSocket event.
- **Expire index** (`mpmd_expire` SQL) + thread-level expiry — a reply extends the whole thread (D5); editing does not reset it (D7).
- **Background sweeper** (ticker goroutine; purge is idempotent, so concurrent cluster nodes are safe).
- **Transactional hard purge (D10)**: `posts`/`fileinfo`/`reactions`/`mentions` removed in one all-or-nothing `DELETE`.
- **Schema-version guard** (`PurgeSchemaAllowlist`): soft-delete fallback on unverified MM schemas (hard purge only on allowlisted versions).
- **`EnablePurge` toggle** (soft-delete fallback) + **EE legal-hold coexist**: hard purge on Team; soft-delete on Enterprise (so MM's `DeletePost` honours legal-hold, D11).

### Known limitations

- Physical attachment blobs are **not** deleted (the MM plugin API exposes no file-delete capability); `fileinfo` rows are. Orphan blobs are cleaned via MM storage maintenance or native Data Retention.
- Disappearing ≠ end-to-end encrypted (D8) — the server reads plaintext until deletion.
- The hard purge is schema-dependent; verify the footprint column names against your MM version.

### Requirements

- Mattermost Server **>= 10.0.0** (Team or Enterprise).
