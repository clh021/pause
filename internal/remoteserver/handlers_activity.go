package remoteserver

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Default: last 24 hours
	to := time.Now().Unix()
	from := to - 86400

	if f := r.URL.Query().Get("from"); f != "" {
		if v, err := strconv.ParseInt(f, 10, 64); err == nil {
			from = v
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil {
			to = v
		}
	}

	if s.activity == nil {
		writeJSON(w, http.StatusOK, ActivitySummary{
			Ticks: []ActivityRecord{},
			Shots: []ShotInfo{},
		})
		return
	}

	summary, err := s.activity.GetActivity(r.Context(), from, to)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleScreenshotList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.screenshots == nil {
		writeJSON(w, http.StatusOK, []ShotInfo{})
		return
	}

	to := time.Now().Unix()
	from := to - 86400

	if f := r.URL.Query().Get("from"); f != "" {
		if v, err := strconv.ParseInt(f, 10, 64); err == nil {
			from = v
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil {
			to = v
		}
	}

	shots := screenshotList(s.screenshotDir(), from, to)
	writeJSON(w, http.StatusOK, shots)
}

// handleServeShot serves a screenshot JPEG from the screenshots directory.
func (s *Server) handleServeShot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	name := suffixPathValue(r.URL.Path, "/shots/")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing shot name")
		return
	}

	// Prevent path traversal: only allow filenames matching shot-*.jpg
	if !strings.HasPrefix(name, "shot-") || !strings.HasSuffix(name, ".jpg") || strings.Contains(name, "/") || strings.Contains(name, "..") {
		writeJSONError(w, http.StatusBadRequest, "invalid shot name")
		return
	}

	dir := s.screenshotDir()
	if dir == "" {
		writeJSONError(w, http.StatusInternalServerError, "cannot resolve screenshot directory")
		return
	}
	path := filepath.Join(dir, name)

	http.ServeFile(w, r, path)
}

func (s *Server) handleAutoScreenshotSetting(w http.ResponseWriter, r *http.Request) {
	if s.activity == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "activity recorder not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.activity.CaptureOnActivity()})

	case http.MethodPost:
		enabled := r.URL.Query().Get("enabled")
		if enabled == "" {
			// Try reading from body
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := decodeJSONBody(r, &body); err == nil {
				s.activity.SetCaptureOnActivity(body.Enabled)
				writeJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
				return
			}
			writeJSONError(w, http.StatusBadRequest, "missing enabled field")
			return
		}
		s.activity.SetCaptureOnActivity(enabled == "true" || enabled == "1")
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.activity.CaptureOnActivity()})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func screenshotList(dir string, fromT, toT int64) []ShotInfo {
	return listShotsInDir(dir, fromT, toT)
}
