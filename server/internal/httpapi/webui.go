package httpapi

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// The web UI is ported from SonderWebInterface.swift (library browser +
// audiobook player). Pages are served verbatim; they hydrate from the same
// JSON routes the iOS client uses. When opened with ?token= (LAN pairing),
// the embedded JS propagates the token to every same-origin request.

//go:embed web/library.html web/audiobooks.html
var webFS embed.FS

func mustReadWeb(name string) []byte {
	b, err := webFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("httpapi: embedded %s missing: %v", name, err))
	}
	return b
}

var (
	libraryPage    = newGzippedPage(func() []byte { return mustReadWeb("web/library.html") })
	audiobooksPage = newGzippedPage(func() []byte { return mustReadWeb("web/audiobooks.html") })
	ebooksPage     = newGzippedPage(func() []byte { return mustReadWeb("web/ebooks.html") })
)

// gzippedPage caches an embedded page's raw and gzip-encoded bytes so each
// request writes pre-compressed output instead of recompressing.
type gzippedPage struct {
	once  sync.Once
	rawFn func() []byte
	raw   []byte
	gz    []byte
}

func newGzippedPage(raw func() []byte) *gzippedPage {
	return &gzippedPage{rawFn: raw}
}

func (p *gzippedPage) bytes() (raw, gz []byte) {
	p.once.Do(func() {
		p.raw = p.rawFn()
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(p.raw)
		_ = zw.Close()
		p.gz = buf.Bytes()
	})
	return p.raw, p.gz
}

// serveHTML writes a cached page, pre-gzipping when the client accepts it.
// It bypasses the withGzip middleware (those routes skip compression).
func serveGzippableHTML(w http.ResponseWriter, r *http.Request, page *gzippedPage) {
	raw, gz := page.bytes()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body := raw
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(gz)))
		body = gz
	} else {
		w.Header().Set("Content-Length", fmt.Sprint(len(raw)))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func serveHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
