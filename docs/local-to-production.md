# Local development → main → the server

The path from editing a file on your Mac to a running server, and the one thing
you have to do by hand.

```
 container machine (Mac)          GitHub                    server host
 ───────────────────────          ──────                    ───────────
 edit + build + test
   make machine-test
      │
      ├─ push branch ────────────► CI on every branch
      │                            gofmt, vet, go test,
      │                            web tests, all 3 targets
      │
      ├─ open a PR ──────────────► CI again (required check)
      │
      └─ merge to main ──────────► Deploy workflow
                                    build + verify (hosted)
                                             │
                                             ▼
                                    self-hosted runner ON the server
                                    swap binary, restart, health-check,
                                    roll back on any failure
```

## The one manual step: register the runner

The deploy job runs on a **self-hosted runner on the server**, not on GitHub's
infrastructure. That is not a preference:

- The server is behind a NAT. Only 80, 443 and 8096 are forwarded, and
  administration happens over Tailscale (`100.x`). A GitHub-hosted runner cannot
  open an SSH connection to it.
- The alternative is exposing port 22 to the internet to save one `scp`. That is
  a bad trade for a media server, and this is not a trade worth making quietly.
- A self-hosted runner needs no inbound access at all. GitHub reaches it over an
  outbound HTTPS connection, and it runs where the binary and the systemd user
  unit already are. No deploy key material sits in GitHub secrets.

Registering it needs a token from the repository's
**Settings → Actions → Runners → New self-hosted runner**, which is an admin
action. On the server:

```bash
mkdir -p ~/actions-runner && cd ~/actions-runner

# 1. Download the runner (x64, since the server is x86_64)
curl -sSL -o runner.tar.gz \
  https://github.com/actions/runner/releases/download/v2.328.0/actions-runner-linux-x64-2.328.0.tar.gz
tar xzf runner.tar.gz

# 2. Configure it. Paste the token from the repo settings page.
./config.sh --url https://github.com/techmore/TM-Sonder \
            --token <TOKEN> \
            --name tm-sonder \
            --labels tm-sonder,linux,x64 \
            --work _work \
            --no-default-labels

# 3. Run it as a service so it survives reboots and logout
sudo ./svc.sh install tm-sonder
sudo ./svc.sh start tm-sonder
systemctl status actions.runner.tm-sonder
```

The runner will pick up jobs labelled `tm-sonder`. Until it is registered, the
`deploy` job simply sits in the queue — `build` still runs, so merges are never
blocked by a missing runner.

Check it is connected from the repository's Actions page, or:

```bash
curl -sS -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/repos/techmore/TM-Sonder/actions/runners | grep -o '"name": *"[^"]*"'
```

## The everyday loop

```bash
make machine-create        # once: build the image, create the machine, start the server
                           # http://localhost:8096
make machine-test          # gofmt, vet, all Go packages, the web client suite
```

Edit on the Mac, in your editor. The repository is mounted into the machine, so
there is no copy step and no sync to forget. Run the tests in the machine, which
is the same Linux environment the release binary is built in.

Then:

```bash
git switch -c fix/whatever
# ... edit, make machine-test ...
git push -u origin fix/whatever
gh pr create --fill
```

CI runs on **every** branch now. It used to run on `[main, master, go-port,
fix/**, codex/**]`, which meant work on other branches — including everything on
`dev-container` — was never built or tested, and a green `main` said nothing
about whether those branches compiled.

Merge, and the deploy runs.

## What the deploy actually does

`deploy/deploy-sonder.sh`, executed on the server:

1. **Refuses the wrong architecture.** It reads the ELF header directly and
   rejects anything that is not x86-64. Without this, a stray arm64 build fails
   at exec time with a message that reads like a permissions problem.
2. **Backs up the running binary** to a dated file, never overwriting an earlier
   backup.
3. **Swaps and restarts** the systemd user unit.
4. **Health-checks the version the API reports**, not merely that a process is
   listening. A process can be up and still serving the previous build; if the
   reported version is not the one that was installed, it rolls back.
5. **Rolls back on any failure** — restart refused, no answer from the API, or a
   version mismatch — and exits non-zero so the workflow goes red.

All of that is covered by `deploy/deploy_sonder_test.go`, which drives the script
against a stub `systemctl`, a stub `curl` and stand-in ELF headers. Two bugs
were found that way: a backup name that collided within the same second and
silently overwrote the previous revision, and a `stat -c` that only works with
GNU coreutils, which meant the script could not be tested off the server at all.

```bash
cd deploy && go test ./...
```

## Rolling back by hand

Every deploy leaves its predecessor behind:

```bash
ls -lt ~/TM-Sonder/bin/sonder-linux-amd64.bak-*
cp -p ~/TM-Sonder/bin/sonder-linux-amd64.bak-<timestamp> ~/TM-Sonder/bin/sonder-linux-amd64
systemctl --user restart tm-sonder
```

## Deploying by hand, without the runner

The runner is a convenience, not a dependency. The original path still works and
is what to use until the runner is registered:

```bash
make machine-build-ship                       # builds all three targets in the machine
scp ~/Projects/TM-Sonder/bin/sonder-linux-amd64 sdolbec@100.127.99.74:~/TM-Sonder/bin/.sonder-linux-amd64.incoming
ssh sdolbec@100.127.99.74 'cp -p ~/TM-Sonder/bin/sonder-linux-amd64 ~/TM-Sonder/bin/sonder-linux-amd64.bak-manual && mv -f ~/TM-Sonder/bin/.sonder-linux-amd64.incoming ~/TM-Sonder/bin/sonder-linux-amd64 && systemctl --user restart tm-sonder'
```

Or let the script do it, which gives you the architecture check and the health
check as well:

```bash
scp ~/Projects/TM-Sonder/bin/sonder-linux-amd64 sdolbec@100.127.99.74:~/TM-Sonder/bin/.sonder-linux-amd64.incoming
ssh sdolbec@100.127.99.74 '~/TM-Sonder/deploy/deploy-sonder.sh'
```

## The two container targets

| | `make container-run` | `make machine-*` |
|---|---|---|
| what it is | one app in one container | a Linux environment |
| sees `/Volumes/14tb` | yes, read-only | no |
| serves the real library | yes | no, generated fixtures |
| good for | "does this work against real data?" | "build, test, run the real binary" |

See [container-machine.md](container-machine.md) for the machine's two silent
failure modes, which will otherwise cost you an afternoon.
