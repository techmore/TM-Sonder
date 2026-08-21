package httpapi

import (
	"embed"
	"net/http"
)

// The web UI is ported from SonderWebInterface.swift (library browser +
// audiobook player). Pages are served verbatim; they hydrate from the same
// JSON routes the iOS client uses. When opened with ?token= (LAN pairing),
// the embedded JS propagates the token to every same-origin request.

//go:embed web/library.html web/audiobooks.html
var webFS embed.FS

func libraryHTML() []byte {
	b, _ := webFS.ReadFile("web/library.html")
	return b
}

func audiobooksHTML() []byte {
	b, _ := webFS.ReadFile("web/audiobooks.html")
	return b
}

func serveHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
