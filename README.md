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

**M0 — Foundation (skeleton).** The plugin compiles, installs, activates and
registers a no-op post hook. The disappearing-messages lifecycle (TTL → expiry
index → sweeper → transactional purge) is implemented in subsequent milestones
(M1 Core, M2 Purge, M3 Hardening).

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
