# Sonder Federation Feature

## Goal

Allow users to manually connect trusted Sonder servers so libraries can be browsed, streamed, and eventually synced without manually copying media between machines.

This feature should remain App Store-safe by avoiding automatic LAN probing or unprompted network discovery. Federation should be explicitly configured by the user.

## User Model

In Settings > Federation, a user can add another Sonder server by URL.

Each federated server should track:

- Display name
- Base URL
- Status: online, offline, degraded, unauthorized
- Last seen time
- Pairing token or auth credential
- Sync policy
- Last catalog refresh

Servers may be unavailable because a Mac is asleep, a VPN is disconnected, a URL changed, or a remote network is temporarily down. The app should tolerate this without blocking local library usage.

## Status Behavior

Sonder should periodically check:

- `/api/health`
- `/api/library`

If a server is unavailable:

- Do not show its media as locally playable.
- Either hide remote-only content or mark it unavailable, depending on user preference.
- Keep the last known catalog for reference.
- Retry with backoff rather than aggressive polling.

## Source States

Remote media should have explicit availability state:

- `local`
- `remoteAvailable`
- `remoteUnavailable`
- `syncing`
- `syncFailed`

This prevents unavailable remote media from polluting the local library as playable content.

## Sync Modes

Initial modes:

- `Catalog only`: show remote library and stream from remote when online.
- `Manual download`: user chooses titles to copy locally.
- `Auto sync`: selected libraries, playlists, shows, or seasons sync to local storage when available.
- `Watch progress sync`: trusted servers share resume positions.

Full media sync should come after local streaming, progress, and library organization are stable.

## App Store Safety

Preferred constraints:

- User manually adds server URLs.
- No automatic port scanning.
- No automatic LAN crawling.
- Bonjour discovery only if user explicitly enables LAN sharing/discovery.
- Require pairing/auth for remote APIs.
- Prefer HTTPS for remote/VPN connections.
- Allow local/VPN HTTP with clear labeling.
- Store synced media only in Application Support or user-selected folders.

## Suggested Implementation Order

1. Add data model for federated servers.
2. Add Settings > Federation UI.
3. Add health checks and status display.
4. Add remote catalog fetch and cached remote library state.
5. Mark remote media availability in the UI.
6. Add remote streaming when online.
7. Add manual download.
8. Add watch progress sync.
9. Add selective auto sync.

## Non-Goals For First Pass

- No automatic media sync.
- No internet-wide sharing.
- No public server directory.
- No unauthenticated remote APIs.
- No automatic network scanning.
