# TM Sonder

Personal media library stack: a **macOS host server** and an **iOS client**.

## Layout

| Path | Role |
| --- | --- |
| `xcode-TM-Sonder/` | Mac app (library manager + HTTP media server) |
| `IOS_Client_Xcode/` | iOS client |
| `Packages/SonderAPI/` | Shared wire DTOs, routes, and discovery models |
| `API.md` | HTTP contract for clients |

## Run the Mac server

1. Open `xcode-TM-Sonder.xcodeproj` in Xcode.
2. Build and run **xcode-TM-Sonder**.
3. Add media folders, scan, and enable **LAN** in Server settings when you want devices on the network to connect (a pairing token is generated automatically).

Default port: `8797`. Local-only until LAN is enabled.

## Run the iOS client

1. Open `IOS_Client_Xcode/TM_Sonder_Client/TM_Sonder_Client.xcodeproj`.
2. Build and run on a device or simulator on the same network (or via Tailscale).
3. Pair with the server URL and token from the Mac Server dashboard.

## Shared API package

Both apps depend on the local Swift package `Packages/SonderAPI`. Change wire shapes there first, then update host mapping / client usage. Package unit tests:

```bash
cd Packages/SonderAPI && swift test
```

## Tests

```bash
# Shared package
cd Packages/SonderAPI && swift test

# Mac host unit tests
xcodebuild test -project xcode-TM-Sonder.xcodeproj -scheme xcode-TM-Sonder \
  -destination 'platform=macOS' -only-testing:xcode-TM-SonderTests
```

CI runs both on GitHub Actions (`.github/workflows/ci.yml`).

## Delegate from OpenCode to Pi

This repo includes an OpenCode MCP entry for Pi. Start OpenCode from the repo
root, then ask it to use the `pi_delegate` tool for an independent coding or
review pass. The bridge starts a fresh, non-persistent Pi RPC session in the
current project for each call.

Set `PI_BIN` if `pi` is not on OpenCode's `PATH`:

```bash
PI_BIN="$HOME/.local/bin/pi" opencode
```
