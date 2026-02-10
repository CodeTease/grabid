package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Config holds the application configuration
type Config struct {
	Port          string
	GrabSecret    string
	MaxSize       int64
	MaxSizeStr    string
	MaxConcurrent int
	RateLimit     rate.Limit
	RateBurst     int
}

// ParseSize parses a size string (e.g., "1GB", "500MB") into bytes.
func ParseSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))
	if sizeStr == "" {
		return 1024 * 1024 * 1024 // Default 1GB
	}
	var multiplier int64 = 1
	if strings.HasSuffix(sizeStr, "GB") {
		multiplier = 1024 * 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "GB")
	} else if strings.HasSuffix(sizeStr, "MB") {
		multiplier = 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "MB")
	} else if strings.HasSuffix(sizeStr, "KB") {
		multiplier = 1024
		sizeStr = strings.TrimSuffix(sizeStr, "KB")
	} else if strings.HasSuffix(sizeStr, "B") {
		sizeStr = strings.TrimSuffix(sizeStr, "B")
	}

	val, err := strconv.ParseInt(strings.TrimSpace(sizeStr), 10, 64)
	if err != nil {
		return 1024 * 1024 * 1024 // Default 1GB
	}
	return val * multiplier
}

// ParseRateLimit parses a rate limit string (e.g., "1-5") into rate and burst.
func ParseRateLimit(rateStr string) (rate.Limit, int) {
	rateStr = strings.TrimSpace(rateStr)
	parts := strings.Split(rateStr, "-")
	if len(parts) != 2 {
		return 1, 5 // Default
	}
	r, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 1, 5 // Default
	}
	return rate.Limit(r), b
}

// LoadConfig loads configuration from environment variables
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	maxSizeStr := os.Getenv("GRAB_MAX_SIZE")
	if maxSizeStr == "" {
		maxSizeStr = "1GB"
	}
	maxConcurrentStr := os.Getenv("GRAB_MAX_CONCURRENT")
	maxConcurrent := 5
	if maxConcurrentStr != "" {
		if val, err := strconv.Atoi(maxConcurrentStr); err == nil {
			maxConcurrent = val
		}
	}
	rateLimitStr := os.Getenv("GRAB_RATE_LIMIT")
	if rateLimitStr == "" {
		rateLimitStr = "1-5"
	}
	r, b := ParseRateLimit(rateLimitStr)

	return Config{
		Port:          port,
		GrabSecret:    os.Getenv("GRAB_SECRET"),
		MaxSize:       ParseSize(maxSizeStr),
		MaxSizeStr:    maxSizeStr,
		MaxConcurrent: maxConcurrent,
		RateLimit:     r,
		RateBurst:     b,
	}
}

// ProbeResponse defines the JSON structure for the /probe endpoint
type ProbeResponse struct {
	Size int64  `json:"size"`
	Type string `json:"type"`
}

// InfoResponse defines the JSON structure for the /info endpoint
type InfoResponse struct {
	MaxSizeStr    string `json:"max_size_limit"`
	MaxConcurrent int    `json:"max_concurrent"`
}

// AuthMiddleware checks for the X-Grab-Token header
func AuthMiddleware(next http.HandlerFunc, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If GRAB_SECRET is empty, it's public mode
		if secret == "" {
			next(w, r)
			return
		}

		token := r.Header.Get("X-Grab-Token")
		if token != secret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages rate limiters per IP with cleanup
type IPRateLimiter struct {
	mu    sync.Mutex
	ips   map[string]*clientLimiter
	limit rate.Limit
	burst int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips:   make(map[string]*clientLimiter),
		limit: r,
		burst: b,
	}

	// Start cleanup goroutine
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			i.Cleanup()
		}
	}()

	return i
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = &clientLimiter{
			limiter: rate.NewLimiter(i.limit, i.burst),
		}
		i.ips[ip] = limiter
	}
	limiter.lastSeen = time.Now()
	return limiter.limiter
}

func (i *IPRateLimiter) Cleanup() {
	i.mu.Lock()
	defer i.mu.Unlock()
	for ip, limiter := range i.ips {
		if time.Since(limiter.lastSeen) > 3*time.Minute {
			delete(i.ips, ip)
		}
	}
}

// RateLimitMiddleware enforces rate limits per IP
func RateLimitMiddleware(next http.HandlerFunc, limiter *IPRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Handle X-Forwarded-For if behind proxy
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = forwarded
		}
		// Remove port
		if strings.Contains(ip, ":") {
			host, _, err := net.SplitHostPort(ip)
			if err == nil {
				ip = host
			}
		}

		if !limiter.GetLimiter(ip).Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// handleProbe handles the HEAD /api/v1/probe endpoint
func handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlParam := r.URL.Query().Get("url")
	if urlParam == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	// Create a HEAD request to the source
	req, err := http.NewRequest(http.MethodHead, urlParam, nil)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to reach source: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("Source returned error: %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	// Prepare JSON response
	probeResp := ProbeResponse{
		Size: resp.ContentLength,
		Type: resp.Header.Get("Content-Type"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(probeResp); err != nil {
		log.Printf("Error encoding probe response: %v", err)
	}
}

// handleStream handles the GET /api/v1/stream endpoint
func handleStream(w http.ResponseWriter, r *http.Request, cfg Config, sem chan struct{}) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Concurrency Check
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	default:
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	urlParam := r.URL.Query().Get("url")
	if urlParam == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	// Create a GET request to the source
	req, err := http.NewRequest(http.MethodGet, urlParam, nil)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to reach source: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("Source returned error: %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	// Size Check (Header)
	if resp.ContentLength > cfg.MaxSize {
		http.Error(w, "Payload Too Large", http.StatusRequestEntityTooLarge)
		return
	}

	// Header Butler
	if resp.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if contentDisposition := resp.Header.Get("Content-Disposition"); contentDisposition != "" {
		w.Header().Set("Content-Disposition", contentDisposition)
	} else {
		parts := strings.Split(urlParam, "/")
		if len(parts) > 0 {
			filename := parts[len(parts)-1]
			if filename != "" {
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
			}
		}
	}

	// Streamer Engine with LimitReader
	limitedBody := io.LimitReader(resp.Body, cfg.MaxSize)
	_, err = io.Copy(w, limitedBody)
	if err != nil {
		// If io.Copy fails, it might be due to limit reached or connection error.
		// We can't really change status code here.
		log.Printf("Error streaming data: %v", err)
	}
}

func main() {
	cfg := LoadConfig()

	// Initialize concurrency semaphore
	concurrencySem := make(chan struct{}, cfg.MaxConcurrent)

	// Initialize Rate Limiter
	ipLimiter := NewIPRateLimiter(cfg.RateLimit, cfg.RateBurst)

	mux := http.NewServeMux()

	// Register endpoints with Auth Middleware
	mux.HandleFunc("/api/v1/probe", AuthMiddleware(handleProbe, cfg.GrabSecret))
	
	// Stream handler with Rate Limit and Concurrency Control
	mux.HandleFunc("/api/v1/stream", AuthMiddleware(RateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handleStream(w, r, cfg, concurrencySem)
	}, ipLimiter), cfg.GrabSecret))

	mux.HandleFunc("/api/v1/info", AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		resp := InfoResponse{
			MaxSizeStr:    cfg.MaxSizeStr,
			MaxConcurrent: cfg.MaxConcurrent,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}, cfg.GrabSecret))

	log.Printf("Starting grabid-backend on port %s", cfg.Port)
	if cfg.GrabSecret == "" {
		log.Println("Running in PUBLIC mode (no authentication required)")
	} else {
		log.Println("Running in SECURE mode (authentication enabled)")
	}

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
