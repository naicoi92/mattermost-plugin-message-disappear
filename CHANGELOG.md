# Changelog

## 1.0.0

First release: disappearing messages for Mattermost (Team + Enterprise editions).

### Features

- **Per-channel TTL** (plugin KV + presets `5m`/`1h`/`8h`/`1d`/`1w` + custom `1m`–`1y`), default OFF, with D2 permission (system/channel admin; DM/Group = any participant).
- **HTTP API** (`POST`/`GET`/`DELETE` `/ttl`) + **`/disappear set|off|status`** slash command.
- **Webapp**: channel-header button + TTL selector modal (D8 E2EE warning) + post badge (⏱); `ttl_changed` WebSocket event.
- **Expire index** (`mpmd_expire` SQL) + thread-level expiry — a reply extends the whole thread (D5); editing does not reset it (D7).
- **HA sweeper** (`cluster.Schedule`, single cluster node — no double-purge).
- **Transactional hard purge (D10)**: `posts`/`fileinfo`/`reactions`/`mentions` removed in one all-or-nothing `DELETE`.
- **Schema-version guard** (`PurgeSchemaAllowlist`): fail-safe skip on unverified MM schemas.
- **`EnablePurge` toggle** (soft-delete fallback) + **EE legal-hold coexist**: hard purge on Team; soft-delete on Enterprise (so MM's `DeletePost` honours legal-hold, D11).

### Known limitations

- Physical attachment blobs are **not** deleted (the MM plugin API exposes no file-delete capability); `fileinfo` rows are. Orphan blobs are cleaned via MM storage maintenance or native Data Retention.
- Disappearing ≠ end-to-end encrypted (D8) — the server reads plaintext until deletion.
- The hard purge is schema-dependent; verify the footprint column names against your MM version.

### Requirements

- Mattermost Server **>= 10.0.0** (Team or Enterprise).
