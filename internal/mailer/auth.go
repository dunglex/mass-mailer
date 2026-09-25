package mailer

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"sync"
)

var (
	authUser     = os.Getenv("AUTH_USER")
	authPassword = os.Getenv("AUTH_PASSWORD")
	warnOnce     sync.Once
)

// compareCredentials performs constant-time comparison of provided credentials
// against configured credentials using SHA256 digests.
// Returns true only if both username and password match exactly.
func compareCredentials(user, pass, configUser, configPass string) bool {
	// Constant-time comparison of SHA256 digests
	userHash := sha256.Sum256([]byte(user))
	configUserHash := sha256.Sum256([]byte(configUser))
	userMatch := subtle.ConstantTimeCompare(userHash[:], configUserHash[:])

	passHash := sha256.Sum256([]byte(pass))
	configPassHash := sha256.Sum256([]byte(configPass))
	passMatch := subtle.ConstantTimeCompare(passHash[:], configPassHash[:])

	// Both must match; no short-circuit
	return userMatch == 1 && passMatch == 1
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no credentials configured, pass through
		if authUser == "" && authPassword == "" {
			next(w, r)
			return
		}

		// Extract credentials from request
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Mass Mailer", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify credentials
		if !compareCredentials(user, pass, authUser, authPassword) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Mass Mailer", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Credentials valid
		next(w, r)
	}
}

func warnIfUnauthenticated() {
	if authUser == "" && authPassword == "" {
		warnOnce.Do(func() {
			log.Printf("WARNING: AUTH_USER/AUTH_PASSWORD unset — Mass Mailer is serving with no authentication")
		})
	}
}
