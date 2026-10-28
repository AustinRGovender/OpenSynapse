package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// oauthFlow is one in-progress authorization-code flow.
type oauthFlow struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Verifier     string
	Created      time.Time
	Token        *oauthToken
	Err          string
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// oauthFlowTTL bounds how long a user has to finish signing in.
const oauthFlowTTL = 10 * time.Minute

func randomURLToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64URL(b)
}

// callbackURL is where the provider sends the browser back to: this
// control plane, as seen by the browser (behind nginx the Host header and
// X-Forwarded-Proto describe the public address).
func callbackURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/v1/playground/oauth/callback"
}

// OAuthStart begins an authorization-code flow with PKCE and returns the
// URL to open in a popup.
func (h *PlaygroundHandlers) OAuthStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthURL      string `json:"auth_url"`
		TokenURL     string `json:"token_url"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	authURL, err := url.Parse(req.AuthURL)
	if err != nil || req.AuthURL == "" || req.TokenURL == "" || req.ClientID == "" {
		badRequest(w, "VALIDATION_ERROR", "auth_url, token_url and client_id are required", nil)
		return
	}

	verifier := randomURLToken(32)
	sum := sha256.Sum256([]byte(verifier))
	state := randomURLToken(24)
	redirect := callbackURL(r)

	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", req.ClientID)
	q.Set("redirect_uri", redirect)
	q.Set("state", state)
	q.Set("code_challenge", base64URL(sum[:]))
	q.Set("code_challenge_method", "S256")
	if req.Scope != "" {
		q.Set("scope", req.Scope)
	}
	authURL.RawQuery = q.Encode()

	h.oauthMu.Lock()
	for k, f := range h.oauthPending {
		if time.Since(f.Created) > oauthFlowTTL {
			delete(h.oauthPending, k)
		}
	}
	h.oauthPending[state] = &oauthFlow{TokenURL: req.TokenURL, ClientID: req.ClientID, ClientSecret: req.ClientSecret,
		RedirectURI: redirect, Verifier: verifier, Created: time.Now()}
	h.oauthMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"authorize_url": authURL.String(), "state": state, "redirect_uri": redirect})
}

var callbackPage = template.Must(template.New("cb").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>OpenSynapse sign-in</title></head>
<body style="font-family: system-ui, sans-serif; padding: 2rem;">
<p>{{if .OK}}Signed in. You can close this window.{{else}}Sign-in failed: {{.Err}}{{end}}</p>
<script>
  if (window.opener) {
    window.opener.postMessage({ type: 'opensynapse-oauth', state: {{.State}}, ok: {{.OK}} }, window.location.origin);
    {{if .OK}}window.close();{{end}}
  }
</script>
</body></html>`))

// OAuthCallback receives the provider's redirect and exchanges the code.
// The page only tells the opener the flow finished; the token itself is
// fetched by the authenticated UI from OAuthResult.
func (h *PlaygroundHandlers) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	h.oauthMu.Lock()
	flow := h.oauthPending[state]
	h.oauthMu.Unlock()

	data := struct {
		OK    bool
		Err   string
		State string
	}{State: state}
	switch {
	case flow == nil:
		data.Err = "unknown or expired sign-in attempt"
	case q.Get("error") != "":
		data.Err = q.Get("error") + " " + q.Get("error_description")
		flow.Err = data.Err
	default:
		tok, err := exchangeCode(flow, q.Get("code"))
		if err != nil {
			data.Err = err.Error()
			flow.Err = data.Err
		} else {
			flow.Token = tok
			data.OK = true
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	callbackPage.Execute(w, data)
}

// OAuthResult returns (once) the token obtained by a finished flow.
func (h *PlaygroundHandlers) OAuthResult(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	h.oauthMu.Lock()
	flow := h.oauthPending[state]
	if flow != nil && (flow.Token != nil || flow.Err != "") {
		delete(h.oauthPending, state)
	}
	h.oauthMu.Unlock()
	switch {
	case flow == nil:
		notFound(w, "OAUTH_FLOW", state)
	case flow.Err != "":
		writeError(w, http.StatusBadGateway, "OAUTH_FAILED", flow.Err, nil)
	case flow.Token == nil:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
	default:
		writeJSON(w, http.StatusOK, flow.Token)
	}
}

func postToken(tokenURL string, form url.Values) (*oauthToken, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tok oauthToken
	if json.Unmarshal(body, &tok) != nil || tok.AccessToken == "" {
		// Some providers (e.g. GitHub) answer form-encoded.
		if v, err := url.ParseQuery(string(body)); err == nil && v.Get("access_token") != "" {
			tok.AccessToken, tok.TokenType, tok.Scope = v.Get("access_token"), v.Get("token_type"), v.Get("scope")
		} else {
			return nil, fmt.Errorf("token endpoint response has no access_token")
		}
	}
	return &tok, nil
}

func exchangeCode(f *oauthFlow, code string) (*oauthToken, error) {
	if code == "" {
		return nil, fmt.Errorf("the provider did not return a code")
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {f.RedirectURI},
		"client_id":     {f.ClientID},
		"code_verifier": {f.Verifier},
	}
	if f.ClientSecret != "" {
		form.Set("client_secret", f.ClientSecret)
	}
	return postToken(f.TokenURL, form)
}

func fetchClientCredentialsToken(tokenURL, clientID, clientSecret, scope string) (*oauthToken, error) {
	if tokenURL == "" || clientID == "" {
		return nil, fmt.Errorf("client credentials need a token URL and client ID")
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {clientSecret}}
	if scope != "" {
		form.Set("scope", scope)
	}
	return postToken(tokenURL, form)
}

// OAuthClientCredentials fetches a token with the client-credentials grant
// so the UI can show and reuse it.
func (h *PlaygroundHandlers) OAuthClientCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TokenURL     string `json:"token_url"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	tok, err := fetchClientCredentialsToken(req.TokenURL, req.ClientID, req.ClientSecret, req.Scope)
	if err != nil {
		writeError(w, http.StatusBadGateway, "OAUTH_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, tok)
}
