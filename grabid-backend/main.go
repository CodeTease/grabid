package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	Port       string
	GrabSecret string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return Config{
		Port:       port,
		GrabSecret: os.Getenv("GRAB_SECRET"),
	}
}

// ProbeResponse defines the JSON structure for the /probe endpoint
type ProbeResponse struct {
	Size int64  `json:"size"`
	Type string `json:"type"`
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
func handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	// Use a client with no timeout for streaming large files, or a very long one
	// Default client has no timeout, which is good for streaming but bad for hanging connections.
	// We'll trust the TCP keep-alive and user cancellation.
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

	// Header Butler: Forward Content-Length, Content-Type, Content-Disposition
	if resp.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if contentDisposition := resp.Header.Get("Content-Disposition"); contentDisposition != "" {
		w.Header().Set("Content-Disposition", contentDisposition)
	} else {
		// Try to derive filename from URL if not provided
		// Basic fallback, can be improved
		parts := strings.Split(urlParam, "/")
		if len(parts) > 0 {
			filename := parts[len(parts)-1]
			if filename != "" {
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
			}
		}
	}

	// Streamer Engine: io.Copy
	// This handles the backpressure automatically via TCP flow control
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		// Cannot write http error here because headers are already sent
		log.Printf("Error streaming data: %v", err)
	}
}

func main() {
	cfg := LoadConfig()

	mux := http.NewServeMux()

	// Register endpoints with Auth Middleware
	mux.HandleFunc("/api/v1/probe", AuthMiddleware(handleProbe, cfg.GrabSecret))
	mux.HandleFunc("/api/v1/stream", AuthMiddleware(handleStream, cfg.GrabSecret))

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
