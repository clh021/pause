package remoteserver

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const remoteAuthCookieName = "pause_remote_auth"

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if s.cfg.Token == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.isAuthorized(r) {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if strings.TrimSpace(s.cfg.Token) == "" {
			writeJSONError(w, http.StatusServiceUnavailable, "remote auth is unavailable")
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !tokensEqual(body.Token, s.cfg.Token) {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     remoteAuthCookieName,
			Value:    s.cfg.Token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   30 * 24 * 60 * 60,
		})
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
	case http.MethodDelete:
		http.SetCookie(w, &http.Cookie{
			Name:     remoteAuthCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) isAuthorized(r *http.Request) bool {
	return tokensEqual(s.requestToken(r), s.cfg.Token)
}

func (s *Server) requestToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	if cookie, err := r.Cookie(remoteAuthCookieName); err == nil {
		if token := strings.TrimSpace(cookie.Value); token != "" {
			return token
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func tokensEqual(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
