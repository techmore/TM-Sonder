# Sonder menu-bar status

Build with `make status-app`, then open `bin/Sonder Status.app`.
The small accessory app polls the local Go server every ten seconds, with a
four-second timeout and at most one request in flight. It reads the port and
pairing token from `~/.config/sonder/server.json` (default port 8797). The
token is used only for the local status request and is never displayed or
logged. The app does not start another server or load the media catalog.

The menu shows running, scanning, updating, unreachable, or unexpected-response
status; item count; and links to the authenticated web interface and logs.
Opening Sonder from the menu includes the local pairing token in the web URL;
the status poll itself uses a Bearer header instead. Quitting the indicator does
not stop the launchd server. It has no Dock icon.

Install it as a persistent login menu-bar indicator with
`make install-status-app`. This builds the app, installs a per-user LaunchAgent,
and starts it immediately. Remove that integration with
`make uninstall-status-app`. Quitting the indicator does not stop the server.

Verification: compiled with Swift 6, checked plist syntax, and launched locally.
Status rendering and menu accessibility still benefit from manual UI review.
