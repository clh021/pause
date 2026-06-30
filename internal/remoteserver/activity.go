package remoteserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pause/internal/backend/bootstrap"
	"pause/internal/logx"
)

const (
	activityPollInterval  = 10 * time.Second
	activityRetentionDays = 7
)

// ActivityRecord is one tick of the activity monitor (written as JSONL).
type ActivityRecord struct {
	Timestamp int64 `json:"t"` // unix epoch seconds
	Active    bool  `json:"a"` // true=keyboard/mouse active, false=idle
	IdleSec   int   `json:"i"` // current idle seconds at time of tick
}

// ActivitySummary is returned by the API.
type ActivitySummary struct {
	TotalTicks int              `json:"totalTicks"`
	ActiveSec  int              `json:"activeSec"`
	IdleSec    int              `json:"idleSec"`
	Ticks      []ActivityRecord `json:"ticks"`
	Shots      []ShotInfo       `json:"shots"` // screenshots captured in the time range
}

// ShotInfo describes a saved screenshot JPEG.
type ShotInfo struct {
	Timestamp int64  `json:"t"`
	Path      string `json:"path"` // relative URL path for serving
	Name      string `json:"name"`
}

// ActivityRecorder polls the engine every 10 seconds and logs active/idle state.
type ActivityRecorder struct {
	engine        bootstrap.RuntimeEngine
	dir           string
	screenshotDir string

	mu            sync.Mutex
	prevActive    bool
	buffer        []ActivityRecord
	flushTicker   *time.Ticker
	captureOnAct  bool
	screenshotSvc *ScreenshotService
	nowFn         func() time.Time
}

// NewActivityRecorder creates the recorder and starts the background flush loop.
func NewActivityRecorder(engine bootstrap.RuntimeEngine, screenshotSvc *ScreenshotService, captureOnActivity bool) (*ActivityRecorder, error) {
	dir := testActivityDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("activity: mkdir: %w", err)
	}

	screenshotsDir := screenshotsBaseDir()
	if err := os.MkdirAll(screenshotsDir, 0o755); err != nil {
		return nil, fmt.Errorf("activity: screenshots mkdir: %w", err)
	}

	r := &ActivityRecorder{
		engine:        engine,
		dir:           dir,
		screenshotDir: screenshotsDir,
		buffer:        make([]ActivityRecord, 0, 64),
		flushTicker:   time.NewTicker(30 * time.Second),
		captureOnAct:  captureOnActivity,
		screenshotSvc: screenshotSvc,
		nowFn:         time.Now,
	}
	go r.flushLoop()
	return r, nil
}

// Tick polls the engine and records one activity sample. Called every 10 s.
func (r *ActivityRecorder) Tick(ctx context.Context) {
	st := r.engine.GetRuntimeState(r.nowFn())
	rec := ActivityRecord{
		Timestamp: r.nowFn().Unix(),
		Active:    st.LastTickActive,
		IdleSec:   st.CurrentIdleSec,
	}

	r.mu.Lock()
	r.buffer = append(r.buffer, rec)
	wasIdle := !r.prevActive
	isActive := st.LastTickActive
	triggerScreenshot := isActive && wasIdle && r.captureOnAct && r.screenshotSvc != nil
	r.prevActive = isActive
	r.mu.Unlock()

	if triggerScreenshot {
		r.tryAutoScreenshot(ctx)
	}
}

func (r *ActivityRecorder) tryAutoScreenshot(ctx context.Context) {
	result, err := r.screenshotSvc.Capture(ctx)
	if err != nil {
		logx.Warnf("activity.auto_screenshot_err err=%v", err)
		return
	}
	jpgPath := filepath.Join(r.screenshotDir, screenshotJPEGName(r.nowFn()))
	if err := compressPNGToJPEGFile(result.PNG, jpgPath, 55); err != nil {
		logx.Warnf("activity.auto_screenshot_compress_err err=%v", err)
		return
	}
	_ = os.Remove(result.LatestPath) // we keep the JPEG, remove the full-quality PNG
	logx.Infof("activity.auto_screenshot saved=%s", filepath.Base(jpgPath))
}

// GetActivity returns all records and screenshot paths for a time range.
func (r *ActivityRecorder) GetActivity(ctx context.Context, fromT, toT int64) (ActivitySummary, error) {
	r.mu.Lock()
	bufCopy := make([]ActivityRecord, len(r.buffer))
	copy(bufCopy, r.buffer)
	r.mu.Unlock()

	// Read activity files
	files, err := r.listActivityFiles()
	if err != nil {
		return ActivitySummary{}, err
	}
	var all []ActivityRecord
	for _, fi := range files {
		records, err := readActivityFile(fi.path)
		if err != nil {
			logx.Warnf("activity.read_file_err path=%s err=%v", fi.path, err)
			continue
		}
		all = append(all, records...)
	}
	all = append(all, bufCopy...)

	// Filter & sort ticks
	filtered := make([]ActivityRecord, 0, len(all))
	for _, rec := range all {
		if rec.Timestamp >= fromT && rec.Timestamp <= toT {
			filtered = append(filtered, rec)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp < filtered[j].Timestamp
	})

	summary := ActivitySummary{
		Ticks: filtered,
	}
	for _, rec := range filtered {
		summary.TotalTicks++
		if rec.Active {
			summary.ActiveSec += 10
		} else {
			summary.IdleSec += 10
		}
	}

	// Gather screenshot JPEGs in the time range
	summary.Shots = r.listShots(fromT, toT)
	return summary, nil
}

func (r *ActivityRecorder) listShots(fromT, toT int64) []ShotInfo {
	entries, err := os.ReadDir(r.screenshotDir)
	if err != nil {
		return nil
	}
	var shots []ShotInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "shot-") || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		ts := parseShotTimestamp(e.Name())
		if ts == 0 {
			continue
		}
		if ts >= fromT && ts <= toT {
			shots = append(shots, ShotInfo{
				Timestamp: ts,
				Path:      "/shots/" + e.Name(),
				Name:      e.Name(),
			})
		}
	}
	sort.Slice(shots, func(i, j int) bool {
		return shots[i].Timestamp < shots[j].Timestamp
	})
	return shots
}

// SetCaptureOnActivity enables or disables automatic screenshot capture.
func (r *ActivityRecorder) SetCaptureOnActivity(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.captureOnAct = enabled
}

// CaptureOnActivity returns the current auto-screenshot setting.
func (r *ActivityRecorder) CaptureOnActivity() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.captureOnAct
}

// Close stops the flush loop.
func (r *ActivityRecorder) Close() {
	r.flushTicker.Stop()
	r.flushNow()
}

func (r *ActivityRecorder) flushLoop() {
	for range r.flushTicker.C {
		r.flushNow()
	}
}

func (r *ActivityRecorder) flushNow() {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return
	}
	batch := make([]ActivityRecord, len(r.buffer))
	copy(batch, r.buffer)
	r.buffer = r.buffer[:0]
	r.mu.Unlock()

	fpath := filepath.Join(r.dir, activityFileName(r.nowFn()))
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logx.Warnf("activity.flush_err path=%s err=%v", fpath, err)
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, rec := range batch {
		if err := enc.Encode(rec); err != nil {
			logx.Warnf("activity.encode_err err=%v", err)
		}
	}

	// Prune old files
	r.pruneOldFiles()
}

func (r *ActivityRecorder) pruneOldFiles() {
	cutoff := r.nowFn().AddDate(0, 0, -activityRetentionDays)
	files, err := r.listActivityFiles()
	if err != nil {
		return
	}
	for _, fi := range files {
		if fi.modTime.Before(cutoff) {
			_ = os.Remove(fi.path)
		}
	}
	// Also prune old JPEG shots
	entries, _ := os.ReadDir(r.screenshotDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "shot-") || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(r.screenshotDir, e.Name()))
		}
	}
}

type activityFileInfo struct {
	path    string
	modTime time.Time
}

func (r *ActivityRecorder) listActivityFiles() ([]activityFileInfo, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	var files []activityFileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "activity-") || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, activityFileInfo{
			path:    filepath.Join(r.dir, e.Name()),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	return files, nil
}

func activityFileName(now time.Time) string {
	return fmt.Sprintf("activity-%s.jsonl", now.Format("2006-01-02"))
}

func readActivityFile(path string) ([]ActivityRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	records := make([]ActivityRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec ActivityRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

func listShotsInDir(dir string, fromT, toT int64) []ShotInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var shots []ShotInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "shot-") || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		// Parse timestamp from filename: shot-YYYY-MM-DD_HHMMSS.jpg
		ts := parseShotTimestamp(e.Name())
		if ts == 0 {
			continue
		}
		if ts >= fromT && ts <= toT {
			shots = append(shots, ShotInfo{
				Timestamp: ts,
				Path:      "/shots/" + e.Name(),
				Name:      e.Name(),
			})
		}
	}
	sort.Slice(shots, func(i, j int) bool {
		return shots[i].Timestamp < shots[j].Timestamp
	})
	return shots
}

// parseShotTimestamp extracts the unix timestamp from a shot filename.
// Format: shot-2006-01-02_150405.jpg  (uses local time, matching screenshotJPEGName)
func parseShotTimestamp(name string) int64 {
	// Remove prefix "shot-" and suffix ".jpg"
	mid := strings.TrimSuffix(strings.TrimPrefix(name, "shot-"), ".jpg")
	t, err := time.ParseInLocation("2006-01-02_150405", mid, time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func screenshotJPEGName(now time.Time) string {
	return fmt.Sprintf("shot-%s.jpg", now.Format("2006-01-02_150405"))
}

// compressPNGToJPEGFile decodes PNG bytes and re-encodes as JPEG at the given quality (1-100).
// Uses ImageMagick convert if available, otherwise falls back to Go stdlib.
func compressPNGToJPEGFile(pngData []byte, outputPath string, quality int) error {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}

	// Try ImageMagick first for best compression
	if cmdPath, err := exec.LookPath("convert"); err == nil {
		tmpPath := outputPath + ".tmp.png"
		if err := os.WriteFile(tmpPath, pngData, 0o644); err != nil {
			return err
		}
		defer os.Remove(tmpPath)
		cmd := exec.Command(cmdPath, tmpPath, "-quality", fmt.Sprintf("%d", quality), outputPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("imagemagick compress failed: %w: %s", err, string(output))
		}
		return nil
	}

	// Fallback: Go stdlib JPEG encoder
	return goJPEGCompress(pngData, outputPath, quality)
}
