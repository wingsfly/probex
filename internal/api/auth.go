package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// tokenTTL is how long a login token stays valid.
const tokenTTL = 7 * 24 * time.Hour

// makeToken builds a stateless bearer token "<exp>.<sig>" where sig is
// HMAC-SHA256(password, exp). It survives restarts and needs no session store.
func makeToken(password string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(password))
	fmt.Fprintf(mac, "%d", exp)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%d.%s", exp, sig)
}

// verifyToken checks signature and expiry in constant time.
func verifyToken(password, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	expected := makeToken(password, exp)
	return hmac.Equal([]byte(token), []byte(expected))
}

// login handles POST /api/v1/login: {"password":"..."} -> {"token":"..."}.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.authPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	exp := time.Now().Add(tokenTTL).Unix()
	writeData(w, map[string]string{"token": makeToken(s.authPassword, exp)})
}

// authMiddleware rejects requests without a valid bearer token. The public
// endpoints /api/v1/mode and /api/v1/login pass through untouched.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/mode", "/api/v1/login":
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions { // let CORS preflight through
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || !verifyToken(s.authPassword, token) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
