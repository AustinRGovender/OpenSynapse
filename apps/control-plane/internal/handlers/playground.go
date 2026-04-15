package handlers

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type PlaygroundHandlers struct {
	collections *db.CollectionStore
}

func NewPlaygroundHandlers(collections *db.CollectionStore) *PlaygroundHandlers {
	return &PlaygroundHandlers{collections: collections}
}

// PlaygroundRequest is the request spec from the UI.
type PlaygroundRequest struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	BodyType   string            `json:"body_type"`
	AuthType   string            `json:"auth_type"`
	AuthConfig json.RawMessage   `json:"auth_config"`
}

// PlaygroundResponse is what we return to the UI.
type PlaygroundResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	BodySize   int               `json:"body_size"`
	Timing     TimingBreakdown   `json:"timing"`
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

// ExecuteRequest handles POST /api/v1/playground/request
func (h *PlaygroundHandlers) ExecuteRequest(w http.ResponseWriter, r *http.Request) {
	var req PlaygroundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.URL == "" {
		badRequest(w, "VALIDATION_ERROR", "URL is required", nil)
		return
	}
	if req.Method == "" {
		req.Method = "GET"
	}

	// Build the HTTP request
	var bodyReader io.Reader
	if req.Body != "" && req.Method != "GET" && req.Method != "HEAD" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, bodyReader)
	if err != nil {
		badRequest(w, "INVALID_REQUEST", fmt.Sprintf("Cannot create request: %s", err), nil)
		return
	}

	// Set headers
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Apply auth
	applyAuth(httpReq, req.AuthType, req.AuthConfig)

	// Set content type for body
	if req.BodyType == "json" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	} else if req.BodyType == "form" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// Timing instrumentation via httptrace
	var dnsStart, dnsEnd time.Time
	var connStart, connEnd time.Time
	var tlsStart, tlsEnd time.Time
	var gotFirstByte time.Time
	requestStart := time.Now()

	trace := &httptrace.ClientTrace{
		DNSStart:             func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(_ httptrace.DNSDoneInfo) { dnsEnd = time.Now() },
		ConnectStart:         func(_, _ string) { connStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { connEnd = time.Now() },
		TLSHandshakeStart:   func() { tlsStart = time.Now() },
		TLSHandshakeDone:    func(_ tls.ConnectionState, _ error) { tlsEnd = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), trace))

	// Execute
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
		writeError(w, http.StatusBadGateway, "REQUEST_FAILED", fmt.Sprintf("Request failed: %s", err), nil)
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	requestEnd := time.Now()

	if err != nil {
		writeError(w, http.StatusBadGateway, "RESPONSE_READ_FAILED", fmt.Sprintf("Failed to read response: %s", err), nil)
		return
	}

	// Compute timing
	timing := TimingBreakdown{
		TotalMS: float64(requestEnd.Sub(requestStart).Microseconds()) / 1000.0,
	}
	if !dnsStart.IsZero() && !dnsEnd.IsZero() {
		timing.DNSMS = float64(dnsEnd.Sub(dnsStart).Microseconds()) / 1000.0
	}
	if !connStart.IsZero() && !connEnd.IsZero() {
		timing.ConnectMS = float64(connEnd.Sub(connStart).Microseconds()) / 1000.0
	}
	if !tlsStart.IsZero() && !tlsEnd.IsZero() {
		timing.TLSMS = float64(tlsEnd.Sub(tlsStart).Microseconds()) / 1000.0
	}
	if !gotFirstByte.IsZero() {
		// Wait = time from request sent to first byte
		sendEnd := connEnd
		if !tlsEnd.IsZero() {
			sendEnd = tlsEnd
		}
		if !sendEnd.IsZero() {
			timing.WaitMS = float64(gotFirstByte.Sub(sendEnd).Microseconds()) / 1000.0
		}
		timing.ReceiveMS = float64(requestEnd.Sub(gotFirstByte).Microseconds()) / 1000.0
	}
	timing.SendMS = timing.TotalMS - timing.DNSMS - timing.ConnectMS - timing.TLSMS - timing.WaitMS - timing.ReceiveMS
	if timing.SendMS < 0 {
		timing.SendMS = 0
	}

	// Build response headers map
	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		respHeaders[k] = strings.Join(v, ", ")
	}

	result := PlaygroundResponse{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Headers:    respHeaders,
		Body:       string(respBody),
		BodySize:   len(respBody),
		Timing:     timing,
	}

	writeJSON(w, http.StatusOK, result)
}

func applyAuth(req *http.Request, authType string, authConfig json.RawMessage) {
	switch authType {
	case "basic":
		var cfg struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		json.Unmarshal(authConfig, &cfg)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
			[]byte(cfg.Username+":"+cfg.Password)))

	case "bearer":
		var cfg struct {
			Token string `json:"token"`
		}
		json.Unmarshal(authConfig, &cfg)
		req.Header.Set("Authorization", "Bearer "+cfg.Token)

	case "api_key":
		var cfg struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			AddTo string `json:"add_to"` // "header" or "query"
		}
		json.Unmarshal(authConfig, &cfg)
		if cfg.AddTo == "query" {
			q := req.URL.Query()
			q.Set(cfg.Key, cfg.Value)
			req.URL.RawQuery = q.Encode()
		} else {
			req.Header.Set(cfg.Key, cfg.Value)
		}
	}
}

// Collections CRUD

func (h *PlaygroundHandlers) ListCollections(w http.ResponseWriter, r *http.Request) {
	result, err := h.collections.List()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *PlaygroundHandlers) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string             `json:"name"`
		Requests []db.SavedRequest  `json:"requests"`
	}
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
