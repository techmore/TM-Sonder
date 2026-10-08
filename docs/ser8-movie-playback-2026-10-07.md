# SER8 movie playback repair — 2026-10-07

The Ninth Gate mobile failure exposed two problems:

- Transcode requests reused stdout after its MP4 initialization atoms had already been consumed. Requests now restart the stream, and the distributor waits until the initial reader is attached.
- Ubuntu AppArmor stacking on SER8 prevented signaling processes inside Sonder. Kernel logs showed a signal denial to `incus-sonder_</var/lib/incus>//&unconfined`. A narrowly scoped instance rule permits signaling the same container’s stacked profiles, preserving the container boundary.

Applied instance configuration (`incus config get sonder raw.apparmor`):

```apparmor
signal (send, receive) peer="incus-sonder_</var/lib/incus>//&*",
```

A forced container restart applied the rule. Two consecutive public transcode requests subsequently returned HTTP 200 and began with `ftyp`, in 0.48 and 0.23 seconds. Previously, a reused request began with `moof`, and a subsequent restart request timed out because the encoder could not be killed.

The profile name is specific to this SER8 instance; adjust it if moving to another Incus installation. Do not disable AppArmor or make the container privileged.
