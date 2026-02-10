package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestAuthMiddleware(t *testing.T) {
	// Mock handler
	nextHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	tests := []struct {
		name           string
		secret         string
		tokenHeader    string
		expectedStatus int
	}{
		{
			name:           "Public Mode (No Secret)",
			secret:         "",
			tokenHeader:    "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Secure Mode - Correct Token",
			secret:         "mysecret",
			tokenHeader:    "mysecret",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Secure Mode - Incorrect Token",
			secret:         "mysecret",
			tokenHeader:    "wrong",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Secure Mode - No Token",
			secret:         "mysecret",
			tokenHeader:    "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.tokenHeader != "" {
				req.Header.Set("X-Grab-Token", tt.tokenHeader)
			}

			w := httptest.NewRecorder()
			handler := AuthMiddleware(nextHandler, tt.secret)
			handler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1GB", 1024 * 1024 * 1024},
		{"500MB", 500 * 1024 * 1024},
		{"1KB", 1024},
		{"1024B", 1024},
		{"100", 100}, // Defaults to bytes if no suffix? My implementation treats it as bytes if just number, or maybe not.
		// Let's check logic: if no suffix matches, it tries ParseInt on the string.
		{"", 1024 * 1024 * 1024}, // Default
		{"INVALID", 1024 * 1024 * 1024}, // Default on error
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseSize(tt.input)
			if got != tt.expected {
				t.Errorf("ParseSize(%q) = %d; want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseRateLimit(t *testing.T) {
	tests := []struct {
		input         string
		expectedLimit rate.Limit
		expectedBurst int
	}{
		{"1-5", 1, 5},
		{"10-20", 10, 20},
		{"invalid", 1, 5}, // Default
		{"", 1, 5}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l, b := ParseRateLimit(tt.input)
			if l != tt.expectedLimit || b != tt.expectedBurst {
				t.Errorf("ParseRateLimit(%q) = %v, %d; want %v, %d", tt.input, l, b, tt.expectedLimit, tt.expectedBurst)
			}
		})
	}
}
