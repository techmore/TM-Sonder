# Running the TM-Sonder server on this Mac

There are **two** ways this server can be started on this machine, and they use
**different config files and different data directories**. Confusing them is easy
and the failure is quiet: the wrong instance starts, binds the same ports, and
reports an empty library.

| | Homebrew service | LaunchAgent (the real instance) |
| --- | --- | --- |
| Label | `sh.brew.tm-sonder` | `com.sonder.server` |
| Config | `/opt/homebrew/etc/sonder/server.json` | `~/.config/sonder/server.json` |
| Data dir | `~/Library/Application Support/TM-Sonder-Server` | `~/.config/sonder/data` |
| Libraries | template placeholder `/path/to/media` | the four real NAS libraries |
| Catalog | empty (0 items) | ~21,700 items |

`brew services start local/tm-sonder/tm-sonder` starts the *template* config.
The formula's `service do` block hardcodes `etc/"sonder/server.json"`, and the
formula only writes `server.json.example` there, so the server generates an
untouched template on first run. It answers `/api/health` with `ok` and
`/api/status` with `itemCount: 0`, which looks like a broken scan rather than a
wrong instance.

**Use the LaunchAgent.** Stop the Homebrew service first if it is running, then:

```sh
brew services stop local/tm-sonder/tm-sonder          # if it was started
launchctl bootstrap gui/$(id -u) \
  "$HOME/Library/LaunchAgents/com.sonder.server.plist"
launchctl kickstart -k gui/$(id -u)/com.sonder.server
```

Verify you are talking to the right instance:

```sh
lsof -nP -iTCP:8097 -sTCP:LISTEN                        # sonder-da, not sonder
curl -s "http://127.0.0.1:8097/api/status" | head -c 200 # expect a non-zero itemCount
```

If `itemCount` is `0`, the wrong config is in use.

Two stale LaunchAgents also exist (`com.tm-sonder.server` and
`com.sonder.server`) with identical program arguments; only one is bootstrapped
at a time. Bootstrapping both fails with `address already in use`.

Logs for the real instance go to
`~/Library/Application Support/TM-Sonder-Server/logs/launchd-err.log`. The
Homebrew service logs to `/opt/homebrew/var/log/tm-sonder.log`.

## Library paths

`~/NAS` must be mounted before starting, or every library resolves to nothing.
See the NFS notes in the project handoff: the Synology export is NFS **v3** at
`/volume2/14tb` and only mounts with `vers=3,resvport,proto=tcp`.

## After changing parser or scanner output

`ParserVersion` in `server/internal/library/parser.go` must be bumped whenever
parse output semantics change, then `make mac` and restart. The incremental scan
skips files whose `parseVersion` already matches, so without the bump a catalog
fix appears to have no effect.
