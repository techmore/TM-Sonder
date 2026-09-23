# Sonder menu-bar status

Build with `make status-app`, then open `bin/Sonder Status.app`.
The small accessory app polls the private local API every 30 seconds, with a
four-second timeout and at most one request in flight. It reads the API port
and pairing token from `~/.config/sonder/server.json` (default API port 8097;
the web listener follows Jellyfin's native 8096 default).
The token is used only for local status/rebind requests and is never displayed
or logged. The app does not start another server or load the media catalog.

The menu is a compact native AppKit dashboard: a branded status header, a
two-column metric grid, a connection summary, and clearly separated Server,
Updates, and Tools sections. It shows running, scanning, updating, unreachable,
or unexpected-response status; item count; installed version; selected
interface/IP; private web/API binds and ports; uptime; Caddy/public pulse; and
links to the authenticated web interface and logs. **Scan Library** starts a
safe background reconciliation from the same menu. Its **Network & Access**
submenu contains **Bind interface**, which lists loopback, Wi-Fi/LAN, Ethernet,
VPN, public, and every active exact interface ID. **Change web port…** and
**Change API port…** submit the same atomic live exposure operation as the web
Settings panel. Binding, scanning, and port controls are disabled during a
change, and failed operations produce a native alert.

The menu also exposes **Check for Updates…** and **Install Update…** for the
Homebrew `tm-sonder` formula. Checks refresh Homebrew metadata only when the
user requests a check, with a quiet background check at launch and every ten
minutes. An update displays each stage in the menu—metadata refresh, download,
installation, service ownership, restart, and version verification—while
disabling conflicting controls. The updater restarts Sonder only when
`brew services` reports that the Homebrew instance is running. If the active
server is manually launched or repository-built, it installs the formula
update without starting a second server and explains that a manual restart is
required. The catalog and media files are never modified by the update
operation.
Opening Sonder from the menu includes the local pairing token in the web URL;
the status poll itself uses a Bearer header instead. Quitting the indicator does
not stop the launchd server. It has no Dock icon.

Install it as a persistent login menu-bar indicator with
`make install-status-app`. This builds the app, installs a per-user LaunchAgent,
and starts it immediately. Remove that integration with
`make uninstall-status-app`. Quitting the indicator does not stop the server.

Verification: compiled with Swift 6, checked plist syntax, and launched locally.
Status rendering and menu accessibility still benefit from manual UI review.
