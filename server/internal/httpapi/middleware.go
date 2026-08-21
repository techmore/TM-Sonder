package httpapi

import (
	"compress/gzip"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync"
)

// gzipWriter wraps a ResponseWriter, compressing writes. Content-Encoding is
// set lazily on first Write so bodiless responses (304) stay unadvertised.
type gzipWriter struct {
	http.ResponseWriter
	gz   *gzip.Writer
	once sync.Once
}

func (g *gzipWriter) prepare() {
	g.once.Do(func() {
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Del("Content-Length")
	})
}

func (g *gzipWriter) WriteHeader(code int) {
	switch {
	case code == http.StatusNoContent || code == http.StatusNotModified:
		// Bodiless: pass through uncompressed.
	default:
		g.prepare()
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	g.prepare()
	return g.gz.Write(p)
}

// Flush propagates to the underlying writer (used by streaming paths that
// bypass gzip anyway) and the compressor.
func (g *gzipWriter) Flush() {
	_ = g.gz.Flush()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

var gzPool = sync.Pool{
	New: func() any { return gzip.NewWriter(nil) },
}

func newGzipWriter(w http.ResponseWriter) *gzipWriter {
	gz := gzPool.Get().(*gzip.Writer)
	gz.Reset(w)
	return &gzipWriter{ResponseWriter: w, gz: gz}
}

func (g *gzipWriter) Close() {
	_ = g.gz.Close()
	gzPool.Put(g.gz)
}

// withLocalPprof mounts net/http/pprof only for loopback peers. It is
// registered on the same mux under /debug/pprof/*.
func withLocalPprof(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/debug/pprof") {
			next.ServeHTTP(w, r)
			return
		}
		if !isLoopback(peerHost(r)) {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
