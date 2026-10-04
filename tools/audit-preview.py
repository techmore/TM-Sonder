#!/usr/bin/env python3
"""Loopback-only, read-only web preview using production data over SSH."""
import argparse
import http.client
import json
import mimetypes
from pathlib import Path
import re
import socket
import subprocess
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qsl, urlsplit

ROOT = Path(__file__).resolve().parents[1] / 'server/internal/httpapi/web'
STATIC = {'/': 'library.html', '/audiobooks': 'library.html',
          '/audiobooks-classic': 'audiobooks.html', '/audiobooks-beta': 'audiobooks-beta.html',
          '/ebooks': 'ebooks.html', **{'/' + n: n for n in (
              'library.css', 'library.js', 'shared.js', 'favicon.svg', 'favicon.png')}}
READ_ROUTES = re.compile(r'^(?:/api/(?:library|audiobooks|reading|lists|auth/session|cache/status|status)|'
                         r'/api/audiobooks/[A-Za-z0-9_-]+(?:/chapters)?|'
                         r'/api/playback/[A-Za-z0-9_-]+|/artwork/(?:poster|backdrop|curated)/[A-Za-z0-9_-]+|'
                         r'/stream/[A-Za-z0-9_-]+)$')
QUERY_KEYS = {'v', 'width', 'height', 'w', 'h', 'kind', 'limit', 'offset', 'search'}
RESPONSE_HEADERS = {'content-type', 'content-length', 'content-range', 'accept-ranges',
                    'etag', 'last-modified', 'content-disposition'}
MAX_METADATA = 64 * 1024 * 1024


def allowed_request(method, target):
    url = urlsplit(target)
    if method not in ('GET', 'HEAD') or url.scheme or url.netloc or url.fragment:
        return False
    if url.path not in STATIC and not READ_ROUTES.fullmatch(url.path):
        return False
    return all(k in QUERY_KEYS for k, _ in parse_qsl(url.query, keep_blank_values=True))


class PreviewHandler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.0'

    def log_message(self, *_):
        pass  # Neither private catalog paths nor credentials enter access logs.

    def do_GET(self):
        self.serve()

    do_HEAD = do_GET

    def deny(self):
        self.send_error(403, 'Read-only audit preview: request blocked')

    do_POST = do_PUT = do_PATCH = do_DELETE = do_OPTIONS = do_CONNECT = do_TRACE = deny

    def serve(self):
        expected = f'127.0.0.1:{self.server.server_port}'
        host = self.headers.get('Host', '')
        origin = self.headers.get('Origin')
        if host not in (expected, f'localhost:{self.server.server_port}') or (
                origin and origin not in (f'http://{expected}', f'http://localhost:{self.server.server_port}')):
            self.deny()
            return
        # Allow opening a static preview page from another tab, but never
        # cross-site API/media subrequests. No credential is exposed in HTML.
        top_level_asset = urlsplit(self.path).path in STATIC and self.headers.get('Sec-Fetch-Mode') == 'navigate'
        if self.headers.get('Sec-Fetch-Site') in ('cross-site', 'same-site') and not top_level_asset:
            self.deny()
            return
        if not allowed_request(self.command, self.path):
            self.deny()
            return
        path = urlsplit(self.path).path
        if path in STATIC:
            data = (ROOT / STATIC[path]).read_bytes()
            self.send_response(200)
            self.send_header('Content-Type', mimetypes.guess_type(STATIC[path])[0] or 'application/octet-stream')
            self.send_header('Content-Length', str(len(data)))
            self.send_header('Cache-Control', 'no-store')
            self.send_header('X-Content-Type-Options', 'nosniff')
            self.end_headers()
            if self.command != 'HEAD':
                self.wfile.write(data)
            return
        connection = http.client.HTTPConnection('127.0.0.1', self.server.upstream_port, timeout=30)
        headers = {'Authorization': 'Bearer ' + self.server.token, 'Accept-Encoding': 'identity'}
        for name in ('Range', 'If-Range', 'If-None-Match', 'If-Modified-Since', 'Accept'):
            if self.headers.get(name):
                headers[name] = self.headers[name]
        try:
            connection.request(self.command, self.path, headers=headers)
            response = connection.getresponse()
            # Never follow a redirect with the credential or return upstream login URLs.
            if 300 <= response.status < 400 and response.status != 304:
                self.send_error(502, 'Upstream redirected; verify audit connection')
                return
            media = path.startswith('/stream/')
            length = response.getheader('Content-Length')
            if not media and length and int(length) > MAX_METADATA:
                self.send_error(502, 'Metadata response exceeds audit limit')
                return
            self.send_response(response.status)
            for key, value in response.getheaders():
                if key.lower() in RESPONSE_HEADERS:
                    self.send_header(key, value)
            self.send_header('Cache-Control', 'no-store')
            self.send_header('X-Content-Type-Options', 'nosniff')
            self.end_headers()
            if self.command != 'HEAD':
                remaining = None if media else MAX_METADATA
                while remaining is None or remaining > 0:
                    chunk = response.read(64 * 1024 if remaining is None else min(64 * 1024, remaining))
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    if remaining is not None:
                        remaining -= len(chunk)
        except (OSError, ValueError, http.client.HTTPException):
            # No exception strings: subprocess/network messages can contain sensitive data.
            self.close_connection = True
        finally:
            connection.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--port', type=int, default=8768)
    parser.add_argument('--ssh-host', default='sdolbec@100.127.99.74')
    parser.add_argument('--container', default='sonder')
    parser.add_argument('--container-address', default='10.96.131.52')
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9_-]+', args.container):
        parser.error('Invalid container name')
    ssh = ['ssh', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', args.ssh_host]
    # Read only the credential and web port into memory. Never print the result.
    remote = (f"incus exec {args.container} -- python3 -c 'import json; "
              'c=json.load(open("/etc/sonder/server.json")); '
              'print(json.dumps({"token":c["pairingToken"],"port":c.get("webPort",8096)}))' + "'")
    try:
        config = json.loads(subprocess.check_output(ssh + [remote], stderr=subprocess.DEVNULL, timeout=20))
        if not config.get('token'):
            raise ValueError()
    except (OSError, ValueError, subprocess.SubprocessError):
        raise SystemExit('Could not read existing audit credential over SSH') from None
    with socket.socket() as reservation:
        reservation.bind(('127.0.0.1', 0))
        upstream = reservation.getsockname()[1]
    tunnel = subprocess.Popen(ssh[:-1] + ['-o', 'ExitOnForwardFailure=yes', '-N', '-L',
                              f'127.0.0.1:{upstream}:{args.container_address}:{int(config["port"])}', args.ssh_host],
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        for _ in range(50):
            if tunnel.poll() is not None:
                raise SystemExit('SSH audit tunnel failed')
            try:
                with socket.create_connection(('127.0.0.1', upstream), timeout=.2):
                    break
            except OSError:
                time.sleep(.1)
        else:
            raise SystemExit('SSH audit tunnel timed out')
        with ThreadingHTTPServer(('127.0.0.1', args.port), PreviewHandler) as server:
            server.token = config['token']
            server.upstream_port = upstream
            print(f'Read-only audit preview: http://127.0.0.1:{args.port}/audiobooks', flush=True)
            server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        tunnel.terminate()
        try:
            tunnel.wait(timeout=5)
        except subprocess.TimeoutExpired:
            tunnel.kill()
            tunnel.wait()


if __name__ == '__main__':
    main()
