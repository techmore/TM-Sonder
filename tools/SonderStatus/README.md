# Sonder menu-bar status

Build with `make status-app`, then open `bin/Sonder Status.app`.
The small accessory app polls the local Go server every ten seconds, with a
four-second timeout and at most one request in flight. It reads the port from
`~/.config/sonder/server.json` (default 8797), never reads the pairing token,
and does not start another server or load the media catalog.

The menu shows running, scanning, updating, unreachable, or unexpected-response
status; item count; and links to the web interface and logs. Quitting the
indicator does not stop the launchd server. It has no Dock icon.

For automatic startup, add the built app in macOS System Settings → General →
Login Items. Automatic startup is intentionally not installed by the build.

Verification: compiled with Swift 6, checked plist syntax, and launched locally.
Status rendering and menu accessibility still benefit from manual UI review.
