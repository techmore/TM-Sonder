package httpapi

import (
	"io"
	"net/http"
)

// withSecurityHeaders sets conservative response headers on every reply.
// There is deliberately no script-src policy: the embedded pages use inline
// scripts, and a CSP that allowed them would add little. frame-ancestors and
// X-Frame-Options stop clickjacking of the sign-in and admin pages; nosniff
// stops user-supplied media metadata from being sniffed as HTML.
//
// The headers are applied twice: once up front so handlers that inspect the
// header map see them, and again when the status line is written. The second
// pass matters on the LAN web listener, which reverse-proxies to the private
// API listener: both hops run this middleware, and the proxy copies the
// upstream reply's headers with Add, so every browser response used to carry
// each security header twice. Set at write time replaces the copies.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &securityHeaderWriter{ResponseWriter: w, secure: requestIsSecure(r)}
		sw.apply()
		next.ServeHTTP(sw, r)
	})
}

type securityHeaderWriter struct {
	http.ResponseWriter
	secure bool
	wrote  bool
}

func (w *securityHeaderWriter) apply() {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Content-Security-Policy", "frame-ancestors 'self'")
	h.Set("Referrer-Policy", "same-origin")
	if w.secure {
		h.Set("Strict-Transport-Security", "max-age=15552000")
	}
}

func (w *securityHeaderWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		w.apply()
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *securityHeaderWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// ReadFrom keeps sendfile available to http.ServeContent for direct-play
// streams; embedding the interface alone would hide it.
func (w *securityHeaderWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(w.ResponseWriter, r)
}

// Flush forwards to the underlying Flusher so HLS and progress streams keep
// streaming through the wrapper.
func (w *securityHeaderWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *securityHeaderWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
