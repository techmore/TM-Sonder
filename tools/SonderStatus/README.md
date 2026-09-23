# Sonder menu-bar status

Build with `make status-app`, then open `bin/Sonder Status.app`.
The small accessory app polls the private local API every 30 seconds, with a
four-second timeout and at most one request in flight. It reads the API port
and pairing token from `~/.config/sonder/server.json` (default API port 8798).
The token is used only for local status/rebind requests and is never displayed
or logged. The app does not start another server or load the media catalog.

The menu shows running, scanning, updating, unreachable, or unexpected-response
status; item count; installed version; selected interface/IP; private web/API
binds; uptime; Caddy/public pulse; and links to the authenticated web
interface and logs. Its **Bind interface** submenu lists loopback, Wi-Fi/LAN,
Ethernet, VPN, public, and every active exact interface ID. Binding controls
are disabled during a swap, and failed swaps produce a native alert.
Opening Sonder from the menu includes the local pairing token in the web URL;
the status poll itself uses a Bearer header instead. Quitting the indicator does
not stop the launchd server. It has no Dock icon.

Install it as a persistent login menu-bar indicator with
`make install-status-app`. This builds the app, installs a per-user LaunchAgent,
and starts it immediately. Remove that integration with
`make uninstall-status-app`. Quitting the indicator does not stop the server.

Verification: compiled with Swift 6, checked plist syntax, and launched locally.
Status rendering and menu accessibility still benefit from manual UI review.
