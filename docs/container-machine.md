# The container machine

A container machine is a full Linux environment with systemd, whose Mac home
directory is mounted inside it. This document covers what it is for, what it
cannot do, and the two toolchain behaviours that will otherwise cost you an
afternoon.

## Why a machine and not the app container

There are two container targets in this repo and they are not redundant.

| | `make container-run` | `make machine-*` |
|---|---|---|
| what it is | one app in one container | a Linux environment with systemd |
| sees `/Volumes/14tb` | yes, mounted read-only | **no** |
| serves the real library | yes | no, it serves fixtures |
| runs the server as | `sonder` in a container | your host user, from a pidfile |
| good for | "does this work against real data?" | "build, test, and run the real binary" |

The machine is the one that matches production. `deploy/systemd/tm-sonder.service`
runs a static Linux binary from `%h/TM-Sonder/bin`, reading
`%h/.config/sonder/server.json`. The machine builds that same binary from the
same tree, in the same paths, and runs it with the same arguments. A bug found
here is a bug that would have been found on the server host.

The fixture media are not a compromise to work around a limitation so much as a
safety property: a machine cannot see the NAS, so it cannot mutate the real
library by accident while you are experimenting.

## Quick start

```bash
make machine-create     # build the image, create the machine, generate fixtures, start
make machine-status     # is it up, what does the API say
make machine-shell      # interactive shell; the repo is at ~/TM-Sonder
make machine-test       # gofmt, go vet, go test, and the web client suite
make machine-logs       # tail the server log
make machine-stop       # stop the machine (storage is kept)
make machine-destroy    # delete it and its storage
```

Then open <http://localhost:8096>.

## Two things that will waste your time

### 1. `container machine run` word-splits every argument

It does not honour quoting. A quoted argument containing spaces arrives as
several arguments:

```bash
# on the Mac
container machine run -n sonder -- /bin/bash args.sh "one two" three
# inside: arg1=[one] arg2=[two] arg3=[three]
```

This is why an inline `sh -c` or `bash -lc` appears to succeed and does nothing:

```bash
container machine run -n sonder -- sh -c 'echo hello'
# sh receives: -c "echo"   with "hello" as $0
# so it runs the command `echo`, which prints nothing
```

The failure is silent, which is the dangerous part. It looks like the command
produced no output rather than like a broken invocation.

**The rule: pass a script file by absolute path, never an inline `-c` command.**

```bash
# good
container machine run -n sonder -- /bin/bash "$PWD/tools/machine/inside-setup.sh"

# silently does nothing
container machine run -n sonder -- /bin/bash -lc 'cd ~/x && make test'
```

Every helper in `tools/machine/inside-*.sh` exists for this reason. They are run
by path, and they take no arguments that contain spaces.

### 2. A container machine has no systemd *user* session

systemd itself runs, but there is no session for your uid: `/run/user/501` does
not exist, `loginctl enable-linger` answers "No sessions", and `machinectl` is
absent because `machined` is not running. So:

```bash
systemctl --user status tm-sonder    # Failed to connect to bus
```

The server is therefore supervised by a pidfile
(`~/.config/sonder/sonder.pid`) and started with `setsid nohup` so it outlives the
`container machine run` session that launched it.

This is a real gap against the deployment, and worth being clear about it: what
is reproduced here is the binary, the config layout, the paths and the command
line. The **supervisor is not**. Restart-on-failure, the systemd sandbox and
`systemctl --user` behaviour are only exercised on the server host.

## The home directory is not where the docs say

The Apple docs state the Mac home directory is mounted at `/home/<you>`. On
container 1.0.0 it is mounted at its **macOS** path, `/Users/<you>`, and `$HOME`
inside the machine is an ordinary Linux home that does not contain the
repository.

Rather than depend on either, `tools/machine/inside-link.sh` finds the repository
and links it to `$HOME/TM-Sonder`. Everything else refers to that, so the setup
works on both layouts.

## The image

`deploy/machine/Containerfile.machine`. Ubuntu 24.04, `ENV container container`,
systemd as init, plus Go (pinned to a version, from the tarball), ffmpeg, node,
python3, jq and git.

Two lines in it are load-bearing and are easy to lose in a refactor:

```dockerfile
ENV container container          # marks the image as a machine image
RUN ln -sf /lib/systemd/systemd /sbin/init
```

The second is the difference between a machine that boots and one that does not.
Without it, `container machine create` fails with:

```
failed to start process ... vmexec error: ... "No such process"
```

which reads like a virtualisation fault and is not one — `ubuntu:24.04` ships no
`/sbin/init` for the machine to exec.

One deviation from Apple's reference Containerfile: it runs
`systemctl disable networkd-dispatcher.service`, which exits non-zero because
plain `ubuntu:24.04` has no such unit, failing the build. Masking is the
equivalent no-op.

## Fixtures

`tools/machine/make-fixtures.sh` generates real, decodable media with ffmpeg —
a few seconds per file, so the whole tree is under a megabyte. It covers all four
kinds, and deliberately includes a **three-file book**, because "one card per
book, plays through in order" cannot be tested without a book made of several
files. That is the behaviour most likely to regress.

```bash
make machine-fixtures                        # regenerate
container machine run -n sonder -- ls -R ~/TM-Sonder/media/fixtures
```

## Deploying what you built

The binary built in the machine is the artifact that ships — a static Linux
binary, same toolchain as the release build:

```bash
container machine run -n sonder -- /bin/bash ~/TM-Sonder/tools/machine/inside-build.sh
scp sonder-linux-arm64 sdolbec@100.127.99.74:...
```

That is the point of the machine rather than cross-compiling on the Mac: the
thing that passed the tests is the thing that gets deployed.
