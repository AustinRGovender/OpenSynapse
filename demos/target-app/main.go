// Demo target application for OpenSynapse.
// A minimal HTTP server with several endpoints suitable for load testing.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- data ----------

type Product struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Category    string  `json:"category"`
	InStock     bool    `json:"in_stock"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	CSRFToken string `json:"csrf_token"`
	ExpiresIn int    `json:"expires_in"`
}

var products = []Product{
	{ID: 1, Name: "Load Tester Pro", Description: "Professional load testing toolkit", Price: 299.99, Category: "software", InStock: true},
	{ID: 2, Name: "Monitoring Dashboard", Description: "Real-time application monitoring", Price: 149.99, Category: "software", InStock: true},
	{ID: 3, Name: "API Gateway", Description: "High-performance API gateway appliance", Price: 999.99, Category: "hardware", InStock: false},
	{ID: 4, Name: "Stress Test Kit", Description: "Hardware stress testing equipment", Price: 449.99, Category: "hardware", InStock: true},
	{ID: 5, Name: "Performance Analyzer", Description: "Automated performance analysis tool", Price: 199.99, Category: "software", InStock: true},
	{ID: 6, Name: "Network Simulator", Description: "Simulate various network conditions", Price: 349.99, Category: "software", InStock: true},
	{ID: 7, Name: "Chaos Monkey", Description: "Controlled failure injection system", Price: 79.99, Category: "software", InStock: true},
	{ID: 8, Name: "Capacity Planner", Description: "Infrastructure capacity planning suite", Price: 599.99, Category: "software", InStock: false},
	{ID: 9, Name: "Benchmark Rig", Description: "Standardised benchmarking hardware", Price: 1299.99, Category: "hardware", InStock: true},
	{ID: 10, Name: "Latency Probe", Description: "Sub-millisecond latency measurement device", Price: 249.99, Category: "hardware", InStock: true},
}

// ---------- token store (in-memory, demo only) ----------

var (
	tokensMu sync.RWMutex
	tokens   = map[string]string{} // token -> username
)

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func generateCSRFToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("csrf-fallback-%d", time.Now().UnixNano())
	}
	return "csrf_" + hex.EncodeToString(b)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// ---------- handlers ----------

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func handleProductByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract ID from path: /api/products/{id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/products/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "missing product id")
		return
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	for _, p := range products {
		if p.ID == id {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}

	writeError(w, http.StatusNotFound, "product not found")
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}

	// Demo: accept any non-empty credentials
	token := generateToken()
	csrfToken := generateCSRFToken()

	tokensMu.Lock()
	tokens[token] = req.Username
	tokensMu.Unlock()

	writeJSON(w, http.StatusOK, LoginResponse{
		Token:     token,
		CSRFToken: csrfToken,
		ExpiresIn: 3600,
	})
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"query":   "",
			"results": []Product{},
			"total":   0,
		})
		return
	}

	var results []Product
	for _, p := range products {
		if strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.Description), query) ||
			strings.Contains(strings.ToLower(p.Category), query) {
			results = append(results, p)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"results": results,
		"total":   len(results),
	})
}

// ---------- router ----------

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/products", handleProducts)
	mux.HandleFunc("/api/products/", handleProductByID)
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/search", handleSearch)
	return mux
}

// ---------- main ----------

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	mux := newMux()

	log.Printf("Demo target app listening on :%s", port)
	log.Printf("Endpoints:")
	log.Printf("  GET  /health")
	log.Printf("  GET  /api/products")
	log.Printf("  GET  /api/products/:id")
	log.Printf("  POST /api/login")
	log.Printf("  GET  /api/search?q=...")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
