package remoteserver

import (
	"net/http"
	"strconv"

	reminderdomain "pause/internal/backend/domain/reminder"
	"pause/internal/logx"
)

func (s *Server) handleListReminders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	reminders, err := s.services.ReminderService.List(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, remindersToDTOs(reminders))
}

func (s *Server) handleCreateReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input ReminderCreateInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	reminders, err := s.services.ReminderService.Create(r.Context(), input.toDomain())
	if err != nil {
		writeServerError(w, err)
		return
	}
	logx.Infof("remote.create_reminder name=%s", input.Name)
	writeJSON(w, http.StatusCreated, remindersToDTOs(reminders))
}

func (s *Server) handleUpdateReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idStr := suffixPathValue(r.URL.Path, "/api/reminders/update/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid reminder id")
		return
	}
	var patch ReminderPatch
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	patch.ID = id
	reminders, err := s.services.ReminderService.Update(r.Context(), patch.toDomain())
	if err != nil {
		writeServerError(w, err)
		return
	}
	logx.Infof("remote.update_reminder id=%d", id)
	writeJSON(w, http.StatusOK, remindersToDTOs(reminders))
}

func (s *Server) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idStr := suffixPathValue(r.URL.Path, "/api/reminders/delete/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid reminder id")
		return
	}
	reminders, err := s.services.ReminderService.Delete(r.Context(), id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	logx.Infof("remote.delete_reminder id=%d", id)
	writeJSON(w, http.StatusOK, remindersToDTOs(reminders))
}

func (s *Server) handlePauseReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idStr := suffixPathValue(r.URL.Path, "/api/reminders/pause/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid reminder id")
		return
	}
	rs, err := s.services.Engine.PauseReminder(id, s.now())
	if err != nil {
		writeServerError(w, err)
		return
	}
	logx.Infof("remote.pause_reminder id=%d", id)
	s.writeRuntimeState(w, http.StatusOK, rs, r.Context())
}

func (s *Server) handleResumeReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idStr := suffixPathValue(r.URL.Path, "/api/reminders/resume/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid reminder id")
		return
	}
	rs, err := s.services.Engine.ResumeReminder(id, s.now())
	if err != nil {
		writeServerError(w, err)
		return
	}
	logx.Infof("remote.resume_reminder id=%d", id)
	s.writeRuntimeState(w, http.StatusOK, rs, r.Context())
}

func reminderToDTO(r reminderdomain.Reminder) ReminderConfig {
	return ReminderConfig{
		ID:           r.ID,
		Name:         r.Name,
		Enabled:      r.Enabled,
		IntervalSec:  r.IntervalSec,
		BreakSec:     r.BreakSec,
		ReminderType: r.ReminderType,
	}
}

func remindersToDTOs(rs []reminderdomain.Reminder) []ReminderConfig {
	dtos := make([]ReminderConfig, len(rs))
	for i, r := range rs {
		dtos[i] = reminderToDTO(r)
	}
	return dtos
}

func (in ReminderCreateInput) toDomain() reminderdomain.CreateInput {
	return reminderdomain.CreateInput{
		Name:         in.Name,
		IntervalSec:  in.IntervalSec,
		BreakSec:     in.BreakSec,
		Enabled:      in.Enabled,
		ReminderType: in.ReminderType,
	}
}

func (p ReminderPatch) toDomain() reminderdomain.Patch {
	return reminderdomain.Patch{
		ID:           p.ID,
		Name:         p.Name,
		Enabled:      p.Enabled,
		IntervalSec:  p.IntervalSec,
		BreakSec:     p.BreakSec,
		ReminderType: p.ReminderType,
	}
}
