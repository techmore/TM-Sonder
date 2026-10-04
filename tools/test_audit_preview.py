import http.client
import importlib.util
from pathlib import Path
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

spec = importlib.util.spec_from_file_location('audit_preview', Path(__file__).with_name('audit-preview.py'))
preview = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preview)


class PreviewSecurityTests(unittest.TestCase):
    def test_allowlist(self):
        for route in ('/audiobooks', '/library.js?v=asset', '/api/library', '/api/audiobooks/book-1/chapters', '/stream/book-1'):
            self.assertTrue(preview.allowed_request('GET', route), route)
        for route in ('/api/settings', '/api/data/export', '/api/network/rebind', '/api/optimization/audiobooks/jobs',
                      '/api/library?token=secret', '/stream/book-1?transcode=1', '/stream/book-1?unexpected=1',
                      '/../../etc/passwd', 'http://evil.test/api/library', '/api/library?api_key=secret'):
            self.assertFalse(preview.allowed_request('GET', route), route)
        for method in ('POST', 'PATCH', 'PUT', 'DELETE', 'CONNECT'):
            self.assertFalse(preview.allowed_request(method, '/api/library'))

    def test_http_boundaries(self):
        # No upstream configured: accepted mutation/foreign-host requests would fail this test.
        server = ThreadingHTTPServer(('127.0.0.1', 0), preview.PreviewHandler)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            for method, route, headers in (
                    ('POST', '/api/progress/book', {}), ('GET', '/api/settings', {}),
                    ('GET', '/library.js', {'Host': 'evil.test'}),
                    ('GET', '/library.js', {'Origin': 'https://evil.test'}),
                    ('GET', '/library.js', {'Sec-Fetch-Site': 'cross-site'})):
                connection = http.client.HTTPConnection('127.0.0.1', server.server_port)
                connection.request(method, route, headers=headers)
                response = connection.getresponse()
                self.assertEqual(response.status, 403)
                response.read()
                connection.close()
            connection = http.client.HTTPConnection('127.0.0.1', server.server_port)
            connection.request('GET', '/library.js')
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            self.assertEqual(response.getheader('Cache-Control'), 'no-store')
            self.assertIsNone(response.getheader('Set-Cookie'))
            response.read()
            connection.close()
        finally:
            server.shutdown()
            server.server_close()
            worker.join()

    def test_range_proxy_and_header_isolation(self):
        seen = []
        class Upstream(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass
            def do_GET(self):
                seen.append((self.path, dict(self.headers)))
                self.send_response(206)
                self.send_header('Content-Length', '4')
                self.send_header('Content-Range', 'bytes 2-5/8')
                self.send_header('Accept-Ranges', 'bytes')
                self.send_header('Set-Cookie', 'credential=do-not-forward')
                self.send_header('Location', '/?token=do-not-forward')
                self.end_headers()
                self.wfile.write(b'2345')
        upstream = ThreadingHTTPServer(('127.0.0.1', 0), Upstream)
        server = ThreadingHTTPServer(('127.0.0.1', 0), preview.PreviewHandler)
        server.token = 'private-audit-test-token'
        server.upstream_port = upstream.server_port
        workers = [threading.Thread(target=s.serve_forever, daemon=True) for s in (upstream, server)]
        for worker in workers:
            worker.start()
        try:
            connection = http.client.HTTPConnection('127.0.0.1', server.server_port)
            connection.request('GET', '/stream/book-1', headers={'Range': 'bytes=2-5', 'Cookie': 'browser-secret=never-upstream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 206)
            self.assertEqual(response.read(), b'2345')
            self.assertEqual(response.getheader('Content-Range'), 'bytes 2-5/8')
            self.assertIsNone(response.getheader('Set-Cookie'))
            self.assertIsNone(response.getheader('Location'))
            self.assertEqual(seen[0][1].get('Range'), 'bytes=2-5')
            self.assertEqual(seen[0][1].get('Authorization'), 'Bearer private-audit-test-token')
            self.assertNotIn('Cookie', seen[0][1])
            self.assertNotIn('token', seen[0][0])
            connection.close()
        finally:
            for service in (server, upstream):
                service.shutdown()
                service.server_close()
            for worker in workers:
                worker.join()


if __name__ == '__main__':
    unittest.main()
