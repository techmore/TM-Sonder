package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStableAssetURLsRevalidateAcrossDeployments(t *testing.T) {
	response := httptest.NewRecorder()
	serveAsset(response, httptest.NewRequest("GET", "/library.js", nil), libraryJS, "text/javascript")
	if cache := response.Header().Get("Cache-Control"); !strings.Contains(cache, "max-age=0") || !strings.Contains(cache, "must-revalidate") {
		t.Fatalf("stale code can survive deployment: %s", cache)
	}
	request := httptest.NewRequest("GET", "/library.js", nil)
	request.Header.Set("If-None-Match", response.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	serveAsset(cached, request, libraryJS, "text/javascript")
	if cached.Code != 304 {
		t.Fatalf("conditional caching lost: %d", cached.Code)
	}
}

func TestVersionedAssetURLsAreImmutable(t *testing.T) {
	response := httptest.NewRecorder()
	serveAsset(response, httptest.NewRequest("GET", "/library.js?v=0123456789abcdef", nil), libraryJS, "text/javascript")
	cache := response.Header().Get("Cache-Control")
	if !strings.Contains(cache, "max-age=31536000") || !strings.Contains(cache, "immutable") {
		t.Fatalf("versioned asset is not immutable: %s", cache)
	}
}
