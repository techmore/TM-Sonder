package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
)

// The web UI is ported from SonderWebInterface.swift (library browser +
// audiobook player). Pages are served verbatim; they hydrate from the same
// JSON routes the iOS client uses. When opened with ?token= (LAN pairing),
// the embedded JS propagates the token to every same-origin request.

//go:embed web/library.html web/library.css web/library.js web/audiobooks.html web/ebooks.html web/shared.js web/favicon.svg web/favicon.png
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
	libraryCSS     = newGzippedPage(func() []byte { return mustReadWeb("web/library.css") })
	libraryJS      = newGzippedPage(func() []byte { return mustReadWeb("web/library.js") })
	audiobooksPage = newGzippedPage(func() []byte { return mustReadWeb("web/audiobooks.html") })
	ebooksPage     = newGzippedPage(func() []byte { return mustReadWeb("web/ebooks.html") })
	sharedJS       = newGzippedPage(func() []byte { return mustReadWeb("web/shared.js") })
	faviconSVG     = newGzippedPage(func() []byte { return mustReadWeb("web/favicon.svg") })
	faviconPNG     = newGzippedPage(func() []byte { return mustReadWeb("web/favicon.png") })
)

// serveAsset writes an embedded, pre-gzipped asset with ETag/304 support.
func serveAsset(w http.ResponseWriter, r *http.Request, page *gzippedPage, contentType string) {
	raw, gz, etag := page.bytes()
	w.Header().Set("Content-Type", contentType)
	// Assets have stable URLs, not content-hashed filenames. Revalidate on
	// reload so a restart cannot pair new catalog data with stale UI code.
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("ETag", etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := raw
	if acceptsGzip(r) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(gz)))
		body = gz
	} else {
		w.Header().Set("Content-Length", fmt.Sprint(len(raw)))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// handleSharedJS serves the shared front-end helpers used by every page.
func (s *Server) handleSharedJS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, sharedJS, "text/javascript; charset=utf-8")
}

// handleLibraryCSS and handleLibraryJS serve the library browser's own assets,
// extracted from library.html so the page, styles, and behaviour evolve
// independently.
func (s *Server) handleLibraryCSS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, libraryCSS, "text/css; charset=utf-8")
}

func (s *Server) handleLibraryJS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, libraryJS, "text/javascript; charset=utf-8")
}

// handleFavicon serves the shared TM Sonder brand mark used by the web UI.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, faviconSVG, "image/svg+xml")
}

// handleFaviconPNG serves the generated Sonder mark for clients that prefer a
// raster favicon or app-touch icon.
func (s *Server) handleFaviconPNG(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, faviconPNG, "image/png")
}

// gzippedPage caches an embedded page's raw and gzip-encoded bytes so each
// request writes pre-compressed output instead of recompressing.
type gzippedPage struct {
	once  sync.Once
	rawFn func() []byte
	raw   []byte
	gz    []byte
	etag  string
}

func newGzippedPage(raw func() []byte) *gzippedPage {
	return &gzippedPage{rawFn: raw}
}

func (p *gzippedPage) bytes() (raw, gz []byte, etag string) {
	p.once.Do(func() {
		p.raw = p.rawFn()
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(p.raw)
		_ = zw.Close()
		p.gz = buf.Bytes()
		sum := sha256.Sum256(p.raw)
		p.etag = `"` + hex.EncodeToString(sum[:8]) + `"`
	})
	return p.raw, p.gz, p.etag
}

// serveHTML writes a cached page, pre-gzipping when the client accepts it.
// It bypasses the withGzip middleware (those routes skip compression).
func serveGzippableHTML(w http.ResponseWriter, r *http.Request, page *gzippedPage) {
	raw, gz, etag := page.bytes()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := raw
	if acceptsGzip(r) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(gz)))
		body = gz
	} else {
		w.Header().Set("Content-Length", fmt.Sprint(len(raw)))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
