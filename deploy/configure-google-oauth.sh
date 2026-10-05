#!/usr/bin/env bash
# Copy the approved Google OAuth client from the Homeboard proxy into Sonder's
# protected systemd environment file. Credentials are passed over pipes and are
# never written to the runner log or repository.
set -euo pipefail

INSTANCE="${SONDER_INCUS_INSTANCE:-sonder}"
SOURCE_INSTANCE="${SONDER_OAUTH_SOURCE_INSTANCE:-homeboard}"

python3 - "$SOURCE_INSTANCE" "$INSTANCE" <<'PY'
import json
import subprocess
import sys

source, target = sys.argv[1:]
read_profile = r'''import json, pathlib, re
config = pathlib.Path("/etc/homeboard/oauth2-proxy.cfg").read_text()
def value(name):
    match = re.search(r"(?m)^\s*" + re.escape(name) + r"\s*=\s*\"([^\"]+)\"", config)
    if not match:
        raise SystemExit("required OAuth setting is missing")
    return match.group(1)
client_id = value("client_id")
secret_path = pathlib.Path(value("client_secret_file"))
client_secret = secret_path.read_text().strip()
if not client_id or not client_secret:
    raise SystemExit("Google OAuth client is incomplete")
print(json.dumps({"client_id": client_id, "client_secret": client_secret}))
'''
profile = subprocess.run(
    ["incus", "exec", source, "--", "python3", "-c", read_profile],
    check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
).stdout
values = json.loads(profile)
if not values.get("client_id") or not values.get("client_secret"):
    raise SystemExit("Google OAuth profile is incomplete")
payload = json.dumps(values).encode()
write_profile = r'''import json, os, pathlib, pwd, sys
values = json.load(sys.stdin)
directory = pathlib.Path("/etc/sonder")
directory.mkdir(mode=0o755, parents=True, exist_ok=True)
env = "SONDER_GOOGLE_CLIENT_ID=" + json.dumps(values["client_id"]) + "\n" + "SONDER_GOOGLE_CLIENT_SECRET=" + json.dumps(values["client_secret"]) + "\n"
path = directory / "google-oauth.env"
path.write_text(env)
os.chmod(path, 0o640)
os.chown(path, 0, pwd.getpwnam("ubuntu").pw_gid)
dropin = pathlib.Path("/etc/systemd/system/sonder.service.d")
dropin.mkdir(mode=0o755, parents=True, exist_ok=True)
override = dropin / "google-oauth.conf"
override.write_text("[Service]\nEnvironmentFile=/etc/sonder/google-oauth.env\n")
os.chmod(override, 0o644)
'''
subprocess.run(
    ["incus", "exec", target, "--", "python3", "-c", write_profile],
    input=payload, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
)
subprocess.run(["incus", "exec", target, "--", "systemctl", "daemon-reload"], check=True, stdout=subprocess.DEVNULL)
print("Google OAuth profile installed for Sonder (credential values hidden).")
PY
