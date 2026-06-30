package remoteserver

import (
	"net/http"
	"strconv"

	analyticsdomain "pause/internal/backend/domain/analytics"
)

func (s *Server) handleWeeklyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	fromSec, toSec, err := parseAnalyticsTimeRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	stats, err := s.services.AnalyticsService.GetWeeklyStats(r.Context(), fromSec, toSec)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, analyticsWeeklyStatsToDTO(stats))
}

func (s *Server) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	fromSec, toSec, err := parseAnalyticsTimeRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := s.services.AnalyticsService.GetSummary(r.Context(), fromSec, toSec)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, analyticsSummaryToDTO(summary))
}

func (s *Server) handleAnalyticsTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	fromSec, toSec, err := parseAnalyticsTimeRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	trend, err := s.services.AnalyticsService.GetTrendByDay(r.Context(), fromSec, toSec)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, analyticsTrendToDTO(trend))
}

func (s *Server) handleBreakTypeDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	fromSec, toSec, err := parseAnalyticsTimeRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	dist, err := s.services.AnalyticsService.GetBreakTypeDistribution(r.Context(), fromSec, toSec)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, analyticsBreakTypeDistributionToDTO(dist))
}

func parseAnalyticsTimeRange(r *http.Request) (int64, int64, error) {
	fromStr := r.URL.Query().Get("fromSec")
	toStr := r.URL.Query().Get("toSec")
	fromSec, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	toSec, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return fromSec, toSec, nil
}

func analyticsWeeklyStatsToDTO(s analyticsdomain.WeeklyStats) AnalyticsWeeklyStats {
	reminders := make([]AnalyticsReminderStat, len(s.Reminders))
	for i, r := range s.Reminders {
		reminders[i] = AnalyticsReminderStat{
			ReminderID:          r.ReminderID,
			ReminderName:        r.ReminderName,
			Enabled:             r.Enabled,
			ReminderType:        r.ReminderType,
			TriggeredCount:      r.TriggeredCount,
			CompletedCount:      r.CompletedCount,
			SkippedCount:        r.SkippedCount,
			TotalActualBreakSec: r.TotalActualBreakSec,
			AvgActualBreakSec:   r.AvgActualBreakSec,
		}
	}
	return AnalyticsWeeklyStats{
		FromSec:   s.FromSec,
		ToSec:     s.ToSec,
		Reminders: reminders,
		Summary: AnalyticsSummaryStats{
			TotalSessions:       s.Summary.TotalSessions,
			TotalCompleted:      s.Summary.TotalCompleted,
			TotalSkipped:        s.Summary.TotalSkipped,
			TotalActualBreakSec: s.Summary.TotalActualBreakSec,
			AvgActualBreakSec:   s.Summary.AvgActualBreakSec,
		},
	}
}

func analyticsSummaryToDTO(s analyticsdomain.Summary) AnalyticsSummary {
	return AnalyticsSummary{
		FromSec:             s.FromSec,
		ToSec:               s.ToSec,
		TotalSessions:       s.TotalSessions,
		TotalCompleted:      s.TotalCompleted,
		TotalSkipped:        s.TotalSkipped,
		CompletionRate:      s.CompletionRate,
		SkipRate:            s.SkipRate,
		TotalActualBreakSec: s.TotalActualBreakSec,
		AvgActualBreakSec:   s.AvgActualBreakSec,
	}
}

func analyticsTrendToDTO(t analyticsdomain.Trend) AnalyticsTrend {
	points := make([]AnalyticsTrendPoint, len(t.Points))
	for i, p := range t.Points {
		points[i] = AnalyticsTrendPoint{
			Day:                 p.Day,
			TotalSessions:       p.TotalSessions,
			TotalCompleted:      p.TotalCompleted,
			TotalSkipped:        p.TotalSkipped,
			CompletionRate:      p.CompletionRate,
			SkipRate:            p.SkipRate,
			TotalActualBreakSec: p.TotalActualBreakSec,
			AvgActualBreakSec:   p.AvgActualBreakSec,
		}
	}
	return AnalyticsTrend{
		FromSec: t.FromSec,
		ToSec:   t.ToSec,
		Points:  points,
	}
}

func analyticsBreakTypeDistributionToDTO(d analyticsdomain.BreakTypeDistribution) AnalyticsBreakTypeDistribution {
	items := make([]AnalyticsBreakTypeDistributionItem, len(d.Items))
	for i, item := range d.Items {
		items[i] = AnalyticsBreakTypeDistributionItem{
			ReminderID:      item.ReminderID,
			ReminderName:    item.ReminderName,
			TriggeredCount:  item.TriggeredCount,
			CompletedCount:  item.CompletedCount,
			SkippedCount:    item.SkippedCount,
			CompletionRate:  item.CompletionRate,
			SkipRate:        item.SkipRate,
			TriggeredShare:  item.TriggeredShare,
			ReminderType:    item.ReminderType,
			ReminderEnabled: item.ReminderEnabled,
		}
	}
	return AnalyticsBreakTypeDistribution{
		FromSec:        d.FromSec,
		ToSec:          d.ToSec,
		TotalTriggered: d.TotalTriggered,
		Items:          items,
	}
}
