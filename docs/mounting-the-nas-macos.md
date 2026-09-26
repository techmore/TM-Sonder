# Mounting the NAS on macOS

The library lives on a Synology at `192.168.222.188`, exported at
`/volume2/14tb`. It is reached through `~/NAS`, a symlink to `/Volumes/14tb`.

## Why `mount` keeps failing

```console
$ sudo mount -t nfs -o resvport 192.168.222.188:/volume2/14tb ~/NAS
mount: /Users/seandolbec/NAS: invalid file system.
```

`invalid file system` is `EINVAL` from `mount(2)`, and it is a message about the
*mount point*, not about NFS. Two separate mistakes produce it:

1. **`~/NAS` is a symlink.** `mount` cannot mount onto a symlink; it resolves
   the path first and then mounts on the result.
2. **`/Volumes/14tb` did not exist.** macOS creates a directory under `/Volumes`
   when a mount succeeds, but it is not reliably recreated after a reboot or a
   clean unmount, so a later mount resolves to a missing directory.

`mount: /Volumes/14tb: invalid file system.` is the same error from the other
direction — the NAS is fine, the target is not.

## The working invocation

```bash
sudo mkdir -p /Volumes/14tb
sudo mount -t nfs -o vers=3,resvport,proto=tcp 192.168.222.188:/volume2/14tb /Volumes/14tb
```

All three options matter:

| option | why |
| --- | --- |
| `vers=3` | the export is **NFSv3 only**; the NFSv4 path does not match it |
| `resvport` | ask for a privileged source port, which NFSv3 over UDP needs |
| `proto=tcp` | some NAS firmware refuses NFSv3 over UDP on a wired LAN |

Verify:

```bash
mount | grep nfs
df -h /Volumes/14tb
ls ~/NAS/plex/Audiobooks | head
```

A `not responding` / `is alive again` pair on first access is normal on a hard
mount and resolves itself.

## Making it survive a reboot

`/etc/auto_master` mounts on **first access** rather than at boot, which is what
the `~/NAS` symlink wants: the mount point appears exactly when something reads
through it, and the server sees a normal directory.

```bash
sudo cp /etc/auto_master /etc/auto_master.bak
sudo sh -c 'cat >> /etc/auto_master' <<'EOF'
/Volumes/14tb  -fstype=nfs,vers=3,resvport,proto=tcp  192.168.222.188:/volume2/14tb
EOF
sudo automount -vc          # arm it now; it also runs at boot
```

Keep `auto_master`'s existing `/-  -static` line: that is what tells autofs to
consult the file at all. Verify with:

```bash
grep 14tb /etc/auto_master
sudo automount -vc           # should print the new map
```

If the NAS is powered off, the path stays an empty "ghost" directory rather than
disappearing, which is the behaviour the server wants — see below.

## Why this matters to TM-Sonder

`safeScan: true` is set in `~/.config/sonder/server.json`, and it is what stops
an unmounted NAS from looking like a deleted library. Without it, a scan against
an unreadable root would prune every item; with it, a library that resolves but
reads empty is left alone. A full catalog rebuild of this NAS takes about 19
minutes, so losing the catalog to a stray unmount would be expensive.

To re-scan after a remount:

```bash
python3 tools/audiobooks/rescan.py          # waits, bounded, logs the result
```

which drives the server's existing `POST /api/settings/rescan`. A restart also
scans, but the endpoint avoids taking the service down.

## Two ways this goes wrong

**`~/NAS` is not a directory.** Anything that `os.Stat`s it and expects a
directory will misbehave while the share is down. Reading *through* it is fine.

**The Homebrew service is not the real instance.** `brew services start
local/tm-sonder/tm-sonder` runs `/opt/homebrew/etc/sonder/server.json`, which
the formula only ever fills with its template. It binds the same ports, reports
healthy, and shows an empty catalog. The real instance is the
`com.sonder.server` LaunchAgent on `~/.config/sonder/server.json`. See
`docs/running-the-server-macos.md`.
