package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
