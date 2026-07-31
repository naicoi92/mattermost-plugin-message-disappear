# Disappearing Messages (Mattermost Plugin)

Brings Enterprise-grade **hard-delete message retention** to the Mattermost
**Team** edition: set a per-channel TTL and, once messages expire, they are
**purged from the database** — not merely soft-deleted.

> ⚠️ **Hard delete via direct DB purge.** This plugin deletes posts, file info,
> reactions and mentions from the Mattermost database via a transactional
> `DELETE`. This is unsupported by the Mattermost plugin API and is
> schema-dependent. Back up your database and review the design notes before
> enabling in production. See `min_server_version` and the risk register in the
> design docs.

## Status

Core lifecycle implemented: per-channel TTL (KV + presets + permission), HTTP API +
`/disappear` slash command, webapp badge + selector modal, expire index (SQL),
HA sweeper (`cluster.Schedule`), and a **transactional hard purge** gated by a
schema-version guard (`PurgeSchemaAllowlist`) with an `EnablePurge` soft-delete
fallback. Remaining: EE legal-hold coexist (V5) and release hardening (V6).

## Known limitations

- **Physical attachment blobs are NOT deleted.** The Mattermost plugin API exposes no
  file-delete capability (only upload/read/copy), so the purge deletes the `fileinfo`
  metadata rows (within the transaction) but cannot remove the underlying file blobs
  from disk/object storage. Orphaned blobs must be cleaned via Mattermost storage
  maintenance or native Data Retention (Enterprise). This is a platform constraint,
  not a bug.
- **Disappearing ≠ end-to-end encrypted.** The server reads messages in plaintext until
  deletion (D8). This is ephemeral UX + data minimization, not a security guarantee.
- **Schema-dependent purge.** Hard purge binds to tested MM versions via
  `PurgeSchemaAllowlist`; it is skipped (fail-safe) on unverified schemas. Verify the
  footprint column names against your MM version before production use.
- **Enterprise legal-hold respected via soft-delete.** On a licensed Enterprise server
  the plugin's direct DB DELETE would bypass legal-hold (enforced at the API layer,
  not the DB) — and the plugin API exposes no way to query legal-hold. So on Enterprise
  the sweeper falls back to Mattermost's soft-delete, which honours legal-hold (D11).
  Hard purge runs on the Team edition (no legal-hold). The plugin and native Data
  Retention coexist independently (first-deletion-wins, idempotent).

## Requirements

- Mattermost Server **>= 10.0.0** (Team or Enterprise edition)
- Go **1.25+** (server build)
- Node.js **20+** with npm (webapp build)
- `jq` (manifest parsing in the Makefile)

## Build

```sh
make all        # check-style + test + dist (produces dist/<id>-<version>.tar.gz)
make server     # cross-compile the server only
make webapp     # build the webapp bundle only (webapp/dist/main.js)
make dist       # server + webapp + bundle
```

### Checks

```sh
make check-style  # gofmt + golangci-lint + webapp typecheck
make test         # go test with a coverage profile
make coverage     # report total coverage (target >= 80%)
```

## Install

```sh
make deploy
# or, without a deploy target configured:
make dist && mmctl plugin upload dist/com.github.naicoi92.disappearing-messages-0.1.0.tar.gz
```

Then enable the plugin via **System Console → Plugins → Plugin Management**, or:

```sh
mmctl plugin enable com.github.naicoi92.disappearing-messages
```

On activation the server logs `Disappearing Messages plugin activated`.

## Project layout

```
plugin.json              # manifest (id, min_server_version, executables, webapp bundle)
Makefile                 # self-contained build (cross-compile + webapp bundle + deploy)
server/                  # Go server plugin
  main.go                # entrypoint — plugin.ClientMain
  plugin.go              # MattermostPlugin: OnActivate/OnDeactivate + no-op post hook
  plugin_test.go
webapp/                  # TypeScript webapp plugin
  src/index.tsx          # registerPlugin (no-op UI until V2.3)
.github/workflows/ci.yml # gofmt + golangci-lint + test (coverage gate) + webapp build + bundle
```

## License

Apache-2.0 (see `LICENSE`).
