package remoteserver

import (
	"net/http"

	settingsdomain "pause/internal/backend/domain/settings"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	settings := s.services.SettingsService.Get(r.Context())
	writeJSON(w, http.StatusOK, settingsToDTO(settings))
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var patch SettingsPatch
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.services.SettingsService.Update(r.Context(), patchToDomain(patch))
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsToDTO(updated))
}

func (s *Server) handleGetLaunchAtLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	enabled, err := s.services.SettingsService.GetLaunchAtLogin(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

func (s *Server) handleSetLaunchAtLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled, err := s.services.SettingsService.SetLaunchAtLogin(r.Context(), body.Enabled)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

func settingsToDTO(s settingsdomain.Settings) Settings {
	return Settings{
		Enforcement: EnforcementSettings{
			OverlaySkipAllowed: s.Enforcement.OverlaySkipAllowed,
		},
		Sound: SoundSettings{
			Enabled: s.Sound.Enabled,
		},
		Timer: TimerSettings{
			Mode:                  s.Timer.Mode,
			IdlePauseThresholdSec: s.Timer.IdlePauseThresholdSec,
		},
		UI: UISettings{
			ShowTrayCountdown: s.UI.ShowTrayCountdown,
			Language:          s.UI.Language,
			Theme:             s.UI.Theme,
		},
	}
}

func patchToDomain(p SettingsPatch) settingsdomain.SettingsPatch {
	dp := settingsdomain.SettingsPatch{}
	if p.Enforcement != nil {
		dp.Enforcement = &settingsdomain.EnforcementSettingsPatch{
			OverlaySkipAllowed: p.Enforcement.OverlaySkipAllowed,
		}
	}
	if p.Sound != nil {
		dp.Sound = &settingsdomain.SoundSettingsPatch{
			Enabled: p.Sound.Enabled,
		}
	}
	if p.Timer != nil {
		dp.Timer = &settingsdomain.TimerSettingsPatch{
			Mode:                  p.Timer.Mode,
			IdlePauseThresholdSec: p.Timer.IdlePauseThresholdSec,
		}
	}
	if p.UI != nil {
		dp.UI = &settingsdomain.UISettingsPatch{
			ShowTrayCountdown: p.UI.ShowTrayCountdown,
			Language:          p.UI.Language,
			Theme:             p.UI.Theme,
		}
	}
	return dp
}
