package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tm-sonder/server/internal/config"
)

func TestAccountSetupLoginAndCompatibilitySessions(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) {
		cfg.AllowLAN = true
		cfg.PairingToken = "pair-me"
	})

	setup := httptest.NewRequest(http.MethodGet, "/account/setup?token=pair-me&next=%2F", nil)
	setup.RemoteAddr = "192.168.3.50:50123"
	setup.Host = "sonder.example:8096"
	setup.Header.Set("Accept", "text/html")
	setupRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(setupRec, setup)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup page status = %d", setupRec.Code)
	}
	if body := setupRec.Body.String(); !strings.Contains(body, `/api/auth/setup?token=pair-me`) {
		t.Fatalf("setup form did not preserve one-time token: %s", body)
	}

	form := url.Values{
		"username": {"owner"},
		"password": {"a-long-test-password"},
		"next":     {"/"},
	}
	create := httptest.NewRequest(http.MethodPost, "/api/auth/setup?token=pair-me", strings.NewReader(form.Encode()))
	create.RemoteAddr = setup.RemoteAddr
	create.Host = setup.Host
	create.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(createRec, create)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("setup submit status = %d, body=%s", createRec.Code, createRec.Body.String())
	}
	cookie := createRec.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != sessionCookieName || cookie[0].Value == "" {
		t.Fatalf("setup did not issue session cookie: %#v", cookie)
	}

	private := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	private.RemoteAddr = setup.RemoteAddr
	private.Host = setup.Host
	private.AddCookie(cookie[0])
	privateRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(privateRec, private)
	if privateRec.Code != http.StatusOK {
		t.Fatalf("session-authenticated health status = %d", privateRec.Code)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"owner","password":"a-long-test-password"}`))
	login.RemoteAddr = setup.RemoteAddr
	login.Host = setup.Host
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("JSON login status = %d, body=%s", loginRec.Code, loginRec.Body.String())
	}

	jellyfin := httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName", strings.NewReader(`{"Username":"owner","Pw":"a-long-test-password"}`))
	jellyfin.RemoteAddr = setup.RemoteAddr
	jellyfin.Host = setup.Host
	jellyfin.Header.Set("Content-Type", "application/json")
	jellyfinRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(jellyfinRec, jellyfin)
	if jellyfinRec.Code != http.StatusOK {
		t.Fatalf("Jellyfin login status = %d, body=%s", jellyfinRec.Code, jellyfinRec.Body.String())
	}
	var jellyfinResponse struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.Unmarshal(jellyfinRec.Body.Bytes(), &jellyfinResponse); err != nil {
		t.Fatal(err)
	}
	if jellyfinResponse.AccessToken == "" {
		t.Fatal("Jellyfin login returned an empty access token")
	}

	items := httptest.NewRequest(http.MethodGet, "/Items?Limit=1", nil)
	items.RemoteAddr = setup.RemoteAddr
	items.Host = setup.Host
	items.Header.Set("Authorization", `MediaBrowser Client="BookPlayer", Token="`+jellyfinResponse.AccessToken+`"`)
	itemsRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(itemsRec, items)
	if itemsRec.Code != http.StatusOK {
		t.Fatalf("Jellyfin token-authenticated items status = %d, body=%s", itemsRec.Code, itemsRec.Body.String())
	}

	audiobookshelf := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"owner","password":"a-long-test-password"}`))
	audiobookshelf.RemoteAddr = setup.RemoteAddr
	audiobookshelf.Host = setup.Host
	audiobookshelf.Header.Set("Content-Type", "application/json")
	audiobookshelfRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(audiobookshelfRec, audiobookshelf)
	if audiobookshelfRec.Code != http.StatusOK {
		t.Fatalf("Audiobookshelf login status = %d, body=%s", audiobookshelfRec.Code, audiobookshelfRec.Body.String())
	}
	var audiobookshelfResponse struct {
		User struct {
			Token string `json:"token"`
		} `json:"user"`
	}
	if err := json.Unmarshal(audiobookshelfRec.Body.Bytes(), &audiobookshelfResponse); err != nil {
		t.Fatal(err)
	}
	if audiobookshelfResponse.User.Token == "" {
		t.Fatal("Audiobookshelf login returned an empty token")
	}

	libraries := httptest.NewRequest(http.MethodGet, "/api/libraries", nil)
	libraries.RemoteAddr = setup.RemoteAddr
	libraries.Host = setup.Host
	libraries.Header.Set("Authorization", "Bearer "+audiobookshelfResponse.User.Token)
	librariesRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(librariesRec, libraries)
	if librariesRec.Code != http.StatusOK {
		t.Fatalf("Audiobookshelf token-authenticated libraries status = %d, body=%s", librariesRec.Code, librariesRec.Body.String())
	}
}

func TestCompatibilityCredentialAliasOnlyAppliesToMediaServerLogin(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) {
		cfg.AllowLAN = true
		cfg.PairingToken = "pair-me"
		cfg.CompatibilityUsername = "sonder"
		cfg.CompatibilityPassword = "sonder"
	})
	if err := f.s.accounts.Setup("owner", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}

	login := httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName", strings.NewReader(`{"Username":"sonder","Pw":"sonder"}`))
	login.RemoteAddr = "192.168.3.50:50123"
	login.Host = "sonder.example:8096"
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("compatibility alias status = %d, body=%s", loginRec.Code, loginRec.Body.String())
	}
	var result struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.AccessToken == "" {
		t.Fatal("compatibility alias returned an empty access token")
	}

	items := httptest.NewRequest(http.MethodGet, "/Items?Limit=1", nil)
	items.RemoteAddr = login.RemoteAddr
	items.Host = login.Host
	items.Header.Set("Authorization", `MediaBrowser Client="BookPlayer", Token="`+result.AccessToken+`"`)
	itemsRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(itemsRec, items)
	if itemsRec.Code != http.StatusOK {
		t.Fatalf("compatibility alias token status = %d, body=%s", itemsRec.Code, itemsRec.Body.String())
	}

	// The alias is deliberately not a browser/private-API password.
	browser := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"sonder","password":"sonder"}`))
	browser.RemoteAddr = login.RemoteAddr
	browser.Host = login.Host
	browser.Header.Set("Content-Type", "application/json")
	browserRec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(browserRec, browser)
	if browserRec.Code != http.StatusUnauthorized {
		t.Fatalf("compatibility alias unexpectedly authenticated browser login: %d", browserRec.Code)
	}
}

func TestRemoteBrowserPageRedirectsToLogin(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) {
		cfg.AllowLAN = true
		cfg.PairingToken = "pair-me"
	})
	req := httptest.NewRequest(http.MethodGet, "/audiobooks", nil)
	req.RemoteAddr = "192.168.3.50:50123"
	req.Host = "sonder.example:8096"
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("remote browser status = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.HasPrefix(got, "/account/login?next=") {
		t.Fatalf("redirect location = %q", got)
	}
}
