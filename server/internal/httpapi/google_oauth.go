package httpapi

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleOAuthCookie = "sonder_google_oauth"
	googleCallback    = "/oauth2/callback"
)

type googleOAuthState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Username string `json:"u,omitempty"`
	Link     bool   `json:"l,omitempty"`
	Next     string `json:"r"`
	Expires  int64  `json:"e"`
}

func (s *Server) googleOAuthReady() bool {
	cfg := s.cfg()
	return cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""
}

func (s *Server) handleGoogleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if !s.googleOAuthReady() {
		http.Error(w, "Google sign-in is not configured", http.StatusServiceUnavailable)
		return
	}
	state := googleOAuthState{Next: safeNext(r.URL.Query().Get("next")), Expires: time.Now().Add(10 * time.Minute).Unix()}
	if r.URL.Path == "/oauth2/link" {
		username, ok := s.sessionUsername(r)
		if !ok {
			http.Redirect(w, r, "/account/login", http.StatusSeeOther)
			return
		}
		state.Link = true
		state.Username = username
	}
	state.State, state.Nonce, state.Verifier = oauthRandom(), oauthRandom(), oauthRandom()
	if state.State == "" || state.Nonce == "" || state.Verifier == "" {
		http.Error(w, "Could not start Google sign-in", http.StatusInternalServerError)
		return
	}
	payload, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "Could not start Google sign-in", http.StatusInternalServerError)
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(s.cfg().GoogleClientSecret))
	_, _ = mac.Write([]byte(encoded))
	cookieValue := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{Name: googleOAuthCookie, Value: cookieValue, Path: "/oauth2", HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	query := url.Values{
		"client_id":             {s.cfg().GoogleClientID},
		"redirect_uri":          {googleRedirectURL(r)},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {state.State},
		"nonce":                 {state.Nonce},
		"prompt":                {"select_account"},
		"code_challenge":        {googleCodeChallenge(state.Verifier)},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+query.Encode(), http.StatusSeeOther)
}

func googleRedirectURL(r *http.Request) string {
	return "https://sonder.stoverparc.org" + googleCallback
}

func oauthRandom() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) readGoogleOAuthState(r *http.Request, expectedState string) (googleOAuthState, error) {
	cookie, err := r.Cookie(googleOAuthCookie)
	if err != nil {
		return googleOAuthState{}, errors.New("OAuth state cookie is missing")
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return googleOAuthState{}, errors.New("OAuth state cookie is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return googleOAuthState{}, errors.New("OAuth state cookie is invalid")
	}
	mac := hmac.New(sha256.New, []byte(s.cfg().GoogleClientSecret))
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return googleOAuthState{}, errors.New("OAuth state cookie is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return googleOAuthState{}, errors.New("OAuth state cookie is invalid")
	}
	var state googleOAuthState
	if err := json.Unmarshal(payload, &state); err != nil || state.State == "" || state.Nonce == "" || state.State != expectedState || time.Now().Unix() >= state.Expires {
		return googleOAuthState{}, errors.New("OAuth state has expired or does not match")
	}
	return state, nil
}

func (s *Server) handleGoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: googleOAuthCookie, Value: "", Path: "/oauth2", HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if !s.googleOAuthReady() {
		http.Error(w, "Google sign-in is not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := s.readGoogleOAuthState(r, r.URL.Query().Get("state"))
	if err != nil || r.URL.Query().Get("error") != "" {
		googleOAuthFailure(w, "Google sign-in could not be verified. Please try again.")
		return
	}
	if state.Link {
		username, ok := s.sessionUsername(r)
		if !ok || username != state.Username {
			googleOAuthFailure(w, "Your Sonder session expired. Sign in again, then link Google from your profile.")
			return
		}
	}
	identity, err := exchangeGoogleCode(r.Context(), s.cfg().GoogleClientID, s.cfg().GoogleClientSecret, googleRedirectURL(r), r.URL.Query().Get("code"), state.Verifier, state.Nonce)
	if err != nil {
		s.logger.Printf("Google sign-in verification failed: %v", err)
		googleOAuthFailure(w, "Google sign-in could not be verified. Please try again.")
		return
	}
	if state.Link {
		if err := s.accounts.LinkGoogle(state.Username, identity.Subject); err != nil {
			googleOAuthFailure(w, "That Google account could not be linked. It may already be linked to another Sonder account.")
			return
		}
		http.Redirect(w, r, state.Next, http.StatusSeeOther)
		return
	}
	username, ok := s.accounts.GoogleUsername(identity.Subject)
	if !ok {
		googleOAuthFailure(w, "This Google account is not linked yet. Sign in with your existing Sonder username and password, then choose Link Google account from your profile.")
		return
	}
	token, expires, err := s.accounts.CreateSession(username)
	if err != nil {
		googleOAuthFailure(w, "Could not create a Sonder session. Please try again.")
		return
	}
	s.setSessionCookie(w, r, token, expires)
	http.Redirect(w, r, state.Next, http.StatusSeeOther)
}

func googleOAuthFailure(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Google sign-in · TM Sonder</title><body style="font:16px system-ui;background:#101713;color:#edf5ed;min-height:90vh;display:grid;place-items:center"><main style="max-width:34rem;padding:2rem;background:#17231b;border:1px solid #4f755c;border-radius:16px"><h1>Google sign-in</h1><p>%s</p><a style="color:#c5e6bb" href="/account/login">Back to sign in</a></main></body></html>`, html.EscapeString(message))
}

type googleIdentity struct{ Subject string }

type googleTokenResponse struct {
	IDToken string `json:"id_token"`
}

type googleJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}
type googleJWTClaims struct {
	Issuer          string          `json:"iss"`
	Audience        json.RawMessage `json:"aud"`
	AuthorizedParty string          `json:"azp"`
	Subject         string          `json:"sub"`
	Expires         int64           `json:"exp"`
	IssuedAt        int64           `json:"iat"`
	Nonce           string          `json:"nonce"`
	Email           string          `json:"email"`
	EmailVerified   bool            `json:"email_verified"`
}
type googleJWKSet struct {
	Keys []struct {
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func googleCodeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func exchangeGoogleCode(ctx context.Context, clientID, clientSecret, redirectURL, code, verifier, nonce string) (googleIdentity, error) {
	if code == "" {
		return googleIdentity{}, errors.New("authorization code is missing")
	}
	form := url.Values{"code": {code}, "client_id": {clientID}, "client_secret": {clientSecret}, "redirect_uri": {redirectURL}, "grant_type": {"authorization_code"}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return googleIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return googleIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleIdentity{}, fmt.Errorf("Google token endpoint returned %s", resp.Status)
	}
	var tokens googleTokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tokens); err != nil {
		return googleIdentity{}, err
	}
	return verifyGoogleIDToken(ctx, client, tokens.IDToken, clientID, nonce)
}

func verifyGoogleIDToken(ctx context.Context, client *http.Client, token, clientID, nonce string) (googleIdentity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return googleIdentity{}, errors.New("malformed ID token")
	}
	decode := func(part string, target any) error {
		b, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, target)
	}
	var header googleJWTHeader
	var claims googleJWTClaims
	if err := decode(parts[0], &header); err != nil {
		return googleIdentity{}, err
	}
	if err := decode(parts[1], &claims); err != nil {
		return googleIdentity{}, err
	}
	if header.Alg != "RS256" || header.Kid == "" {
		return googleIdentity{}, errors.New("unsupported ID token signature")
	}
	if claims.Issuer != "https://accounts.google.com" && claims.Issuer != "accounts.google.com" {
		return googleIdentity{}, errors.New("invalid ID token issuer")
	}
	if !googleAudienceContains(claims.Audience, clientID) || (claims.AuthorizedParty != "" && claims.AuthorizedParty != clientID) || (googleAudienceCount(claims.Audience) > 1 && claims.AuthorizedParty != clientID) {
		return googleIdentity{}, errors.New("invalid ID token audience")
	}
	now := time.Now().Unix()
	if claims.Expires <= now || claims.IssuedAt > now+60 || claims.Subject == "" || claims.Nonce != nonce || !claims.EmailVerified {
		return googleIdentity{}, errors.New("ID token claims failed validation")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/certs", nil)
	if err != nil {
		return googleIdentity{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return googleIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleIdentity{}, fmt.Errorf("Google signing-key endpoint returned %s", resp.Status)
	}
	var keys googleJWKSet
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&keys); err != nil {
		return googleIdentity{}, err
	}
	var signingKey *rsa.PublicKey
	for _, key := range keys.Keys {
		if key.Kid != header.Kid || key.Kty != "RSA" || (key.Alg != "" && key.Alg != "RS256") || (key.Use != "" && key.Use != "sig") {
			continue
		}
		modulus, errN := base64.RawURLEncoding.DecodeString(key.N)
		exponent, errE := base64.RawURLEncoding.DecodeString(key.E)
		if errN != nil || errE != nil || len(exponent) == 0 {
			continue
		}
		e := 0
		for _, b := range exponent {
			e = e<<8 | int(b)
		}
		if e < 3 {
			continue
		}
		signingKey = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: e}
		break
	}
	if signingKey == nil {
		return googleIdentity{}, errors.New("Google signing key not found")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return googleIdentity{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(signingKey, crypto.SHA256, digest[:], signature); err != nil {
		return googleIdentity{}, errors.New("invalid ID token signature")
	}
	return googleIdentity{Subject: claims.Subject}, nil
}

func googleAudienceContains(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, audience := range multiple {
		if audience == expected {
			return true
		}
	}
	return false
}

func googleAudienceCount(raw json.RawMessage) int {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return 1
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) == nil {
		return len(multiple)
	}
	return 0
}
