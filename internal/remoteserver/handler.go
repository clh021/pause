package remoteserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"pause/internal/backend/bootstrap"
	"pause/internal/backend/runtime/state"
	"pause/internal/logx"
)

const remoteTriggerCooldownReason = "trigger cooldown active"

type triggerBreakRequest struct {
	Reason   string `json:"reason"`
	BreakSec int    `json:"breakSec"`
}

type forceBreakRequest struct {
	Minutes  int `json:"minutes"`
	BreakSec int `json:"breakSec"`
}

type statusReminder struct {
	ID      int64  `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
}

type statusResponse struct {
	Status        string           `json:"status"`
	RemainingSec  int              `json:"remainingSec"`
	TotalBreakSec int              `json:"totalBreakSec"`
	Reminders     []statusReminder `json:"reminders"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, newStatusResponse(s.services.Engine.GetRuntimeState(s.now())))
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeRuntimeState(w, http.StatusOK, s.services.Engine.Pause(s.now()), r.Context())
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeRuntimeState(w, http.StatusOK, s.services.Engine.Resume(s.now()), r.Context())
}

func (s *Server) handleSkipBreak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	state, err := s.services.Engine.SkipCurrentBreak(s.now(), bootstrap.SkipModeNormal)
	if err != nil {
		writeServerError(w, err)
		return
	}
	s.writeRuntimeState(w, http.StatusOK, state, r.Context())
}

func (s *Server) handlePostponeBreak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	state, err := s.services.Engine.PostponeCurrentBreak(s.now())
	if err != nil {
		writeServerError(w, err)
		return
	}
	s.writeRuntimeState(w, http.StatusOK, state, r.Context())
}

func (s *Server) handleTriggerBreak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	req := triggerBreakRequest{}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := s.now()
	if err := s.checkTriggerCooldown(now); err != nil {
		writeServerError(w, err)
		return
	}

	state, err := s.services.Engine.StartBreakNow(now)
	if err != nil {
		writeServerError(w, err)
		return
	}
	s.recordTrigger(now)
	logx.Infof("remote.trigger_break reason=%s requested_break_sec=%d", req.Reason, req.BreakSec)
	writeJSON(w, http.StatusOK, newStatusResponse(state))
}

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	restore, err := s.prepareScreenshot(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if restore != nil {
		defer restore()
		time.Sleep(150 * time.Millisecond)
	}

	result, err := s.screenshots.Capture(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if dir := s.screenshotDir(); dir != "" {
		if _, err := writeMinuteShot(dir, result.PNG, s.now()); err != nil {
			logx.Warnf("remote.screenshot.minute_shot_err err=%v", err)
		}
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.PNG)
}

func newStatusResponse(runtimeState state.RuntimeState) statusResponse {
	resp := statusResponse{
		Status:    "idle",
		Reminders: make([]statusReminder, 0, len(runtimeState.Reminders)),
	}
	if !runtimeState.GlobalEnabled {
		resp.Status = "paused"
	}
	if runtimeState.CurrentSession != nil {
		resp.Status = "resting"
		resp.RemainingSec = runtimeState.CurrentSession.RemainingSec
		resp.TotalBreakSec = int(runtimeState.CurrentSession.EndsAt.Sub(runtimeState.CurrentSession.StartedAt).Seconds())
	}
	for _, reminder := range runtimeState.Reminders {
		resp.Reminders = append(resp.Reminders, statusReminder{
			ID:      reminder.ID,
			Name:    reminder.Name,
			Enabled: reminder.Enabled,
		})
	}
	return resp
}

func (s *Server) checkTriggerCooldown(now time.Time) error {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	if s.lastTriggerAt.IsZero() {
		return nil
	}
	if now.Sub(s.lastTriggerAt) < s.triggerCooldown {
		return errors.New(remoteTriggerCooldownReason)
	}
	return nil
}

func (s *Server) recordTrigger(now time.Time) {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	s.lastTriggerAt = now
}

func (s *Server) prepareScreenshot(ctx context.Context) (func(), error) {
	if s.beforeScreenshot == nil {
		return nil, nil
	}
	return s.beforeScreenshot(ctx)
}

func (s *Server) handleForceUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	state, err := s.services.Engine.SkipCurrentBreak(s.now(), bootstrap.SkipModeEmergency)
	if err != nil {
		writeServerError(w, err)
		return
	}
	s.writeRuntimeState(w, http.StatusOK, state, r.Context())
}

func (s *Server) handleForceBreak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	req := forceBreakRequest{}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	breakSec := req.BreakSec
	if breakSec <= 0 && req.Minutes > 0 {
		breakSec = req.Minutes * 60
	}

	var (
		resp RuntimeState
		err  error
	)
	if breakSec > 0 {
		var runtimeState state.RuntimeState
		runtimeState, err = s.services.Engine.StartCustomBreakNow(s.now(), breakSec)
		if err == nil {
			resp = runtimeStateToDTO(runtimeState, s.currentSettings(r.Context()))
		}
	} else {
		var runtimeState state.RuntimeState
		runtimeState, err = s.services.Engine.StartBreakNow(s.now())
		if err == nil {
			resp = runtimeStateToDTO(runtimeState, s.currentSettings(r.Context()))
		}
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.services.Quit != nil {
		go s.services.Quit()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRuntimeState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeRuntimeState(w, http.StatusOK, s.services.Engine.GetRuntimeState(s.now()), r.Context())
}

func decodeJSONBody(r *http.Request, dest any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func suffixPathValue(requestPath string, prefix string) string {
	if !strings.HasPrefix(requestPath, prefix) {
		return ""
	}
	value := strings.TrimPrefix(requestPath, prefix)
	if value == "" {
		return ""
	}
	return path.Clean("/" + value)[1:]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeServerError(w http.ResponseWriter, err error) {
	if err == nil {
		writeJSONError(w, http.StatusInternalServerError, "unknown error")
		return
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "break already active"):
		writeJSONError(w, http.StatusConflict, message)
	case strings.Contains(message, remoteTriggerCooldownReason):
		writeJSONError(w, http.StatusConflict, message)
	case strings.Contains(message, "no active break"):
		writeJSONError(w, http.StatusConflict, message)
	case strings.Contains(message, "skip is disabled"):
		writeJSONError(w, http.StatusConflict, message)
	case strings.Contains(message, "global reminders are disabled"):
		writeJSONError(w, http.StatusConflict, message)
	case strings.Contains(message, "no enabled reminder rules"):
		writeJSONError(w, http.StatusConflict, message)
	default:
		writeJSONError(w, http.StatusInternalServerError, message)
	}
}
