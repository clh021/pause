package remoteserver

import (
	"net/http"

	"pause/internal/backend/ports"
)

func (s *Server) handleNotificationCapability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cap := s.services.NotificationCapabilityProvider.GetNotificationCapability()
	writeJSON(w, http.StatusOK, notificationCapabilityToDTO(cap))
}

func (s *Server) handleRequestNotificationPermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cap, err := s.services.NotificationCapabilityProvider.RequestNotificationPermission()
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notificationCapabilityToDTO(cap))
}

func (s *Server) handleOpenNotificationSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.services.NotificationCapabilityProvider.OpenNotificationSettings(); err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notificationCapabilityToDTO(cap ports.NotificationCapability) NotificationCapability {
	return NotificationCapability{
		PermissionState: string(cap.PermissionState),
		CanRequest:      cap.CanRequest,
		CanOpenSettings: cap.CanOpenSettings,
		Reason:          cap.Reason,
	}
}
