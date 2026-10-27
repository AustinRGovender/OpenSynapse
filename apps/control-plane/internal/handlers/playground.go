package handlers

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
)

// PlaygroundHandlers serve the endpoint playground (F-03).
type PlaygroundHandlers struct {
	collections *db.CollectionStore
	// Envs resolves ${VAR} references and "inherit" auth (optional).
	Envs *db.EnvironmentStore
	// Engine runs micro-tests (optional).
	Engine *engine.Engine

	oauthMu      sync.Mutex
	oauthPending map[string]*oauthFlow
}

func NewPlaygroundHandlers(collections *db.CollectionStore) *PlaygroundHandlers {
	return &PlaygroundHandlers{collections: collections, oauthPending: map[string]*oauthFlow{}}
}

// PlaygroundRequest is the request spec from the UI.
type PlaygroundRequest struct {
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	BodyType      string            `json:"body_type"`
	AuthType      string            `json:"auth_type"` // none, basic, bearer, api_key, digest, oauth2, inherit
	AuthConfig    json.RawMessage   `json:"auth_config"`
	EnvironmentID string            `json:"environment_id,omitempty"`
}

// PlaygroundResponse is what we return to the UI.
type PlaygroundResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	Cookies    []CookieInfo      `json:"cookies"`
	Body       string            `json:"body"`
	BodySize   int               `json:"body_size"`
	Truncated  bool              `json:"truncated"`
	Timing     TimingBreakdown   `json:"timing"`
	FinalURL   string            `json:"final_url"`
}

// CookieInfo is a cookie set by the response.
type CookieInfo struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"http_only"`
	Secure   bool   `json:"secure"`
}

// TimingBreakdown matches the spec: DNS, connect, TLS, send, wait, receive.
type TimingBreakdown struct {
	DNSMS     float64 `json:"dns_ms"`
	ConnectMS float64 `json:"connect_ms"`
	TLSMS     float64 `json:"tls_ms"`
	SendMS    float64 `json:"send_ms"`
	WaitMS    float64 `json:"wait_ms"`
	ReceiveMS float64 `json:"receive_ms"`
	TotalMS   float64 `json:"total_ms"`
}

const maxResponseBody = 10 * 1024 * 1024

var playgroundVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// interpolate replaces ${NAME} with environment values; unknown names stay.
func interpolate(s string, vars map[string]string) string {
	if len(vars) == 0 || !strings.Contains(s, "${") {
		return s
	}
	return playgroundVarRe.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vars[m[2:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

func (h *PlaygroundHandlers) envVars(id string) (map[string]string, error) {
	if id == "" || h.Envs == nil {
		return nil, nil
	}
	env, err := h.Envs.Get(id)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, fmt.Errorf("environment %s not found", id)
	}
	vars := map[string]string{}
	for k, v := range env.Variables {
		vars[k] = v.Value
	}
	return vars, nil
}

// resolve applies environment interpolation to a request spec.
func (req *PlaygroundRequest) resolve(vars map[string]string) {
	req.URL = interpolate(req.URL, vars)
	req.Body = interpolate(req.Body, vars)
	for k, v := range req.Headers {
		req.Headers[k] = interpolate(v, vars)
	}
	if len(req.AuthConfig) > 0 {
		var cfg map[string]any
		if json.Unmarshal(req.AuthConfig, &cfg) == nil {
			for k, v := range cfg {
				if s, ok := v.(string); ok {
					cfg[k] = interpolate(s, vars)
				}
			}
			req.AuthConfig, _ = json.Marshal(cfg)
		}
	}
}

func buildRequest(req *PlaygroundRequest) (*http.Request, error) {
	var body io.Reader
	if req.Body != "" && req.Method != "GET" && req.Method != "HEAD" {
		body = strings.NewReader(req.Body)
	}
	httpReq, err := http.NewRequest(req.Method, req.URL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		if strings.TrimSpace(k) != "" {
			httpReq.Header.Set(k, v)
		}
	}
	switch {
	case req.BodyType == "json" && httpReq.Header.Get("Content-Type") == "":
		httpReq.Header.Set("Content-Type", "application/json")
	case req.BodyType == "form" && httpReq.Header.Get("Content-Type") == "":
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return httpReq, nil
}

// ExecuteRequest handles POST /api/v1/playground/request
func (h *PlaygroundHandlers) ExecuteRequest(w http.ResponseWriter, r *http.Request) {
	var req PlaygroundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	vars, err := h.envVars(req.EnvironmentID)
	if err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	req.resolve(vars)
	if req.URL == "" {
		badRequest(w, "VALIDATION_ERROR", "URL is required", nil)
		return
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	req.Method = strings.ToUpper(req.Method)
	if u, err := url.Parse(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		badRequest(w, "VALIDATION_ERROR", "URL must be an absolute http:// or https:// address", nil)
		return
	}

	httpReq, err := buildRequest(&req)
	if err != nil {
		badRequest(w, "INVALID_REQUEST", fmt.Sprintf("Cannot create request: %s", err), nil)
		return
	}
	if err := h.applyAuth(httpReq, req.AuthType, req.AuthConfig, vars); err != nil {
		badRequest(w, "AUTH_ERROR", err.Error(), nil)
		return
	}

	result, err := send(httpReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "REQUEST_FAILED", fmt.Sprintf("Request failed: %s", err), nil)
		return
	}
	// Digest auth needs the server's challenge: retry once with credentials.
	if req.AuthType == "digest" && result.Status == http.StatusUnauthorized {
		if challenge := result.Headers["Www-Authenticate"]; strings.HasPrefix(challenge, "Digest ") {
			retry, err := buildRequest(&req)
			if err == nil {
				var cfg struct{ Username, Password string }
				json.Unmarshal(req.AuthConfig, &cfg)
				retry.Header.Set("Authorization", digestHeader(challenge, retry.Method, retry.URL.RequestURI(), cfg.Username, cfg.Password))
				if again, err := send(retry); err == nil {
					result = again
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// send executes a request with phase timing.
func send(httpReq *http.Request) (*PlaygroundResponse, error) {
	var dnsStart, dnsEnd, connStart, connEnd, tlsStart, tlsEnd, wrote, firstByte time.Time
	start := time.Now()
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(httptrace.DNSDoneInfo) { dnsEnd = time.Now() },
		ConnectStart:         func(_, _ string) { connStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { connEnd = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsEnd = time.Now() },
		WroteRequest:         func(httptrace.WroteRequestInfo) { wrote = time.Now() },
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}
	httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), trace))

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	end := time.Now()
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	ms := func(a, b time.Time) float64 {
		if a.IsZero() || b.IsZero() || b.Before(a) {
			return 0
		}
		return float64(b.Sub(a).Microseconds()) / 1000
	}
	t := TimingBreakdown{
		DNSMS:     ms(dnsStart, dnsEnd),
		ConnectMS: ms(connStart, connEnd),
		TLSMS:     ms(tlsStart, tlsEnd),
		WaitMS:    ms(wrote, firstByte),
		ReceiveMS: ms(firstByte, end),
		TotalMS:   ms(start, end),
	}
	// Sending is everything between the connection being ready and the
	// request being written; it absorbs the remainder so phases sum to total.
	t.SendMS = t.TotalMS - t.DNSMS - t.ConnectMS - t.TLSMS - t.WaitMS - t.ReceiveMS
	if t.SendMS < 0 {
		t.SendMS = 0
	}

	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}
	cookies := []CookieInfo{}
	for _, c := range resp.Cookies() {
		ci := CookieInfo{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, HTTPOnly: c.HttpOnly, Secure: c.Secure}
		if !c.Expires.IsZero() {
			ci.Expires = c.Expires.UTC().Format(time.RFC3339)
		}
		cookies = append(cookies, ci)
	}
	truncated := len(body) > maxResponseBody
	if truncated {
		body = body[:maxResponseBody]
	}
	return &PlaygroundResponse{
		Status: resp.StatusCode, StatusText: resp.Status, Headers: headers, Cookies: cookies,
		Body: string(body), BodySize: len(body), Truncated: truncated, Timing: t, FinalURL: resp.Request.URL.String(),
	}, nil
}

// applyAuth sets credentials on the request. The seven supported methods
// (F-03): none, basic, bearer, api_key (header or query), digest, oauth2
// (an access token from the authorization-code flow, or fetched with client
// credentials), and inherit (credentials from the selected environment).
func (h *PlaygroundHandlers) applyAuth(req *http.Request, authType string, authConfig json.RawMessage, vars map[string]string) error {
	switch authType {
	case "", "none":
	case "basic":
		var cfg struct{ Username, Password string }
		json.Unmarshal(authConfig, &cfg)
		req.SetBasicAuth(cfg.Username, cfg.Password)
	case "bearer":
		var cfg struct{ Token string }
		json.Unmarshal(authConfig, &cfg)
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	case "api_key":
		var cfg struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			AddTo string `json:"add_to"` // header | query
		}
		json.Unmarshal(authConfig, &cfg)
		if cfg.Key == "" {
			return fmt.Errorf("API key name is required")
		}
		if cfg.AddTo == "query" {
			q := req.URL.Query()
			q.Set(cfg.Key, cfg.Value)
			req.URL.RawQuery = q.Encode()
		} else {
			req.Header.Set(cfg.Key, cfg.Value)
		}
	case "digest":
		// Applied after the server's 401 challenge (see ExecuteRequest).
	case "oauth2":
		var cfg struct {
			GrantType    string `json:"grant_type"` // authorization_code | client_credentials
			AccessToken  string `json:"access_token"`
			TokenURL     string `json:"token_url"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			Scope        string `json:"scope"`
		}
		json.Unmarshal(authConfig, &cfg)
		token := cfg.AccessToken
		if cfg.GrantType == "client_credentials" && token == "" {
			tok, err := fetchClientCredentialsToken(cfg.TokenURL, cfg.ClientID, cfg.ClientSecret, cfg.Scope)
			if err != nil {
				return err
			}
			token = tok.AccessToken
		}
		if token == "" {
			return fmt.Errorf("no OAuth access token yet: click \"Get new access token\" first")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	case "inherit":
		switch {
		case vars["AUTH_TOKEN"] != "":
			req.Header.Set("Authorization", "Bearer "+vars["AUTH_TOKEN"])
		case vars["AUTH_USERNAME"] != "":
			req.SetBasicAuth(vars["AUTH_USERNAME"], vars["AUTH_PASSWORD"])
		case vars["API_KEY"] != "":
			header := vars["API_KEY_HEADER"]
			if header == "" {
				header = "X-API-Key"
			}
			req.Header.Set(header, vars["API_KEY"])
		default:
			return fmt.Errorf("the selected environment has no AUTH_TOKEN, AUTH_USERNAME/AUTH_PASSWORD, or API_KEY variable to inherit")
		}
	default:
		return fmt.Errorf("unknown auth type %q", authType)
	}
	return nil
}

var digestParamRe = regexp.MustCompile(`(\w+)=("([^"]*)"|[^,\s]+)`)

// digestHeader builds an RFC 7616 (MD5, qop=auth) Authorization header.
func digestHeader(challenge, method, uri, user, pass string) string {
	p := map[string]string{}
	for _, m := range digestParamRe.FindAllStringSubmatch(strings.TrimPrefix(challenge, "Digest "), -1) {
		v := m[2]
		if m[3] != "" || strings.HasPrefix(v, `"`) {
			v = m[3]
		}
		p[strings.ToLower(m[1])] = v
	}
	h := func(s string) string { sum := md5.Sum([]byte(s)); return hex.EncodeToString(sum[:]) }
	ha1 := h(user + ":" + p["realm"] + ":" + pass)
	ha2 := h(method + ":" + uri)
	cnonceB := make([]byte, 8)
	rand.Read(cnonceB)
	cnonce := hex.EncodeToString(cnonceB)
	nc := "00000001"
	var resp string
	qop := ""
	if strings.Contains(p["qop"], "auth") {
		qop = "auth"
		resp = h(ha1 + ":" + p["nonce"] + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		resp = h(ha1 + ":" + p["nonce"] + ":" + ha2)
	}
	out := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`, user, p["realm"], p["nonce"], uri, resp)
	if qop != "" {
		out += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}
	if p["opaque"] != "" {
		out += fmt.Sprintf(`, opaque="%s"`, p["opaque"])
	}
	return out + `, algorithm=MD5`
}

// --- Collections ---

func (h *PlaygroundHandlers) ListCollections(w http.ResponseWriter, r *http.Request) {
	result, err := h.collections.List()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type collectionRequest struct {
	Name     string            `json:"name"`
	Requests []db.SavedRequest `json:"requests"`
}

func (h *PlaygroundHandlers) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var req collectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Collection name is required", nil)
		return
	}
	col, err := h.collections.Create(req.Name, req.Requests)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (h *PlaygroundHandlers) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	var req collectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Collection name is required", nil)
		return
	}
	col, err := h.collections.Update(r.PathValue("id"), req.Name, req.Requests)
	if err != nil {
		internalError(w, err)
		return
	}
	if col == nil {
		notFound(w, "COLLECTION", r.PathValue("id"))
		return
	}
	writeJSON(w, http.StatusOK, col)
}

func (h *PlaygroundHandlers) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.collections.Delete(r.PathValue("id")); err != nil {
		notFound(w, "COLLECTION", r.PathValue("id"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// base64URL is unpadded base64url (PKCE, state).
func base64URL(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
