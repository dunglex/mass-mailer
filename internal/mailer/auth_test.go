package mailer

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareCredentials(t *testing.T) {
	tests := []struct {
		name          string
		user          string
		pass          string
		configUser    string
		configPass    string
		expectedMatch bool
	}{
		{
			name:          "correct credentials",
			user:          "admin",
			pass:          "secret123",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: true,
		},
		{
			name:          "wrong password",
			user:          "admin",
			pass:          "wrongpass",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: false,
		},
		{
			name:          "wrong username",
			user:          "wronguser",
			pass:          "secret123",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: false,
		},
		{
			name:          "both wrong",
			user:          "wronguser",
			pass:          "wrongpass",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: false,
		},
		{
			name:          "empty credentials against config",
			user:          "",
			pass:          "",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: false,
		},
		{
			name:          "case sensitive password",
			user:          "admin",
			pass:          "Secret123",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: false,
		},
		{
			name:          "case sensitive username",
			user:          "Admin",
			pass:          "secret123",
			configUser:    "admin",
			configPass:    "secret123",
			expectedMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareCredentials(tt.user, tt.pass, tt.configUser, tt.configPass)
			if got != tt.expectedMatch {
				t.Errorf("compareCredentials(%q, %q, %q, %q) = %v, want %v",
					tt.user, tt.pass, tt.configUser, tt.configPass, got, tt.expectedMatch)
			}
		})
	}
}

func TestRequireAuthNoConfigured(t *testing.T) {
	// Test when no credentials are configured (authUser and authPassword are empty)
	// In this case, the middleware should pass through without requiring auth

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	}

	// When no auth is configured, requireAuth should pass through
	middleware := requireAuth(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	middleware(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "Success" {
		t.Errorf("Expected body 'Success', got %q", rec.Body.String())
	}
}

func TestRequireAuthWithConfigured(t *testing.T) {
	tests := []struct {
		name            string
		user            string
		pass            string
		sendAuth        bool
		expectedStatus  int
		expectedWWWAuth bool
		shouldCallNext  bool
	}{
		{
			name:            "no authorization header",
			user:            "",
			pass:            "",
			sendAuth:        false,
			expectedStatus:  http.StatusUnauthorized,
			expectedWWWAuth: true,
			shouldCallNext:  false,
		},
		{
			name:            "correct credentials",
			user:            "testuser",
			pass:            "testpass",
			sendAuth:        true,
			expectedStatus:  http.StatusOK,
			expectedWWWAuth: false,
			shouldCallNext:  true,
		},
		{
			name:            "wrong password",
			user:            "testuser",
			pass:            "wrongpass",
			sendAuth:        true,
			expectedStatus:  http.StatusUnauthorized,
			expectedWWWAuth: true,
			shouldCallNext:  false,
		},
		{
			name:            "wrong username",
			user:            "wronguser",
			pass:            "testpass",
			sendAuth:        true,
			expectedStatus:  http.StatusUnauthorized,
			expectedWWWAuth: true,
			shouldCallNext:  false,
		},
	}

	// Save original values and restore after test
	origUser := authUser
	origPass := authPassword
	defer func() {
		authUser = origUser
		authPassword = origPass
	}()

	// Configure auth
	authUser = "testuser"
	authPassword = "testpass"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			handler := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Success"))
			}

			middleware := requireAuth(handler)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)

			if tt.sendAuth {
				// Construct Basic Auth header
				credentials := base64.StdEncoding.EncodeToString([]byte(tt.user + ":" + tt.pass))
				req.Header.Set("Authorization", "Basic "+credentials)
			}

			middleware(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedWWWAuth {
				auth := rec.Header().Get("WWW-Authenticate")
				if auth == "" {
					t.Errorf("Expected WWW-Authenticate header, got empty")
				} else if auth != `Basic realm="Mass Mailer", charset="UTF-8"` {
					t.Errorf("Expected WWW-Authenticate header %q, got %q",
						`Basic realm="Mass Mailer", charset="UTF-8"`, auth)
				}
			}

			if nextCalled != tt.shouldCallNext {
				t.Errorf("Expected nextCalled=%v, got %v", tt.shouldCallNext, nextCalled)
			}
		})
	}
}
