package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tm-sonder/server/internal/auth"
)

const sessionCookieName = "sonder_session"

type accountCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Next     string `json:"next"`
}

func isAccountPath(path string) bool {
	switch path {
	case "/account/login", "/account/setup", "/api/auth/session", "/api/auth/login", "/api/auth/setup", "/api/auth/logout":
		return true
	default:
		return false
	}
}

func (s *Server) accountReady() bool { return s.accounts != nil && s.authLoadErr == nil }

func (s *Server) sessionUsername(r *http.Request) (string, bool) {
	if !s.accountReady() {
		return "", false
	}
	token := ""
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		token = cookie.Value
	}
	if token == "" {
		if authHeader := r.Header.Get("Authorization"); len(authHeader) >= 7 && strings.EqualFold(authHeader[:7], "bearer ") {
			token = strings.TrimSpace(authHeader[7:])
		}
	}
	return s.accounts.ValidSession(token)
}

func (s *Server) validAccountRequest(r *http.Request) bool {
	if _, ok := s.sessionUsername(r); ok {
		return true
	}
	return s.tokenMatches(r)
}

func (s *Server) setupAuthorized(r *http.Request) bool {
	if !s.accountReady() || s.accounts.HasAccount() {
		return false
	}
	if isLoopback(peerHost(r)) && hostIsLoopback(r.Host) {
		return true
	}
	return s.tokenMatches(r)
}

func (s *Server) issueAccountSession(username, password string) (string, time.Time, error) {
	if !s.accountReady() {
		if s.authLoadErr != nil {
			return "", time.Time{}, s.authLoadErr
		}
		return "", time.Time{}, auth.ErrNoAccount
	}
	username = strings.TrimSpace(username)
	if err := s.accounts.Authenticate(username, password); err != nil {
		// Media-server clients often keep their server definition but ask for
		// credentials again after a token reset. An optional, environment-only
		// compatibility pair gives BookPlayer/Audiobookshelf a stable login
		// without weakening browser or private-API authentication.
		cfg := s.cfg()
		if cfg.CompatibilityUsername == "" ||
			subtle.ConstantTimeCompare([]byte(username), []byte(cfg.CompatibilityUsername)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(cfg.CompatibilityPassword)) != 1 {
			return "", time.Time{}, err
		}
		username = s.accounts.Username()
	}
	return s.accounts.CreateSession(username)
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if !s.accountReady() {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Account store unavailable",
		})
		return
	}
	username, authenticated := s.sessionUsername(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"setupRequired": !s.accounts.HasAccount(),
		"authenticated": authenticated || s.tokenMatches(r),
		"username":      username,
	})
}

func (s *Server) handleAccountLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.accountReady() {
		http.Error(w, "Account store unavailable", http.StatusInternalServerError)
		return
	}
	if _, ok := s.sessionUsername(r); ok {
		http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	if !s.accounts.HasAccount() && s.setupAuthorized(r) {
		location := "/account/setup?next=" + url.QueryEscape(next)
		if token := r.URL.Query().Get("token"); token != "" {
			location += "&token=" + url.QueryEscape(token)
		}
		http.Redirect(w, r, location, http.StatusSeeOther)
		return
	}
	renderAccountPage(w, false, next, "", "")
}

func (s *Server) handleAccountSetupPage(w http.ResponseWriter, r *http.Request) {
	if !s.accountReady() {
		http.Error(w, "Account store unavailable", http.StatusInternalServerError)
		return
	}
	if s.accounts.HasAccount() {
		http.Redirect(w, r, "/account/login", http.StatusSeeOther)
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	message := ""
	if !s.setupAuthorized(r) {
		message = "Account setup requires the one-time pairing token in the URL."
	}
	renderAccountPage(w, true, next, message, r.URL.Query().Get("token"))
}

func (s *Server) handleAccountLogin(w http.ResponseWriter, r *http.Request) {
	if !s.accountReady() {
		writeError(w, http.StatusInternalServerError, "Account store unavailable")
		return
	}
	if !s.accounts.HasAccount() {
		writeError(w, http.StatusConflict, "Create the first account before signing in")
		return
	}
	credentials, form, err := decodeAccountCredentials(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid login request")
		return
	}
	if err := s.accounts.Authenticate(credentials.Username, credentials.Password); err != nil {
		if form {
			renderAccountPage(w, false, safeNext(credentials.Next), "Incorrect username or password.", "")
			return
		}
		writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	token, expires, err := s.accounts.CreateSession(strings.TrimSpace(credentials.Username))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	s.setSessionCookie(w, r, token, expires)
	if form {
		http.Redirect(w, r, safeNext(credentials.Next), http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      strings.TrimSpace(credentials.Username),
		"expiresAt":     expires,
	})
}

func (s *Server) handleAccountSetup(w http.ResponseWriter, r *http.Request) {
	if !s.accountReady() {
		writeError(w, http.StatusInternalServerError, "Account store unavailable")
		return
	}
	if !s.setupAuthorized(r) {
		writeError(w, http.StatusForbidden, "First account setup requires the pairing token")
		return
	}
	credentials, form, err := decodeAccountCredentials(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account setup request")
		return
	}
	if err := s.accounts.Setup(credentials.Username, credentials.Password); err != nil {
		if form {
			renderAccountPage(w, true, safeNext(credentials.Next), err.Error(), r.URL.Query().Get("token"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, expires, err := s.accounts.CreateSession(strings.TrimSpace(credentials.Username))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	s.setSessionCookie(w, r, token, expires)
	if form {
		http.Redirect(w, r, safeNext(credentials.Next), http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"authenticated": true,
		"username":      strings.TrimSpace(credentials.Username),
		"expiresAt":     expires,
	})
}

func (s *Server) handleAccountLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && s.accounts != nil {
		s.accounts.Revoke(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsSecure(r),
	})
	if isHTMLForm(r) {
		http.Redirect(w, r, "/account/login", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expires,
		MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true,
		Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode,
	})
}

func decodeAccountCredentials(r *http.Request) (accountCredentials, bool, error) {
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		var credentials accountCredentials
		dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
		if err := dec.Decode(&credentials); err != nil {
			return accountCredentials{}, false, err
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return accountCredentials{}, false, err
		}
		return credentials, false, nil
	}
	if err := r.ParseForm(); err != nil {
		return accountCredentials{}, true, err
	}
	return accountCredentials{
		Username: r.FormValue("username"),
		Password: r.FormValue("password"),
		Next:     r.FormValue("next"),
	}, true, nil
}

func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func isHTMLForm(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.HasPrefix(contentType, "application/x-www-form-urlencoded") || strings.Contains(accept, "text/html")
}

func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

func renderAccountPage(w http.ResponseWriter, setup bool, next, message, setupToken string) {
	title := "Sign in to TM Sonder"
	heading := "Welcome back"
	button := "Sign in"
	action := "/api/auth/login"
	passwordHint := ""
	if setup {
		title = "Create your TM Sonder account"
		heading = "Create your account"
		button = "Create account"
		action = "/api/auth/setup"
		passwordHint = `<small>Use at least 12 characters. Your password is stored only as a salted hash.</small>`
		if setupToken != "" {
			action += "?token=" + url.QueryEscape(setupToken)
		}
	}
	messageHTML := ""
	if message != "" {
		messageHTML = `<p class="message">` + html.EscapeString(message) + `</p>`
	}
	form := `<form method="post" action="` + html.EscapeString(action) + `">
<input type="hidden" name="next" value="` + html.EscapeString(safeNext(next)) + `">
<label>Username<input name="username" autocomplete="username" required autofocus></label>
<label>Password<input type="password" name="password" autocomplete="` + map[bool]string{true: "new-password", false: "current-password"}[setup] + `" minlength="12" required></label>` + passwordHint + `
<button type="submit">` + html.EscapeString(button) + `</button>
</form>`
	if !setup {
		form += `<p class="secondary">Need the first account? Use the one-time setup link from the server owner.</p>`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="application-name" content="TM Sonder"><meta name="apple-mobile-web-app-title" content="TM Sonder"><link rel="icon" type="image/png" href="/favicon.png?v=asset"><link rel="alternate icon" type="image/svg+xml" href="/favicon.svg"><link rel="apple-touch-icon" href="/favicon.png?v=asset"><title>` + html.EscapeString(title) + `</title><style>
:root{color-scheme:dark;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#101713;color:#edf5ed}body{min-height:100vh;display:grid;place-items:center;margin:0;background:radial-gradient(circle at top,#274331,#101713 62%)}main{width:min(92vw,420px);padding:34px;border:1px solid #4f755c;border-radius:18px;background:#17231b;box-shadow:0 20px 70px #0008}h1{font-size:1.45rem;margin:0 0 24px}label{display:grid;gap:8px;margin:16px 0;font-weight:600}input{box-sizing:border-box;width:100%;padding:12px;border:1px solid #66866d;border-radius:9px;background:#0e1711;color:inherit;font:inherit}button{width:100%;margin-top:14px;padding:12px;border:0;border-radius:9px;background:#b6d9ad;color:#102014;font:inherit;font-weight:700;cursor:pointer}.message{padding:11px;border-radius:9px;background:#552d2d;color:#ffd7d7}.secondary,small{display:block;margin-top:18px;color:#b9c9bc;font-size:.86rem;line-height:1.45}a{color:#c5e6bb}</style></head><body><main><h1>` + html.EscapeString(heading) + `</h1>` + messageHTML + form + `</main></body></html>`))
}
